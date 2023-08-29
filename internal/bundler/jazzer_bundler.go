package bundler

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/mattn/go-zglob"
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
	"code-intelligence.com/cifuzz/util/sliceutil"
)

// SourceMap provides a mapping from package names
// into the corresponding source file locations
type SourceMap struct {
	JavaPackages map[string][]string `json:"java_packages,omitempty"`
}

// The directory inside the fuzzing artifact used to store runtime dependencies
const runtimeDepsPath = "runtime_deps"

type jazzerBundler struct {
	opts          *Opts
	archiveWriter archive.ArchiveWriter
}

func newJazzerBundler(opts *Opts, archiveWriter archive.ArchiveWriter) *jazzerBundler {
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

	log.Info("Creating bundle...")

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

	// add source map to archive
	sourceMap, err := b.createSourceMap()
	if err != nil {
		return nil, err
	}
	if len(sourceMap.JavaPackages) > 0 {
		jsonSourceMap, err := json.Marshal(sourceMap)
		if err != nil {
			return nil, errors.WithStack(err)
		}
		sourceMapName := "source_map.json"
		sourceMapPath := filepath.Join(b.opts.tempDir, sourceMapName)
		err = os.WriteFile(sourceMapPath, jsonSourceMap, 0644)
		if err != nil {
			return nil, errors.WithStack(err)
		}
		err = b.archiveWriter.WriteFile(sourceMapName, sourceMapPath)
		if err != nil {
			return nil, err
		}
	}

	// Iterate over build results to fill archive and create fuzzers
	for _, buildResult := range buildResults {
		fuzzTestName := buildResult.Name
		if buildResult.TargetMethod != "" {
			fuzzTestName = fuzzTestName + "::" + buildResult.TargetMethod
		}

		log.Debugf("build dir: %s\n", buildResult.BuildDir)
		// copy seeds for every fuzz test
		archiveSeedsDir, err := b.copySeeds()
		if err != nil {
			return nil, err
		}

		// creating a manifest.jar for every fuzz test to configure
		// jazzer via MANIFEST.MF
		manifestJar, err := b.createManifestJar(buildResult.Name, buildResult.TargetMethod)
		if err != nil {
			return nil, err
		}
		archiveManifestPath := filepath.Join(fuzzTestName, "manifest.jar")
		// to avoid path conflicts with the java class path, we replace
		// `::` with `_`
		archiveManifestPath = strings.ReplaceAll(archiveManifestPath, "::", "_")
		err = b.archiveWriter.WriteFile(archiveManifestPath, manifestJar)
		if err != nil {
			return nil, err
		}
		// making sure the manifest jar is the first entry in the class path
		runtimePaths := []string{
			archiveManifestPath,
		}

		// this map is used to generate unique artifact names
		artifactsMap := make(map[string]uint)

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
				// If the current runtime dependency is a file, we generate
				// a unique artifact name and add it to the archive.
				artifactName := getUniqueArtifactName(runtimeDep, artifactsMap)
				archivePath := filepath.Join(runtimeDepsPath, artifactName)
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
			Name:         fuzzTestName,
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
	err := dependencies.Check(deps, b.opts.ProjectDir)
	if err != nil {
		return err
	}
	return nil
}

func (b *jazzerBundler) runBuild() ([]*build.Result, error) {
	var fuzzTests []string
	var targetMethods []string
	var err error

	fuzzTests, targetMethods, err = b.fuzzTestIdentifier()
	if err != nil {
		return nil, err
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

		for i := range fuzzTests {
			buildResult, err := builder.Build(fuzzTests[i], targetMethods[i])
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
		for i := range fuzzTests {
			buildResult, err := builder.Build(fuzzTests[i], targetMethods[i])
			if err != nil {
				return nil, err
			}
			buildResults = append(buildResults, buildResult)
		}
	}

	return buildResults, nil
}

// create a manifest.jar to configure jazzer
func (b *jazzerBundler) createManifestJar(targetClass string, targetMethod string) (string, error) {
	// create directory for fuzzer specific files
	fuzzerPath := filepath.Join(b.opts.tempDir, targetClass)
	if targetMethod != "" {
		fuzzerPath = filepath.Join(fuzzerPath, targetMethod)
	}
	err := os.MkdirAll(fuzzerPath, 0o755)
	if err != nil {
		return "", errors.WithStack(err)
	}

	// entries for the MANIFEST.MF
	entries := map[string]string{
		options.JazzerTargetClassManifest:       targetClass,
		options.JazzerTargetClassManifestLegacy: targetClass,
	}
	if targetMethod != "" {
		entries[options.JazzerTargetMethodManifest] = targetMethod
	}

	jarPath, err := java.CreateManifestJar(entries, fuzzerPath)
	if err != nil {
		return "", err
	}

	return jarPath, nil
}

// fuzzTestIdentifier extracts all fuzz tests and their target
// methods from the fuzz tests given to the bundler.
func (b *jazzerBundler) fuzzTestIdentifier() ([]string, []string, error) {
	var err error

	testDirs, err := b.getTestDirs()
	if err != nil {
		return nil, nil, err
	}

	allValidFuzzTests, err := cmdutils.ListJVMFuzzTests(testDirs, "")
	if err != nil {
		return nil, nil, err
	}
	if len(allValidFuzzTests) == 0 {
		return nil, nil, errors.Errorf("No fuzz test could be found in the project directory '%s'", b.opts.ProjectDir)
	}

	var fuzzTests []string
	var targetMethods []string

	if len(b.opts.FuzzTests) == 0 {
		// If bundle is called without any arguments,
		// we want to bundle every fuzz test
		for _, fuzzTest := range allValidFuzzTests {
			class, targetMethod := cmdutils.SeparateTargetClassAndMethod(fuzzTest)
			fuzzTests = append(fuzzTests, class)
			targetMethods = append(targetMethods, targetMethod)
		}
	} else {
		for _, fuzzTest := range b.opts.FuzzTests {
			// Catch already specified target methods early
			if strings.Contains(fuzzTest, "::") {
				// Check first that the fuzz test actually exists
				class, targetMethod := cmdutils.SeparateTargetClassAndMethod(fuzzTest)
				if !sliceutil.Contains(allValidFuzzTests, fuzzTest) {
					return nil, nil, errors.Errorf("Fuzz test '%s' in class '%s' could not be found in the project directory '%s'", targetMethod, class, b.opts.ProjectDir)
				}

				fuzzTests = append(fuzzTests, class)
				targetMethods = append(targetMethods, targetMethod)
			} else {
				// Find all valid fuzz tests for the given class
				fuzzTestsInClass, err := cmdutils.ListJVMFuzzTests(testDirs, fuzzTest)
				if err != nil {
					return nil, nil, err
				}
				if len(fuzzTestsInClass) == 0 {
					return nil, nil, errors.Errorf("No fuzz test could be found for the given class: %s", fuzzTest)
				}

				for _, test := range fuzzTestsInClass {
					class, targetMethod := cmdutils.SeparateTargetClassAndMethod(test)
					fuzzTests = append(fuzzTests, class)
					targetMethods = append(targetMethods, targetMethod)
				}
			}
		}
	}

	return fuzzTests, targetMethods, nil
}

func (b *jazzerBundler) getTestDirs() ([]string, error) {
	var testDirs []string
	var err error
	if b.opts.BuildSystem == config.BuildSystemGradle {
		testDirs, err = gradle.GetTestSourceSets(b.opts.ProjectDir)
		if err != nil {
			return nil, err
		}
	} else if b.opts.BuildSystem == config.BuildSystemMaven {
		testDir, err := maven.GetTestDir(b.opts.ProjectDir)
		if err != nil {
			return nil, err
		}
		testDirs = append(testDirs, testDir)
	} else {
		testDirs = append(testDirs, filepath.Join(b.opts.ProjectDir, "src", "test"))
	}

	return testDirs, nil
}

func (b *jazzerBundler) getSourceDirs() ([]string, error) {
	var sourceDirs []string
	var err error
	if b.opts.BuildSystem == config.BuildSystemGradle {
		sourceDirs, err = gradle.GetMainSourceSets(b.opts.ProjectDir)
		if err != nil {
			return nil, err
		}
	} else if b.opts.BuildSystem == config.BuildSystemMaven {
		sourceDir, err := maven.GetSourceDir(b.opts.ProjectDir)
		if err != nil {
			return nil, err
		}
		sourceDirs = append(sourceDirs, sourceDir)
	} else {
		sourceDirs = append(sourceDirs, filepath.Join(b.opts.ProjectDir, "src", "main"))
	}

	return sourceDirs, nil
}

func (b *jazzerBundler) createSourceMap() (*SourceMap, error) {
	var sourceFiles []string

	sourceDirs, err := b.getSourceDirs()
	if err != nil {
		return nil, errors.WithStack(err)
	}
	testDirs, err := b.getTestDirs()
	if err != nil {
		return nil, errors.WithStack(err)
	}
	dirs := append(sourceDirs, testDirs...)
	for _, dir := range dirs {
		files, err := zglob.Glob(filepath.Join(dir, "**", "*.{java,kt}"))
		if err != nil {
			return nil, errors.WithStack(err)
		}
		sourceFiles = append(sourceFiles, files...)
	}

	sourceMap := SourceMap{
		JavaPackages: make(map[string][]string),
	}
	for _, file := range sourceFiles {
		packageName, err := func() (string, error) {
			fd, err := os.Open(file)
			if err != nil {
				return "", errors.WithStack(err)
			}
			defer fd.Close()
			return java.GetPackageFromSource(fd), nil
		}()
		if err != nil {
			return nil, err
		}
		if packageName == "" {
			continue
		}
		relPath, err := filepath.Rel(b.opts.ProjectDir, file)
		if err != nil {
			return nil, errors.WithStack(err)
		}
		// Replace double slashes on Windows with forward slashes
		relPath = strings.ReplaceAll(relPath, "\\", "/")
		sourceMap.JavaPackages[packageName] = append(sourceMap.JavaPackages[packageName], relPath)
	}

	return &sourceMap, nil
}

func getUniqueArtifactName(dependency string, artifactsMap map[string]uint) string {
	baseName := filepath.Base(dependency)
	count, found := artifactsMap[baseName]
	// If the base name of the dependency hasn't been seen before, we add it to the map
	// and return it.
	if !found {
		artifactsMap[baseName] = 1
		return baseName
	}

	// If the base name of the dependency has been seen before, we increment its
	// count in the map and append the count to the artifact base name.
	artifactsMap[baseName]++
	baseNameWithoutExt := strings.TrimSuffix(baseName, filepath.Ext(dependency))
	newBaseName := fmt.Sprintf("%s-%d.jar", baseNameWithoutExt, count)

	// Add the new base name to the map to prevent collisions
	artifactsMap[newBaseName] = 1

	return newBaseName
}
