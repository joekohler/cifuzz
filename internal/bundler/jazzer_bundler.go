package bundler

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/pkg/errors"
	"github.com/spf13/viper"

	"code-intelligence.com/cifuzz/internal/build"
	"code-intelligence.com/cifuzz/internal/build/gradle"
	"code-intelligence.com/cifuzz/internal/build/maven"
	"code-intelligence.com/cifuzz/internal/bundler/archive"
	"code-intelligence.com/cifuzz/internal/cmdutils"
	"code-intelligence.com/cifuzz/internal/config"
	"code-intelligence.com/cifuzz/pkg/dependencies"
	"code-intelligence.com/cifuzz/pkg/java"
	"code-intelligence.com/cifuzz/pkg/log"
	"code-intelligence.com/cifuzz/pkg/options"
)

// The directory inside the fuzzing artifact used to store runtime dependencies
const runtimeDepsPath = "runtime_deps"

type jazzerBundler struct {
	opts          *Opts
	archiveWriter *archive.ArchiveWriter
}

func newJazzerBundler(opts *Opts, archiveWriter *archive.ArchiveWriter) *jazzerBundler {
	return &jazzerBundler{opts, archiveWriter}
}

func (b *jazzerBundler) bundle() ([]*archive.Fuzzer, error) {
	err := b.checkDependencies()
	if err != nil {
		return nil, err
	}

	buildResults, err := b.runBuild()
	if err != nil {
		return nil, err
	}

	return b.assembleArtifacts(buildResults)
}

func (b *jazzerBundler) assembleArtifacts(buildResults []*build.Result) ([]*archive.Fuzzer, error) {
	var fuzzers []*archive.Fuzzer

	var archiveDict string
	if b.opts.Dictionary != "" {
		archiveDict = "dict"
		err := b.archiveWriter.WriteFile(archiveDict, b.opts.Dictionary)
		if err != nil {
			return nil, err
		}
	}

	// Iterate over build results to fill archive and create fuzzers
	for _, buildResult := range buildResults {
		log.Debugf("build dir: %s\n", buildResult.BuildDir)
		// copy seeds for every fuzz test
		archiveSeedsDir, err := b.copySeeds()
		if err != nil {
			return nil, err
		}

		// creating a manifest.jar for every fuzz test to configure
		// jazzer via MANIFEST.MF
		manifestJar, err := b.createManifestJar(buildResult.Name)
		if err != nil {
			return nil, err
		}
		archiveManifestPath := filepath.Join(buildResult.Name, "manifest.jar")
		err = b.archiveWriter.WriteFile(archiveManifestPath, manifestJar)
		if err != nil {
			return nil, err
		}
		// making sure the manifest jar is the first entry in the class path
		runtimePaths := []string{
			archiveManifestPath,
		}

		for _, runtimeDep := range buildResult.RuntimeDeps {
			log.Debugf("runtime dept: %s", runtimeDep)

			// check if the file exists
			entry, err := os.Stat(runtimeDep)
			if os.IsNotExist(err) {
				continue
			}
			if err != nil {
				return nil, errors.WithStack(err)
			}

			if entry.IsDir() {
				// if the current runtime dep is a directory, add all files to
				// the archive but add just the directory path to the runtime
				// paths. Hence, there will be a single entry for the runtime
				// path but multiple entries in the archive.
				relPath, err := filepath.Rel(buildResult.ProjectDir, runtimeDep)
				if err != nil {
					return nil, errors.WithStack(err)
				}
				relPath = filepath.Join(runtimeDepsPath, relPath)
				runtimePaths = append(runtimePaths, relPath)

				err = b.archiveWriter.WriteDir(relPath, runtimeDep)
				if err != nil {
					return nil, err
				}
			} else {
				// if the current runtime dependency is a file we add it to the
				// archive and add the runtime paths of the metadata
				archivePath := filepath.Join(runtimeDepsPath, filepath.Base(runtimeDep))
				err = b.archiveWriter.WriteFile(archivePath, runtimeDep)
				if err != nil {
					return nil, err
				}
				runtimePaths = append(runtimePaths, archivePath)
			}
		}

		// convert back slashes to forward slashes on windows to make
		// sure that the bundle can be executed on the linux based
		// workers
		// it is done here, right before the creation of the fuzzer struct,
		// to make sure that we do not accidentally miss a runtime path with
		// back slashes
		if runtime.GOOS == "windows" {
			for i, runtimePath := range runtimePaths {
				runtimePaths[i] = filepath.ToSlash(runtimePath)
			}
		}

		fuzzer := &archive.Fuzzer{
			Name:         buildResult.Name,
			Engine:       "JAVA_LIBFUZZER",
			ProjectDir:   buildResult.ProjectDir,
			Dictionary:   archiveDict,
			Seeds:        archiveSeedsDir,
			RuntimePaths: runtimePaths,
			EngineOptions: archive.EngineOptions{
				Env:   b.opts.Env,
				Flags: b.opts.EngineArgs,
			},
			MaxRunTime: uint(b.opts.Timeout.Seconds()),
		}

		fuzzers = append(fuzzers, fuzzer)
	}
	return fuzzers, nil
}

func (b *jazzerBundler) copySeeds() (string, error) {
	// Add seeds from user-specified seed corpus dirs (if any)
	// to the seeds directory in the archive
	// TODO: Isn't this missing the seed corpus from the build result?
	var archiveSeedsDir string
	if len(b.opts.SeedCorpusDirs) > 0 {
		archiveSeedsDir = "seeds"
		err := prepareSeeds(b.opts.SeedCorpusDirs, archiveSeedsDir, b.archiveWriter)
		if err != nil {
			return "", err
		}
	}

	return archiveSeedsDir, nil
}

func (b *jazzerBundler) checkDependencies() error {
	var deps []dependencies.Key
	switch b.opts.BuildSystem {
	case config.BuildSystemMaven:
		deps = []dependencies.Key{dependencies.Java, dependencies.Maven}
	case config.BuildSystemGradle:
		deps = []dependencies.Key{dependencies.Java, dependencies.Gradle}
	}
	err := dependencies.Check(deps)
	if err != nil {
		log.Error(err)
		return cmdutils.WrapSilentError(err)
	}
	return nil
}

func (b *jazzerBundler) runBuild() ([]*build.Result, error) {
	var fuzzTests []string
	var err error

	if len(b.opts.FuzzTests) == 0 {
		fuzzTests, err = cmdutils.ListJVMFuzzTests(b.opts.ProjectDir)
		if err != nil {
			return nil, err
		}
	} else {
		fuzzTests = b.opts.FuzzTests
	}

	var buildResults []*build.Result
	switch b.opts.BuildSystem {
	case config.BuildSystemMaven:
		if len(b.opts.BuildSystemArgs) > 0 {
			log.Warnf("Passing additional arguments is not supported for Maven.\n"+
				"These arguments are ignored: %s", strings.Join(b.opts.BuildSystemArgs, " "))
		}

		builder, err := maven.NewBuilder(&maven.BuilderOptions{
			ProjectDir: b.opts.ProjectDir,
			Parallel: maven.ParallelOptions{
				Enabled: viper.IsSet("build-jobs"),
				NumJobs: b.opts.NumBuildJobs,
			},
			Stdout: b.opts.BuildStdout,
			Stderr: b.opts.BuildStderr,
		})
		if err != nil {
			return nil, err
		}

		for _, test := range fuzzTests {
			buildResult, err := builder.Build(test)
			if err != nil {
				return nil, err
			}
			buildResults = append(buildResults, buildResult)
		}
	case config.BuildSystemGradle:
		if len(b.opts.BuildSystemArgs) > 0 {
			log.Warnf("Passing additional arguments is not supported for Gradle.\n"+
				"These arguments are ignored: %s", strings.Join(b.opts.BuildSystemArgs, " "))
		}

		builder, err := gradle.NewBuilder(&gradle.BuilderOptions{
			ProjectDir: b.opts.ProjectDir,
			Parallel: gradle.ParallelOptions{
				Enabled: viper.IsSet("build-jobs"),
				NumJobs: b.opts.NumBuildJobs,
			},
			Stdout: b.opts.BuildStdout,
			Stderr: b.opts.BuildStderr,
		})
		if err != nil {
			return nil, err
		}
		for _, test := range fuzzTests {
			buildResult, err := builder.Build(test)
			if err != nil {
				return nil, err
			}
			buildResults = append(buildResults, buildResult)
		}
	}

	return buildResults, nil
}

// create a manifest.jar to configure jazzer
func (b *jazzerBundler) createManifestJar(targetClass string) (string, error) {
	// create directory for fuzzer specific files
	fuzzerPath := filepath.Join(b.opts.tempDir, targetClass)
	err := os.MkdirAll(fuzzerPath, 0o755)
	if err != nil {
		return "", errors.WithStack(err)
	}

	// entries for the MANIFEST.MF
	entries := map[string]string{
		options.JazzerTargetClassManifest: targetClass,
	}

	jarPath, err := java.CreateManifestJar(entries, fuzzerPath)
	if err != nil {
		return "", err
	}

	return jarPath, nil
}
