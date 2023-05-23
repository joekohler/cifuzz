package bundler

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/pkg/errors"

	"code-intelligence.com/cifuzz/internal/bundler/archive"
	"code-intelligence.com/cifuzz/internal/config"
	"code-intelligence.com/cifuzz/pkg/log"
	"code-intelligence.com/cifuzz/pkg/vcs"
	"code-intelligence.com/cifuzz/util/fileutil"
	"code-intelligence.com/cifuzz/util/sliceutil"
)

// The (possibly empty) directory inside the fuzzing artifact archive that will
// be the fuzzers working directory.
const archiveWorkDirPath = "work_dir"

type Bundler struct {
	opts *Opts
}

func New(opts *Opts) *Bundler {
	return &Bundler{opts: opts}
}

func (b *Bundler) Bundle() error {
	var dockerImageUsedInBundle = b.opts.DockerImage

	// create temp dir
	var err error
	b.opts.tempDir, err = os.MkdirTemp("", "cifuzz-bundle-")
	if err != nil {
		return errors.WithStack(err)
	}
	defer fileutil.Cleanup(b.opts.tempDir)

	if b.opts.OutputPath != "" {
		// do nothing
	} else if len(b.opts.FuzzTests) == 1 {
		b.opts.OutputPath = filepath.Base(b.opts.FuzzTests[0]) + ".tar.gz"
	} else {
		b.opts.OutputPath = "fuzz_tests.tar.gz"
	}

	bundle, err := os.Create(b.opts.OutputPath)
	if err != nil {
		return errors.Wrap(err, "failed to create fuzzing artifact archive")
	}
	defer bundle.Close()

	bufWriter := bufio.NewWriter(bundle)
	archiveWriter := archive.NewArchiveWriter(bufWriter)

	var fuzzers []*archive.Fuzzer

	switch b.opts.BuildSystem {
	case config.BuildSystemCMake, config.BuildSystemBazel, config.BuildSystemOther:
		fuzzers, err = newLibfuzzerBundler(b.opts, archiveWriter).bundle()
		// Use default Ubuntu Docker image for CMake, Bazel, and other build systems
		if dockerImageUsedInBundle == "" {
			dockerImageUsedInBundle = "ubuntu:rolling"
		}
	case config.BuildSystemMaven, config.BuildSystemGradle:
		fuzzers, err = newJazzerBundler(b.opts, archiveWriter).bundle()
		// Maven and Gradle should use a Docker image with Java
		if dockerImageUsedInBundle == "" {
			dockerImageUsedInBundle = "openjdk:latest"
		}
	}
	if err != nil {
		return err
	}

	// Create and add the top-level metadata file.
	metadata := &archive.Metadata{
		Fuzzers: fuzzers,
		RunEnvironment: &archive.RunEnvironment{
			Docker: dockerImageUsedInBundle,
		},
		CodeRevision: b.getCodeRevision(),
	}

	metadataYamlContent, err := metadata.ToYaml()
	if err != nil {
		return err
	}
	metadataYamlPath := filepath.Join(b.opts.tempDir, archive.MetadataFileName)
	err = os.WriteFile(metadataYamlPath, metadataYamlContent, 0644)
	if err != nil {
		return errors.Wrapf(err, "failed to write %s", archive.MetadataFileName)
	}
	err = archiveWriter.WriteFile(archive.MetadataFileName, metadataYamlPath)
	if err != nil {
		return err
	}

	// The fuzzing artifact archive spec requires this directory even if it is empty.
	tempWorkDirPath := filepath.Join(b.opts.tempDir, archiveWorkDirPath)
	err = os.Mkdir(tempWorkDirPath, 0755)
	if err != nil {
		return errors.WithStack(err)
	}
	err = archiveWriter.WriteDir(archiveWorkDirPath, tempWorkDirPath)
	if err != nil {
		return err
	}

	for _, arg := range b.opts.AdditionalFiles {
		source, target, err := parseAdditionalFilesArgument(arg)
		if err != nil {
			return err
		}

		if !filepath.IsAbs(source) {
			source = filepath.Join(b.opts.ProjectDir, source)
		}

		if fileutil.IsDir(source) {
			err = archiveWriter.WriteDir(target, source)
			if err != nil {
				return err
			}
		} else {
			err = archiveWriter.WriteFile(target, source)
			if err != nil {
				return err
			}
		}
	}

	err = archiveWriter.Close()
	if err != nil {
		return errors.WithStack(err)
	}
	err = bufWriter.Flush()
	if err != nil {
		return errors.WithStack(err)
	}
	err = bundle.Close()
	if err != nil {
		return errors.WithStack(err)
	}

	return nil
}

func (b *Bundler) getCodeRevision() *archive.CodeRevision {
	var err error
	var gitCommit string
	var gitBranch string

	if b.opts.Commit == "" {
		gitCommit, err = vcs.GitCommit()
		if err != nil {
			log.Debugf("failed to get Git commit: %+v", err)
			return nil
		}
	} else {
		gitCommit = b.opts.Commit
	}

	if b.opts.Branch == "" {
		gitBranch, err = vcs.GitBranch()
		if err != nil {
			log.Debugf("failed to get Git branch: %+v", err)
			return nil
		}
	} else {
		gitBranch = b.opts.Branch
	}

	if vcs.GitIsDirty() {
		log.Warnf("The Git repository has uncommitted changes. Archive metadata may be inaccurate.")
	}

	return &archive.CodeRevision{
		Git: &archive.GitRevision{
			Commit: gitCommit,
			Branch: gitBranch,
		},
	}
}

func prepareSeeds(seedCorpusDirs []string, archiveSeedsDir string, archiveWriter *archive.ArchiveWriter) error {
	var targetDirs []string
	for _, sourceDir := range seedCorpusDirs {
		// Put the seeds into subdirectories of the "seeds" directory
		// to avoid seeds with the same name to override each other.

		// Choose a name for the target directory which wasn't used
		// before
		basename := filepath.Join(archiveSeedsDir, filepath.Base(sourceDir))
		targetDir := basename
		i := 1
		for sliceutil.Contains(targetDirs, targetDir) {
			targetDir = fmt.Sprintf("%s-%d", basename, i)
			i++
		}
		targetDirs = append(targetDirs, targetDir)

		// Add the seeds of the seed corpus directory to the target directory
		err := archiveWriter.WriteDir(targetDir, sourceDir)
		if err != nil {
			return err
		}
	}
	return nil
}

func parseAdditionalFilesArgument(arg string) (string, string, error) {
	var source, target string
	parts := strings.Split(arg, ";")

	if len(parts) == 1 {
		// if there is no ; separator just use the work_dir
		// handles "test.txt"
		source = parts[0]
		target = filepath.Join(archiveWorkDirPath, filepath.Base(arg))
	} else {
		// handles test.txt;test2.txt
		source = parts[0]
		target = parts[1]
	}

	if len(parts) > 2 || source == "" || target == "" {
		return "", "", errors.New("could not parse '--add' argument")
	}

	if filepath.IsAbs(target) {
		return "", "", errors.New("when using '--add source;target', target has to be a relative path")
	}

	return source, target, nil
}
