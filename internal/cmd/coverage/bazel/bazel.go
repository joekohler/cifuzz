package bazel

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/pkg/errors"

	"code-intelligence.com/cifuzz/internal/build"
	"code-intelligence.com/cifuzz/internal/build/bazel"
	"code-intelligence.com/cifuzz/internal/cmdutils"
	"code-intelligence.com/cifuzz/pkg/log"
	"code-intelligence.com/cifuzz/pkg/parser/coverage"
	"code-intelligence.com/cifuzz/pkg/runfiles"
	"code-intelligence.com/cifuzz/util/envutil"
	"code-intelligence.com/cifuzz/util/fileutil"
)

type CoverageGenerator struct {
	FuzzTest        string
	OutputFormat    string
	OutputPath      string
	BuildSystemArgs []string
	ProjectDir      string
	Engine          string
	NumJobs         uint
	CorpusDirs      []string
	Stdout          io.Writer
	Stderr          io.Writer
	BuildStdout     io.Writer
	BuildStderr     io.Writer
	Verbose         bool
}

// symlinkUserInputsToGeneratedCorpus handles user defined inputs set via
// '--corpus-dir'.
// They are added as symlinks to the generated corpus directory, so they can be
// included while creating the coverage report and are removed afterward. The
// generated corpus is automatically included in the 'bazel coverage' command
// because of the "@cifuzz//:collect_coverage" line in the fuzz test definition.
// This solution is used because there is no easy way to add them via flags
// to bazel or without adjusting the BUILD.bazel.
func (cov *CoverageGenerator) symlinkUserInputsToGeneratedCorpus(commonFlags []string) (func(), error) {
	symlinks := make([]string, 0)

	// Get path to generated corpus of the fuzz test
	fuzzTestPath, err := bazel.PathFromLabel(cov.FuzzTest, commonFlags)
	if err != nil {
		return nil, err
	}
	generatedCorpusBasename := "." + filepath.Base(fuzzTestPath) + "_cifuzz_corpus"
	generatedCorpus := filepath.Join(cov.ProjectDir, filepath.Dir(fuzzTestPath), generatedCorpusBasename)

	// Make sure that the generated corpus directory actually exists. If the user
	// for any reason calls the coverage command without a prior fuzzing run, we
	// still want this to work but also delete the directory again to not clutter
	// up the project.
	exist, err := fileutil.Exists(generatedCorpus)
	if err != nil {
		return nil, err
	}
	if !exist {
		err = os.Mkdir(generatedCorpus, 0755)
		if err != nil {
			return nil, errors.WithStack(err)
		}
		// Add it to the symlinks slice to delete it with the others at the end of
		// the build step
		symlinks = append(symlinks, generatedCorpus)
	}

	for _, dir := range cov.CorpusDirs {
		// Create a symlink in the generated corpus directory for every input in the
		// user specified directories
		err = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return errors.WithStack(err)
			}

			info, err := d.Info()
			if err != nil {
				return errors.WithStack(err)
			}
			if info.IsDir() {
				return nil
			}

			link := filepath.Join(generatedCorpus, filepath.Base(path))
			err = os.Symlink(path, link)
			if err != nil && !errors.Is(err, os.ErrExist) {
				return errors.Wrapf(err, "Failed to symlink'%s' to '%s'", path, generatedCorpus)
			}
			if errors.Is(err, os.ErrExist) {
				// TODO: don't skip input if a file with the same name already exists
				log.Debugf("File %s already exists in corpus directory, a symlink for %s cannot be created", link, path)
				// Return here to not add the path to the list of files that will be
				// removed after the coverage run
				return nil
			}

			symlinks = append(symlinks, link)
			return nil
		})
		// nolint: wrapcheck
		if err != nil {
			return nil, err
		}
	}

	removeSymlinks := func() {
		for _, s := range symlinks {
			err = os.RemoveAll(s)
			if err != nil {
				log.Errorf(errors.WithStack(err), "Failed to remove '%s': %v", s, err.Error())
			}
		}
	}

	return removeSymlinks, nil
}

func (cov *CoverageGenerator) BuildFuzzTestForCoverage() error {
	commonFlags, err := cov.getBazelCommandFlags()
	if err != nil {
		return err
	}

	if len(cov.CorpusDirs) != 0 {
		removeSymlinks, err := cov.symlinkUserInputsToGeneratedCorpus(commonFlags)
		if err != nil {
			return err
		}
		defer removeSymlinks()
	}

	// The cc_fuzz_test rule defines multiple bazel targets: If the
	// name is "foo", it defines the targets "foo", "foo_bin", and
	// others. We need to run the "foo" target here but want to
	// allow users to specify either "foo" or "foo_bin", so we check
	// if the fuzz test name  with a "_bin" suffix removed is a valid
	// target and use that in that case.
	if strings.HasSuffix(cov.FuzzTest, "_bin") {
		trimmedLabel := strings.TrimSuffix(cov.FuzzTest, "_bin")
		cmd := exec.Command("bazel", "query", trimmedLabel)
		err = cmd.Run()
		if err == nil {
			cov.FuzzTest = trimmedLabel
		}
	}

	// Flags which should only be used for bazel run because they are
	// not supported by the other bazel commands we use
	coverageFlags := []string{
		// Build with debug symbols
		"-c", "opt", "--copt", "-g",
		// Disable source fortification, which is currently not supported
		// in combination with ASan, see https://github.com/google/sanitizers/issues/247
		"--copt", "-U_FORTIFY_SOURCE",
		// Build with the rules_fuzzing replayer
		"--@rules_fuzzing//fuzzing:cc_engine=@rules_fuzzing//fuzzing/engines:replay",
		"--@rules_fuzzing//fuzzing:cc_engine_instrumentation=none",
		"--@rules_fuzzing//fuzzing:cc_engine_sanitizer=none",
		"--instrument_test_targets",
		"--combined_report=lcov",
		"--experimental_use_llvm_covmap",
		"--experimental_generate_llvm_lcov",
		"--verbose_failures",
	}
	if os.Getenv("BAZEL_SUBCOMMANDS") != "" {
		coverageFlags = append(coverageFlags, "--subcommands")
	}

	args := []string{"coverage"}
	args = append(args, commonFlags...)
	args = append(args, coverageFlags...)
	args = append(args, cov.BuildSystemArgs...)
	args = append(args, cov.FuzzTest)

	cmd := exec.Command("bazel", args...)
	// Redirect the build command's stdout to stderr to only have
	// reports printed to stdout
	cmd.Stdout = cov.BuildStdout
	cmd.Stderr = cov.BuildStderr
	log.Debugf("Command: %s", cmd.String())
	err = cmd.Run()
	if err != nil {
		return cmdutils.WrapExecError(errors.WithStack(err), cmd)
	}

	return nil
}

func (cov *CoverageGenerator) GenerateCoverageReport() (string, error) {
	// Get the path of the created lcov report
	cmd := exec.Command("bazel", "info", "output_path")
	out, err := cmd.Output()
	if err != nil {
		return "", cmdutils.WrapExecError(errors.WithStack(err), cmd)
	}
	bazelOutputDir := strings.TrimSpace(string(out))
	reportPath := filepath.Join(bazelOutputDir, "_coverage", "_coverage_report.dat")

	log.Debugf("Parsing lcov report %s", reportPath)

	lcovReportContent, err := os.ReadFile(reportPath)
	if err != nil {
		return "", errors.WithStack(err)
	}
	reportReader := strings.NewReader(string(lcovReportContent))
	summary, err := coverage.ParseLCOVReportIntoSummary(reportReader)
	if err != nil {
		return "", err
	}
	summary.PrintTable(cov.Stderr)

	commonFlags, err := cov.getBazelCommandFlags()
	if err != nil {
		return "", err
	}

	if cov.OutputFormat == "lcov" {
		if cov.OutputPath == "" {
			path, err := bazel.PathFromLabel(cov.FuzzTest, commonFlags)
			if err != nil {
				return "", err
			}
			name := strings.ReplaceAll(path, "/", "-")
			cov.OutputPath = name + ".coverage.lcov"
		}
		// We don't use copy.Copy here to be able to set the permissions
		// to 0o644 before umask - copy.Copy just copies the permissions
		// from the source file, which has permissions 555 like all
		// files created by bazel.
		content, err := os.ReadFile(reportPath)
		if err != nil {
			return "", errors.WithStack(err)
		}
		err = os.WriteFile(cov.OutputPath, content, 0o644)
		if err != nil {
			return "", errors.WithStack(err)
		}
		return cov.OutputPath, nil
	}

	// If no output path was specified, create the coverage report in a
	// temporary directory
	if cov.OutputPath == "" {
		outputDir, err := os.MkdirTemp("", "coverage-")
		if err != nil {
			return "", errors.WithStack(err)
		}
		path, err := bazel.PathFromLabel(cov.FuzzTest, commonFlags)
		if err != nil {
			return "", err
		}
		cov.OutputPath = filepath.Join(outputDir, path)
	}

	// Create an HTML report via genhtml
	genHTML, err := runfiles.Finder.GenHTMLPath()
	if err != nil {
		return "", err
	}
	args := []string{"--output", cov.OutputPath, reportPath}

	cmd = exec.Command(genHTML, args...)
	cmd.Dir = cov.ProjectDir
	cmd.Stderr = os.Stderr
	log.Debugf("Command: %s", cmd.String())
	err = cmd.Run()
	if err != nil {
		return "", errors.WithStack(err)
	}

	return cov.OutputPath, nil
}

// getBazelCommandFlags returns flags to be used when executing a bazel command
// to avoid part of the loading and/or analysis phase to rerun.
func (cov *CoverageGenerator) getBazelCommandFlags() ([]string, error) {
	env, err := build.CommonBuildEnv()
	if err != nil {
		return nil, err
	}

	flags := []string{
		"--repo_env=CC=" + envutil.Getenv(env, "CC"),
		"--repo_env=CXX=" + envutil.Getenv(env, "CXX"),
		// Don't use the LLVM from Xcode
		"--repo_env=BAZEL_USE_CPP_ONLY_TOOLCHAIN=1",
	}
	if cov.NumJobs != 0 {
		flags = append(flags, "--jobs", fmt.Sprint(cov.NumJobs))
	}

	llvmCov, err := runfiles.Finder.LLVMCovPath()
	if err != nil {
		return nil, err
	}
	llvmProfData, err := runfiles.Finder.LLVMProfDataPath()
	if err != nil {
		return nil, err
	}
	flags = append(flags,
		"--repo_env=BAZEL_USE_LLVM_NATIVE_COVERAGE=1",
		"--repo_env=BAZEL_LLVM_COV="+llvmCov,
		"--repo_env=BAZEL_LLVM_PROFDATA="+llvmProfData,
		"--repo_env=GCOV="+llvmProfData,
	)

	return flags, nil
}
