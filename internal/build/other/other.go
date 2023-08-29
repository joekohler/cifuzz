package other

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/pkg/errors"
	"golang.org/x/exp/slices"

	"code-intelligence.com/cifuzz/internal/build"
	"code-intelligence.com/cifuzz/internal/cmdutils"
	"code-intelligence.com/cifuzz/internal/ldd"
	"code-intelligence.com/cifuzz/pkg/dependencies"
	"code-intelligence.com/cifuzz/pkg/log"
	"code-intelligence.com/cifuzz/pkg/runfiles"
	"code-intelligence.com/cifuzz/util/envutil"
	"code-intelligence.com/cifuzz/util/fileutil"
)

// Warning: Changing these will lead to a breaking change!
const (
	// EnvBuildStep states for what a fuzz test is build.
	// e.g. "coverage", "fuzzing"
	EnvBuildStep string = "CIFUZZ_BUILD_STEP"

	// EnvBuildLocation states the path of the executable.
	// Default is identical with FUZZ_TEST.
	EnvBuildLocation string = "CIFUZZ_BUILD_LOCATION"

	// EnvCommand holds the name of the command cifuzz was called with.
	// e.g. "run", "bundle", "remote-run"
	EnvCommand string = "CIFUZZ_COMMAND"

	// EnvFuzzTest holds the name of the fuzz test.
	EnvFuzzTest string = "FUZZ_TEST"

	// EnvFuzzTestCFlags hold the CFLAGS used for building the fuzz test.
	EnvFuzzTestCFlags string = "FUZZ_TEST_CFLAGS"

	// EnvFuzzTestCXXFlags hold the CXXFLAGS used for building the fuzz test.
	EnvFuzzTestCXXFlags string = "FUZZ_TEST_CXXFLAGS"

	// EnvFuzzTestLDFlags hold the LDFLAGS used for building the fuzz test.
	EnvFuzzTestLDFlags string = "FUZZ_TEST_LDFLAGS"
)

type BuilderOptions struct {
	ProjectDir   string
	BuildCommand string
	CleanCommand string
	Sanitizers   []string

	RunfilesFinder runfiles.RunfilesFinder
	Stdout         io.Writer
	Stderr         io.Writer
}

func (opts *BuilderOptions) Validate() error {
	// Check that the project dir is set
	if opts.ProjectDir == "" {
		return errors.New("ProjectDir is not set")
	}
	// Check that the project dir exists and can be accessed
	_, err := os.Stat(opts.ProjectDir)
	if err != nil {
		return errors.WithStack(err)
	}

	if opts.RunfilesFinder == nil {
		opts.RunfilesFinder = runfiles.Finder
	}

	return nil
}

type Builder struct {
	*BuilderOptions
	env      []string
	buildDir string
}

func NewBuilder(opts *BuilderOptions) (*Builder, error) {
	err := opts.Validate()
	if err != nil {
		return nil, err
	}

	b := &Builder{BuilderOptions: opts}

	// Create a temporary build directory
	b.buildDir, err = os.MkdirTemp("", "cifuzz-build-")
	if err != nil {
		return nil, errors.WithStack(err)
	}

	b.env, err = build.CommonBuildEnv()
	if err != nil {
		return nil, err
	}

	// Set CFLAGS, CXXFLAGS, LDFLAGS, and FUZZ_TEST_LDFLAGS which must
	// be passed to the build commands by the build system.
	if len(opts.Sanitizers) == 1 && opts.Sanitizers[0] == "coverage" {
		err = b.setCoverageEnv()
	} else {
		for _, sanitizer := range opts.Sanitizers {
			if sanitizer != "address" && sanitizer != "undefined" {
				panic(fmt.Sprintf("Invalid sanitizer: %q", sanitizer))
			}
		}
		err = b.setLibFuzzerEnv()
	}
	if err != nil {
		return nil, err
	}

	return b, nil
}

// Build builds the specified fuzz test via the user-specified build command
func (b *Builder) Build(fuzzTest string) (*build.CBuildResult, error) {
	var err error

	if !slices.Equal(b.Sanitizers, []string{"coverage"}) {
		// We compile the dumper without any user-provided flags. This
		// should be safe as it does not use any stdlib functions.
		dumperSource, err := b.RunfilesFinder.DumperSourcePath()
		if err != nil {
			return nil, err
		}
		clang, err := b.RunfilesFinder.ClangPath()
		if err != nil {
			return nil, err
		}
		// Compile with -fPIC just in case the fuzz test is a PIE.
		cmd := exec.Command(clang, "-fPIC", "-c", dumperSource, "-o", filepath.Join(b.buildDir, "dumper.o"))
		cmd.Stdout = b.Stdout
		cmd.Stderr = b.Stderr
		log.Debugf("Dumper Command: %s", cmd.String())
		err = cmd.Run()
		if err != nil {
			return nil, errors.WithStack(err)
		}
	}

	err = b.setBuildCommandEnv(fuzzTest)
	if err != nil {
		return nil, err
	}

	// Run the build command
	cmd := exec.Command("/bin/sh", "-c", b.BuildCommand)
	cmd.Stdout = b.Stdout
	cmd.Stderr = b.Stderr
	cmd.Env = b.env
	log.Debugf("Build Command: %s", cmd.String())
	err = cmd.Run()
	if err != nil {
		return nil, cmdutils.WrapExecError(errors.WithStack(err), cmd)
	}

	executable, err := b.findFuzzTestExecutable(fuzzTest)
	if err != nil {
		return nil, err
	}

	if executable == "" {
		return nil, cmdutils.WrapExecError(errors.Errorf("Could not find executable for fuzz test %q", fuzzTest), cmd)
	}

	// For the build system type "other", we expect the default seed corpus next
	// to the fuzzer executable.
	seedCorpus := executable + "_inputs"
	runtimeDeps, err := ldd.NonSystemSharedLibraries(executable)
	if err != nil {
		return nil, err
	}

	wd, err := os.Getwd()
	if err != nil {
		return nil, errors.WithStack(err)
	}

	generatedCorpus := filepath.Join(b.ProjectDir, ".cifuzz-corpus", fuzzTest)
	return &build.CBuildResult{
		Name:       fuzzTest,
		ProjectDir: b.ProjectDir,
		Sanitizers: b.Sanitizers,
		BuildResult: &build.BuildResult{
			Executable:      executable,
			GeneratedCorpus: generatedCorpus,
			SeedCorpus:      seedCorpus,
			BuildDir:        wd,
			RuntimeDeps:     runtimeDeps,
		},
	}, nil
}

// Clean cleans the project's build artifacts user-specified build command.
func (b *Builder) Clean() error {
	if b.CleanCommand == "" {
		log.Debug("No clean command provided")
		return nil
	}

	err := b.setCleanCommandEnv()
	if err != nil {
		return err
	}

	// Run the clean command
	cmd := exec.Command("/bin/sh", "-c", b.CleanCommand)
	cmd.Stdout = b.Stdout
	cmd.Stderr = b.Stderr
	cmd.Env = b.env
	log.Debugf("Clean Command: %s", cmd.String())
	if err := cmd.Run(); err != nil {
		return cmdutils.WrapExecError(errors.WithStack(err), cmd)
	}

	return nil
}

func (b *Builder) setBuildCommandEnv(fuzzTest string) error {
	var err error

	b.env, err = setEnvWithDebugMsg(b.env, EnvCommand, cmdutils.CurrentInvocation.Command)
	if err != nil {
		return err
	}

	b.env, err = setEnvWithDebugMsg(b.env, EnvFuzzTest, fuzzTest)
	if err != nil {
		return err
	}

	b.env, err = setEnvWithDebugMsg(b.env, EnvBuildLocation, fuzzTest)
	if err != nil {
		return err
	}

	return nil
}

func (b *Builder) setCleanCommandEnv() error {
	var err error

	b.env, err = setEnvWithDebugMsg(b.env, EnvCommand, cmdutils.CurrentInvocation.Command)
	if err != nil {
		return err
	}

	return nil
}

func (b *Builder) setLibFuzzerEnv() error {
	var err error

	b.env, err = setEnvWithDebugMsg(b.env, EnvBuildStep, "fuzzing")
	if err != nil {
		return err
	}

	// Set CFLAGS and CXXFLAGS
	cflags := build.LibFuzzerCFlags()
	b.env, err = setEnvWithDebugMsg(b.env, "CFLAGS", strings.Join(cflags, " "))
	if err != nil {
		return err
	}
	b.env, err = setEnvWithDebugMsg(b.env, "CXXFLAGS", strings.Join(cflags, " "))
	if err != nil {
		return err
	}

	ldflags := []string{
		// ----- Flags used to build with ASan -----
		// Link ASan and UBSan runtime
		"-fsanitize=address,undefined",
	}
	b.env, err = setEnvWithDebugMsg(b.env, "LDFLAGS", strings.Join(ldflags, " "))
	if err != nil {
		return err
	}

	// Users should pass the environment variable FUZZ_TEST_CFLAGS or
	// FUZZ_TEST_CXXFLAGS to the compiler command building the fuzz test.
	cifuzzIncludePath, err := b.RunfilesFinder.CIFuzzIncludePath()
	if err != nil {
		return err
	}
	// -I adds the include directory to the list of directories
	// to be searched for header files
	fuzzTestCFlags := []string{fmt.Sprintf("-I%s", cifuzzIncludePath)}
	b.env, err = setEnvWithDebugMsg(b.env, EnvFuzzTestCFlags, strings.Join(fuzzTestCFlags, " "))
	if err != nil {
		return err
	}
	b.env, err = setEnvWithDebugMsg(b.env, EnvFuzzTestCXXFlags, strings.Join(fuzzTestCFlags, " "))
	if err != nil {
		return err
	}

	// Users should pass the environment variable FUZZ_TEST_LDFLAGS to
	// the linker command building the fuzz test. For libfuzzer, we set
	// it to "-fsanitize=fuzzer" to build a libfuzzer binary.
	// We also link in an additional object to ensure that non-fatal
	// sanitizer findings still have an input attached.
	// See src/dumper.c for details.
	var fuzzTestLdflags []string
	if runtime.GOOS != "darwin" {
		// Redirect calls to __sanitizer_set_death_callback to our implemented
		// __wrap__sanitizer_set_death_callback (in dumper.c/.cpp) to modify
		// the behavior of the original libfuzzer function
		fuzzTestLdflags = append(fuzzTestLdflags, "-Wl,--wrap=__sanitizer_set_death_callback")
	}
	fuzzTestLdflags = append(fuzzTestLdflags,
		// Build with instrumentation for Fuzzing
		"-fsanitize=fuzzer",
		// Path to the dumper of CI Fuzz which ensures that non-fatal sanitizer
		// findings still have an input attached
		filepath.Join(b.buildDir, "dumper.o"))
	b.env, err = setEnvWithDebugMsg(b.env, EnvFuzzTestLDFlags, strings.Join(fuzzTestLdflags, " "))
	if err != nil {
		return err
	}

	return nil
}

func (b *Builder) setCoverageEnv() error {
	var err error

	b.env, err = setEnvWithDebugMsg(b.env, EnvBuildStep, "coverage")
	if err != nil {
		return err
	}

	// Set CFLAGS and CXXFLAGS. Note that these flags must not contain
	// spaces, because the environment variables are space separated.
	//
	// Note: Keep in sync with share/cmake/cifuzz-functions.cmake
	clangVersion, err := dependencies.Version(dependencies.Clang, b.ProjectDir)
	if err != nil {
		log.Warnf("Failed to determine version of clang: %v", err)
	}
	cflags := build.CoverageCFlags(clangVersion)

	b.env, err = setEnvWithDebugMsg(b.env, "CFLAGS", strings.Join(cflags, " "))
	if err != nil {
		return err
	}
	b.env, err = setEnvWithDebugMsg(b.env, "CXXFLAGS", strings.Join(cflags, " "))
	if err != nil {
		return err
	}

	ldflags := []string{
		// ----- Flags used to link in coverage runtime -----
		// Generate instrumented code to collect execution counts
		"-fprofile-instr-generate",
	}
	b.env, err = setEnvWithDebugMsg(b.env, "LDFLAGS", strings.Join(ldflags, " "))
	if err != nil {
		return err
	}

	// Users should pass the environment variable FUZZ_TEST_CFLAGS or
	// FUZZ_TEST_CXXFLAGS to the compiler command building the fuzz test.
	cifuzzIncludePath, err := b.RunfilesFinder.CIFuzzIncludePath()
	if err != nil {
		return err
	}
	// -I adds the include directory to the list of directories
	// to be searched for header files
	fuzzTestCFlags := []string{fmt.Sprintf("-I%s", cifuzzIncludePath)}
	b.env, err = setEnvWithDebugMsg(b.env, EnvFuzzTestCFlags, strings.Join(fuzzTestCFlags, " "))
	if err != nil {
		return err
	}
	b.env, err = setEnvWithDebugMsg(b.env, EnvFuzzTestCXXFlags, strings.Join(fuzzTestCFlags, " "))
	if err != nil {
		return err
	}

	// Users should pass the environment variable FUZZ_TEST_LDFLAGS to
	// the linker command building the fuzz test. We use it to link in libFuzzer
	// in coverage builds to use its crash-resistant merge feature.
	b.env, err = setEnvWithDebugMsg(b.env, EnvFuzzTestLDFlags, "-fsanitize=fuzzer")
	if err != nil {
		return err
	}

	return nil
}

func (b *Builder) findFuzzTestExecutable(fuzzTest string) (string, error) {
	if exists, _ := fileutil.Exists(fuzzTest); exists {
		absPath, err := filepath.Abs(fuzzTest)
		if err != nil {
			return "", errors.WithStack(err)
		}
		log.Debugf("Fuzz test executable found at %s", absPath)
		return absPath, nil
	}

	var executable string
	err := filepath.Walk(".", func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return errors.WithStack(err)
		}
		if info.IsDir() {
			return nil
		}
		if runtime.GOOS == "windows" {
			if info.Name() == fuzzTest+".exe" {
				executable = path
			}
		} else {
			// As a heuristic, verify that the executable candidate has some
			// executable bit set - it may not be sufficient to actually execute
			// it as the current user.
			if info.Name() == fuzzTest && (info.Mode()&0111 != 0) {
				executable = path
			}
		}
		return nil
	})
	if err != nil {
		return "", errors.WithMessage(err, "Failed to search through project to find fuzz test executable")
	}
	// No executable was found, we handle this error in the caller
	if executable == "" {
		return "", nil
	}
	absPath, err := filepath.Abs(executable)
	if err != nil {
		return "", errors.WithStack(err)
	}
	log.Debugf("Fuzz test executable found at %s", absPath)
	return absPath, nil
}

func setEnvWithDebugMsg(env []string, key, value string) ([]string, error) {
	log.Debugf("Setting ENV: %s=%s", key, value)
	env, err := envutil.Setenv(env, key, value)
	if err != nil {
		return nil, err
	}

	return env, nil
}
