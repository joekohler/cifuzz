package run

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/pkg/errors"
	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"golang.org/x/sync/errgroup"

	"code-intelligence.com/cifuzz/internal/api"
	"code-intelligence.com/cifuzz/internal/build"
	"code-intelligence.com/cifuzz/internal/build/bazel"
	"code-intelligence.com/cifuzz/internal/build/cmake"
	"code-intelligence.com/cifuzz/internal/build/gradle"
	"code-intelligence.com/cifuzz/internal/build/maven"
	"code-intelligence.com/cifuzz/internal/build/other"
	"code-intelligence.com/cifuzz/internal/cmd/run/reporthandler"
	"code-intelligence.com/cifuzz/internal/cmdutils"
	"code-intelligence.com/cifuzz/internal/cmdutils/auth"
	"code-intelligence.com/cifuzz/internal/cmdutils/login"
	"code-intelligence.com/cifuzz/internal/cmdutils/resolve"
	"code-intelligence.com/cifuzz/internal/completion"
	"code-intelligence.com/cifuzz/internal/config"
	"code-intelligence.com/cifuzz/internal/ldd"
	"code-intelligence.com/cifuzz/internal/tokenstorage"
	"code-intelligence.com/cifuzz/pkg/cicheck"
	"code-intelligence.com/cifuzz/pkg/dependencies"
	"code-intelligence.com/cifuzz/pkg/dialog"
	"code-intelligence.com/cifuzz/pkg/finding"
	"code-intelligence.com/cifuzz/pkg/log"
	"code-intelligence.com/cifuzz/pkg/report"
	"code-intelligence.com/cifuzz/pkg/runner/jazzer"
	"code-intelligence.com/cifuzz/pkg/runner/libfuzzer"
	"code-intelligence.com/cifuzz/util/fileutil"
	"code-intelligence.com/cifuzz/util/stringutil"
)

type runOptions struct {
	BuildSystem           string        `mapstructure:"build-system"`
	BuildCommand          string        `mapstructure:"build-command"`
	CleanCommand          string        `mapstructure:"clean-command"`
	NumBuildJobs          uint          `mapstructure:"build-jobs"`
	Dictionary            string        `mapstructure:"dict"`
	EngineArgs            []string      `mapstructure:"engine-args"`
	SeedCorpusDirs        []string      `mapstructure:"seed-corpus-dirs"`
	Timeout               time.Duration `mapstructure:"timeout"`
	Interactive           bool          `mapstructure:"interactive"`
	Server                string        `mapstructure:"server"`
	Project               string        `mapstructure:"project"`
	UseSandbox            bool          `mapstructure:"use-sandbox"`
	PrintJSON             bool          `mapstructure:"print-json"`
	BuildOnly             bool          `mapstructure:"build-only"`
	ResolveSourceFilePath bool

	ProjectDir   string
	fuzzTest     string
	targetMethod string
	argsToPass   []string

	buildStdout io.Writer
	buildStderr io.Writer
}

func (opts *runOptions) validate() error {
	var err error

	opts.SeedCorpusDirs, err = cmdutils.ValidateSeedCorpusDirs(opts.SeedCorpusDirs)
	if err != nil {
		log.Error(err, err.Error())
		return cmdutils.ErrSilent
	}

	if opts.Dictionary != "" {
		// Check if the dictionary exists and can be accessed
		_, err = os.Stat(opts.Dictionary)
		if err != nil {
			err = errors.WithStack(err)
			log.Error(err, err.Error())
			return cmdutils.ErrSilent
		}
	}

	if opts.BuildSystem == "" {
		opts.BuildSystem, err = config.DetermineBuildSystem(opts.ProjectDir)
		if err != nil {
			return err
		}
	} else {
		err = config.ValidateBuildSystem(opts.BuildSystem)
		if err != nil {
			return err
		}
	}

	// To build with other build systems, a build command must be provided
	if opts.BuildSystem == config.BuildSystemOther && opts.BuildCommand == "" {
		msg := "Flag \"build-command\" must be set when using build system type \"other\""
		return cmdutils.WrapIncorrectUsageError(errors.New(msg))
	}

	if opts.Timeout != 0 && opts.Timeout < time.Second {
		msg := fmt.Sprintf("invalid argument %q for \"--timeout\" flag: timeout can't be less than a second", opts.Timeout)
		return cmdutils.WrapIncorrectUsageError(errors.New(msg))
	}

	return nil
}

type runCmd struct {
	*cobra.Command

	opts      *runOptions
	apiClient *api.APIClient

	reportHandler *reporthandler.ReportHandler
	tempDir       string
}

type runner interface {
	Run(context.Context) error
	Cleanup(context.Context)
}

func New() *cobra.Command {
	opts := &runOptions{}
	var bindFlags func()

	cmd := &cobra.Command{
		Use:   "run [flags] <fuzz test> [--] [<build system arg>...] ",
		Short: "Build and run a fuzz test",
		Long: `This command builds and executes a fuzz test. The usage of this command
depends on the build system configured for the project.

` + pterm.Style{pterm.Reset, pterm.Bold}.Sprint("CMake") + `
  <fuzz test> is the name of the fuzz test defined in the add_fuzz_test
  command in your CMakeLists.txt.

  Command completion for the <fuzz test> argument is supported when the
  fuzz test was built before or after running 'cifuzz reload'.

  The --build-command flag is ignored.

  Additional CMake arguments can be passed after a "--". For example:

    cifuzz run my_fuzz_test -- -G Ninja

  The inputs found in the directory

    <fuzz test>_inputs

  are used as a starting point for the fuzzing run.

` + pterm.Style{pterm.Reset, pterm.Bold}.Sprint("Bazel") + `
  <fuzz test> is the name of the cc_fuzz_test target as defined in your
  BUILD file, either as a relative or absolute Bazel label.

  Command completion for the <fuzz test> argument is supported.

  The --build-command flag is ignored.

  Additional Bazel arguments can be passed after a "--". For example:

    cifuzz run my_fuzz_test -- --sandbox_debug

  The inputs found in the directory

    <fuzz test>_inputs

  are used as a starting point for the fuzzing run.

` + pterm.Style{pterm.Reset, pterm.Bold}.Sprint("Maven/Gradle") + `
  <fuzz test> is the name of the class containing the fuzz test.

  Command completion for the <fuzz test> argument is supported.

  The --build-command flag is ignored.

  The inputs found in the directory

    src/test/resources/.../<fuzz test>Inputs

  are used as a starting point for the fuzzing run.

` + pterm.Style{pterm.Reset, pterm.Bold}.Sprint("Other build systems") + `
  <fuzz test> is either the path or basename of the fuzz test executable
  created by the build command. If it's the basename, it will be searched
  for recursively in the current working directory.

  A command which builds the fuzz test executable must be provided via
  the --build-command flag or the build-command setting in cifuzz.yaml.

  The value specified for <fuzz test> is made available to the build
  command in the FUZZ_TEST environment variable. For example:

    echo "build-command: make clean && make \$FUZZ_TEST" >> cifuzz.yaml
    cifuzz run my_fuzz_test

  To avoid cleaning the build artifacts after building each fuzz test, you
  can provide a clean command using the --clean-command flag or specifying
  the "clean-command" option in cifuzz.yaml. The clean command is then
  executed once before building the fuzz tests.

  The inputs found in the directory

    <fuzz test>_inputs

  are used as a starting point for the fuzzing run.

`,
		ValidArgsFunction: completion.ValidFuzzTests,
		PreRunE: func(cmd *cobra.Command, args []string) error {
			// Bind viper keys to flags. We can't do this in the New
			// function, because that would re-bind viper keys which
			// were bound to the flags of other commands before.
			bindFlags()

			// Check correct number of fuzz test args (exactly one)
			var lenFuzzTestArgs int
			var argsToPass []string
			if cmd.ArgsLenAtDash() != -1 {
				lenFuzzTestArgs = cmd.ArgsLenAtDash()
				argsToPass = args[cmd.ArgsLenAtDash():]
				args = args[:cmd.ArgsLenAtDash()]
			} else {
				lenFuzzTestArgs = len(args)
			}
			if lenFuzzTestArgs != 1 {
				msg := fmt.Sprintf("Exactly one <fuzz test> argument must be provided, got %d", lenFuzzTestArgs)
				return cmdutils.WrapIncorrectUsageError(errors.New(msg))
			}

			err := config.FindAndParseProjectConfig(opts)
			if err != nil {
				log.Errorf(err, "Failed to parse cifuzz.yaml: %v", err.Error())
				return cmdutils.WrapSilentError(err)
			}

			// Check if the fuzz test is a method of a class
			// And remove method from fuzz test argument
			if strings.Contains(args[0], "::") {
				split := strings.Split(args[0], "::")
				args[0], opts.targetMethod = split[0], split[1]
			}

			fuzzTests, err := resolve.FuzzTestArgument(opts.ResolveSourceFilePath, args, opts.BuildSystem, opts.ProjectDir)
			if err != nil {
				log.Error(err)
				return cmdutils.WrapSilentError(err)
			}
			opts.fuzzTest = fuzzTests[0]

			opts.argsToPass = argsToPass

			opts.buildStdout = cmd.OutOrStdout()
			opts.buildStderr = cmd.OutOrStderr()
			if cmdutils.ShouldLogBuildToFile() {
				opts.buildStdout, err = cmdutils.BuildOutputToFile(opts.ProjectDir, []string{opts.fuzzTest})
				if err != nil {
					log.Errorf(err, "Failed to setup logging: %v", err.Error())
					return cmdutils.WrapSilentError(err)
				}
				opts.buildStderr = opts.buildStdout
			}

			return opts.validate()
		},
		RunE: func(c *cobra.Command, args []string) error {
			// Check if the server option is a valid URL
			err := api.ValidateURL(opts.Server)
			if err != nil {
				// See if prefixing https:// makes it a valid URL
				err = api.ValidateURL("https://" + opts.Server)
				if err != nil {
					log.Error(err, fmt.Sprintf("server %q is not a valid URL", opts.Server))
				}
				opts.Server = "https://" + opts.Server
			}

			// normalize server URL
			url, err := url.JoinPath(opts.Server)
			if err != nil {
				return err
			}
			opts.Server = url

			cmd := runCmd{Command: c, opts: opts}
			cmd.apiClient = api.NewClient(opts.Server, cmd.Command.Root().Version)
			return cmd.run()
		},
	}

	// Note: If a flag should be configurable via cifuzz.yaml as well,
	// bind it to viper in the PreRunE function.
	funcs := []func(cmd *cobra.Command) func(){
		cmdutils.AddBuildCommandFlag,
		cmdutils.AddCleanCommandFlag,
		cmdutils.AddBuildJobsFlag,
		cmdutils.AddBuildOnlyFlag,
		cmdutils.AddDictFlag,
		cmdutils.AddEngineArgFlag,
		cmdutils.AddInteractiveFlag,
		cmdutils.AddPrintJSONFlag,
		cmdutils.AddProjectFlag,
		cmdutils.AddProjectDirFlag,
		cmdutils.AddSeedCorpusFlag,
		cmdutils.AddServerFlag,
		cmdutils.AddTimeoutFlag,
		cmdutils.AddUseSandboxFlag,
		cmdutils.AddResolveSourceFileFlag,
	}
	bindFlags = cmdutils.AddFlags(cmd, funcs...)
	return cmd
}

func (c *runCmd) run() error {
	err := c.checkDependencies()
	if err != nil {
		return err
	}

	authenticatedUser := false

	var errorDetails *[]finding.ErrorDetails

	authenticatedUser, err = c.setupSync()
	if err != nil {
		return err
	}

	if authenticatedUser {
		errorDetails, err = c.errorDetails()
		if err != nil {
			return err
		}
	}

	// Create a temporary directory which the builder can use to create
	// temporary files
	c.tempDir, err = os.MkdirTemp("", "cifuzz-run-")
	if err != nil {
		return errors.WithStack(err)
	}
	defer fileutil.Cleanup(c.tempDir)

	buildResult, err := c.buildFuzzTest()
	if err != nil {
		var execErr *cmdutils.ExecError
		if errors.As(err, &execErr) {
			// It is expected that some commands might fail due to user
			// configuration so we print the error without the stack trace
			// (in non-verbose mode) and silence it
			log.Error(err)
			return cmdutils.ErrSilent
		}
		return err
	}

	if c.opts.BuildOnly {
		return nil
	}

	// Initialize the report handler. Only do this right before we start
	// the fuzz test, because this is storing a timestamp which is used
	// to figure out how long the fuzzing run is running.
	c.reportHandler, err = reporthandler.NewReportHandler(
		c.opts.fuzzTest,
		&reporthandler.ReportHandlerOptions{
			ProjectDir:    c.opts.ProjectDir,
			SeedCorpusDir: buildResult.SeedCorpus,
			PrintJSON:     c.opts.PrintJSON,
		})
	if err != nil {
		return err
	}
	c.reportHandler.ErrorDetails = errorDetails

	err = c.runFuzzTest(buildResult)
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && c.opts.UseSandbox {
			return cmdutils.WrapCouldBeSandboxError(err)
		}
		return err
	}

	c.reportHandler.PrintCrashingInputNote()

	err = c.printFinalMetrics(buildResult.GeneratedCorpus, buildResult.SeedCorpus)
	if err != nil {
		return err
	}

	// We need this check, otherwise we might hang forever in CI
	if c.opts.Project == "" && !c.opts.Interactive {
		log.Info("Skipping upload of findings because no project was specified and running in non-interactive mode.")
		return nil
	}

	// check if there are findings that should be uploaded
	if authenticatedUser && len(c.reportHandler.Findings) > 0 {
		err = c.uploadFindings(c.opts.fuzzTest, c.reportHandler.FirstMetrics, c.reportHandler.LastMetrics, c.opts.NumBuildJobs)
		if err != nil {
			return err
		}
	}

	return nil
}

func (c *runCmd) buildFuzzTest() (*build.Result, error) {
	var err error

	if cmdutils.ShouldLogBuildToFile() {
		log.CreateCurrentProgressSpinner(nil, log.BuildInProgressMsg)
		defer func(err *error) {
			if *err != nil {
				log.StopCurrentProgressSpinner(log.GetPtermErrorStyle(), log.BuildInProgressErrorMsg)
				printErr := cmdutils.PrintBuildLogOnStdout()
				if printErr != nil {
					log.Error(printErr)
				}
			} else {
				log.StopCurrentProgressSpinner(log.GetPtermSuccessStyle(), log.BuildInProgressSuccessMsg)
				log.Info(cmdutils.GetMsgPathToBuildLog())
			}
		}(&err)
	}

	// TODO: Do not hardcode these values.
	sanitizers := []string{"address", "undefined"}

	if runtime.GOOS == "windows" &&
		(c.opts.BuildSystem != config.BuildSystemCMake &&
			c.opts.BuildSystem != config.BuildSystemMaven &&
			c.opts.BuildSystem != config.BuildSystemGradle) {

		return nil, errors.New("Build system unsupported on Windows")
	}

	switch c.opts.BuildSystem {
	case config.BuildSystemBazel:
		// The cc_fuzz_test rule defines multiple bazel targets: If the
		// name is "foo", it defines the targets "foo", "foo_bin", and
		// others. We need to run the "foo_bin" target but want to
		// allow users to specify either "foo" or "foo_bin", so we check
		// if the fuzz test name appended with "_bin" is a valid target
		// and use that in that case
		cmd := exec.Command("bazel", "query", c.opts.fuzzTest+"_bin")
		err = cmd.Run()
		if err == nil {
			c.opts.fuzzTest += "_bin"
		}

		var builder *bazel.Builder
		builder, err = bazel.NewBuilder(&bazel.BuilderOptions{
			ProjectDir: c.opts.ProjectDir,
			Args:       c.opts.argsToPass,
			NumJobs:    c.opts.NumBuildJobs,
			Stdout:     c.opts.buildStdout,
			Stderr:     c.opts.buildStderr,
			TempDir:    c.tempDir,
			Verbose:    viper.GetBool("verbose"),
		})
		if err != nil {
			return nil, err
		}

		var buildResults []*build.Result
		buildResults, err = builder.BuildForRun([]string{c.opts.fuzzTest})
		if err != nil {
			return nil, err
		}
		return buildResults[0], nil

	case config.BuildSystemCMake:
		var builder *cmake.Builder
		builder, err = cmake.NewBuilder(&cmake.BuilderOptions{
			ProjectDir: c.opts.ProjectDir,
			Args:       c.opts.argsToPass,
			Sanitizers: sanitizers,
			Parallel: cmake.ParallelOptions{
				Enabled: viper.IsSet("build-jobs"),
				NumJobs: c.opts.NumBuildJobs,
			},
			Stdout:    c.opts.buildStdout,
			Stderr:    c.opts.buildStderr,
			BuildOnly: c.opts.BuildOnly,
		})
		if err != nil {
			return nil, err
		}
		err = builder.Configure()
		if err != nil {
			return nil, err
		}

		var buildResults []*build.Result
		buildResults, err = builder.Build([]string{c.opts.fuzzTest})
		if err != nil {
			return nil, err
		}

		if c.opts.BuildOnly {
			return nil, nil
		}
		return buildResults[0], nil

	case config.BuildSystemMaven:
		if len(c.opts.argsToPass) > 0 {
			log.Warnf("Passing additional arguments is not supported for Maven.\n"+
				"These arguments are ignored: %s", strings.Join(c.opts.argsToPass, " "))
		}

		var builder *maven.Builder
		builder, err = maven.NewBuilder(&maven.BuilderOptions{
			ProjectDir: c.opts.ProjectDir,
			Parallel: maven.ParallelOptions{
				Enabled: viper.IsSet("build-jobs"),
				NumJobs: c.opts.NumBuildJobs,
			},
			Stdout: c.opts.buildStdout,
			Stderr: c.opts.buildStderr,
		})
		if err != nil {
			return nil, err
		}

		var buildResult *build.Result
		buildResult, err = builder.Build(c.opts.fuzzTest)
		if err != nil {
			return nil, err
		}
		return buildResult, err

	case config.BuildSystemGradle:
		if len(c.opts.argsToPass) > 0 {
			log.Warnf("Passing additional arguments is not supported for Gradle.\n"+
				"These arguments are ignored: %s", strings.Join(c.opts.argsToPass, " "))
		}

		var builder *gradle.Builder
		builder, err = gradle.NewBuilder(&gradle.BuilderOptions{
			ProjectDir: c.opts.ProjectDir,
			Parallel: gradle.ParallelOptions{
				Enabled: viper.IsSet("build-jobs"),
				NumJobs: c.opts.NumBuildJobs,
			},
			Stdout: c.opts.buildStdout,
			Stderr: c.opts.buildStderr,
		})
		if err != nil {
			return nil, err
		}

		var buildResult *build.Result
		buildResult, err = builder.Build(c.opts.fuzzTest)
		if err != nil {
			return nil, err
		}
		return buildResult, err
	case config.BuildSystemOther:
		if len(c.opts.argsToPass) > 0 {
			log.Warnf("Passing additional arguments is not supported for build system type \"other\".\n"+
				"These arguments are ignored: %s", strings.Join(c.opts.argsToPass, " "))
		}

		var builder *other.Builder
		builder, err = other.NewBuilder(&other.BuilderOptions{
			ProjectDir:   c.opts.ProjectDir,
			BuildCommand: c.opts.BuildCommand,
			CleanCommand: c.opts.CleanCommand,
			Sanitizers:   sanitizers,
			Stdout:       c.opts.buildStdout,
			Stderr:       c.opts.buildStderr,
		})
		if err != nil {
			return nil, err
		}

		err := builder.Clean()
		if err != nil {
			return nil, err
		}

		var buildResult *build.Result
		buildResult, err = builder.Build(c.opts.fuzzTest)
		if err != nil {
			return nil, err
		}
		return buildResult, nil
	}

	return nil, errors.Errorf("Unsupported build system \"%s\"", c.opts.BuildSystem)
}

func (c *runCmd) runFuzzTest(buildResult *build.Result) error {
	if c.opts.targetMethod != "" {
		log.Infof("Running %s", pterm.Style{pterm.Reset, pterm.FgLightBlue}.Sprintf(c.opts.fuzzTest+"::"+c.opts.targetMethod))
	} else {
		log.Infof("Running %s", pterm.Style{pterm.Reset, pterm.FgLightBlue}.Sprintf(c.opts.fuzzTest))
	}

	if buildResult.Executable != "" {
		log.Debugf("Executable: %s", buildResult.Executable)
	}

	err := os.MkdirAll(buildResult.GeneratedCorpus, 0o755)
	if err != nil {
		return errors.WithStack(err)
	}
	log.Infof("Storing generated corpus in %s", fileutil.PrettifyPath(buildResult.GeneratedCorpus))

	// Use user-specified seed corpus dirs (if any) and the default seed
	// corpus (if it exists)
	seedCorpusDirs := c.opts.SeedCorpusDirs
	exists, err := fileutil.Exists(buildResult.SeedCorpus)
	if err != nil {
		return err
	}
	if exists {
		seedCorpusDirs = append(seedCorpusDirs, buildResult.SeedCorpus)
	}

	// Ensure that symlinks are resolved to be able to add minijail
	// bindings for the corpus dirs.
	buildResult.GeneratedCorpus, err = filepath.EvalSymlinks(buildResult.GeneratedCorpus)
	if err != nil {
		return errors.WithStack(err)
	}
	for i, dir := range seedCorpusDirs {
		seedCorpusDirs[i], err = filepath.EvalSymlinks(dir)
		if err != nil {
			return errors.WithStack(err)
		}
	}

	if c.opts.BuildSystem == config.BuildSystemBazel {
		// The install base directory contains e.g. the script generated
		// by bazel via --script_path and must therefore be accessible
		// inside the sandbox.
		cmd := exec.Command("bazel", "info", "install_base")
		err = cmd.Run()
		if err != nil {
			// It's expected that bazel might fail due to user configuration,
			// so we print the error without the stack trace.
			err = cmdutils.WrapExecError(errors.WithStack(err), cmd)
			log.Error(err)
			return cmdutils.ErrSilent
		}
	}

	var libraryPaths []string
	if runtime.GOOS != "windows" && buildResult.Executable != "" {
		libraryPaths, err = ldd.LibraryPaths(buildResult.Executable)
		if err != nil {
			return errors.WithStack(err)
		}
	}

	runnerOpts := &libfuzzer.RunnerOptions{
		Dictionary:         c.opts.Dictionary,
		EngineArgs:         c.opts.EngineArgs,
		EnvVars:            []string{"NO_CIFUZZ=1"},
		FuzzTarget:         buildResult.Executable,
		LibraryDirs:        libraryPaths,
		GeneratedCorpusDir: buildResult.GeneratedCorpus,
		KeepColor:          !c.opts.PrintJSON,
		ProjectDir:         c.opts.ProjectDir,
		ReadOnlyBindings:   []string{buildResult.BuildDir},
		ReportHandler:      c.reportHandler,
		SeedCorpusDirs:     seedCorpusDirs,
		Timeout:            c.opts.Timeout,
		UseMinijail:        c.opts.UseSandbox,
		Verbose:            viper.GetBool("verbose"),
	}

	var runner runner

	switch c.opts.BuildSystem {
	case config.BuildSystemCMake, config.BuildSystemBazel, config.BuildSystemOther:
		runner = libfuzzer.NewRunner(runnerOpts)
	case config.BuildSystemMaven, config.BuildSystemGradle:
		runnerOpts := &jazzer.RunnerOptions{
			TargetClass:      c.opts.fuzzTest,
			TargetMethod:     c.opts.targetMethod,
			ClassPaths:       buildResult.RuntimeDeps,
			LibfuzzerOptions: runnerOpts,
		}
		runner = jazzer.NewRunner(runnerOpts)
	}

	return executeRunner(runner)
}

func (c *runCmd) printFinalMetrics(generatedCorpus, seedCorpus string) error {
	numCorpusEntries, err := countCorpusEntries(append(c.opts.SeedCorpusDirs, generatedCorpus, seedCorpus))
	if err != nil {
		return err
	}

	return c.reportHandler.PrintFinalMetrics(numCorpusEntries)
}

func (c *runCmd) checkDependencies() error {
	var deps []dependencies.Key
	switch c.opts.BuildSystem {
	case config.BuildSystemCMake:
		switch runtime.GOOS {
		case "linux", "darwin":
			deps = []dependencies.Key{
				dependencies.Clang,
				dependencies.CMake,
				dependencies.LLVMSymbolizer,
			}
		case "windows":
			deps = []dependencies.Key{
				dependencies.VisualStudio,
			}
		}
	case config.BuildSystemMaven:
		deps = []dependencies.Key{
			dependencies.Java,
			dependencies.Maven,
		}
	case config.BuildSystemGradle:
		// First check if gradle wrapper exists and check for gradle in path otherwise
		wrapper, err := gradle.FindGradleWrapper(c.opts.ProjectDir)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		// TODO: Do we really not want to check that Java is installed
		// too when we found the gradle wrapper?
		if wrapper != "" {
			return nil
		}

		deps = []dependencies.Key{
			dependencies.Java,
			dependencies.Gradle,
		}
	case config.BuildSystemOther:
		switch runtime.GOOS {
		case "linux", "darwin":
			deps = []dependencies.Key{
				dependencies.Clang,
				dependencies.LLVMSymbolizer,
			}
		case "windows":
			deps = []dependencies.Key{
				dependencies.VisualStudio,
			}
		}
	case config.BuildSystemBazel:
		// All dependencies are managed via bazel but it should be checked
		// that the correct bazel version is installed
		deps = []dependencies.Key{
			dependencies.Bazel,
		}
	default:
		return errors.Errorf("Unsupported build system \"%s\"", c.opts.BuildSystem)
	}

	depsErr := dependencies.Check(deps)
	if depsErr != nil {
		log.Error(depsErr)
		return cmdutils.WrapSilentError(depsErr)
	}
	return nil
}

// setupSync initiates user dialog and returns if findings should be synced
func (c *runCmd) setupSync() (bool, error) {
	interactive := viper.GetBool("interactive")
	if cicheck.IsCIEnvironment() {
		interactive = false
	}
	var willSync bool

	// Check if the server option is a valid URL
	err := api.ValidateURL(c.opts.Server)
	if err != nil {
		// See if prefixing https:// makes it a valid URL
		err = api.ValidateURL("https://" + c.opts.Server)
		if err != nil {
			log.Error(err, fmt.Sprintf("server %q is not a valid URL", c.opts.Server))
		}
		c.opts.Server = "https://" + c.opts.Server
	}

	// normalize server URL
	url, err := url.JoinPath(c.opts.Server)
	if err != nil {
		return false, cmdutils.WrapSilentError(err)
	}
	c.opts.Server = url

	authenticated, err := auth.GetAuthStatus(c.opts.Server)
	if err != nil {
		var connErr *api.ConnectionError
		if errors.As(err, &connErr) {
			log.Warn("Connection to API failed. Skipping sync.")
			log.Debugf("Connection error: %s (continuing gracefully)", connErr)
			return false, nil
		} else {
			fmt.Println("AUTH STATUS CHECK ERROR")
			return false, cmdutils.WrapSilentError(err)
		}
	}

	if authenticated {
		willSync = true
		log.Infof(`✓ You are authenticated.
Your results will be synced to the remote fuzzing server at %s`, c.opts.Server)
	} else if !interactive {
		willSync = false
		log.Warn(`You are not authenticated with a remote fuzzing server.
Your results will not be synced to a remote fuzzing server.`)
	}

	if interactive && !authenticated {
		// establish server connection to check user auth
		willSync, err = auth.ShowServerConnectionDialog(c.opts.Server)
		if err != nil {
			var connErr *api.ConnectionError
			if errors.As(err, &connErr) {
				log.Warn("Connection to API failed. Skipping sync.")
				log.Debugf("Connection error: %v (continuing gracefully)", connErr)
				return false, nil
			} else {
				return false, cmdutils.WrapSilentError(err)
			}
		}
	}
	return willSync, nil
}

func (c *runCmd) errorDetails() (*[]finding.ErrorDetails, error) {
	token := login.GetToken(c.opts.Server)
	if token == "" {
		return nil, errors.New("No access token found")
	}

	errorDetails, err := c.apiClient.GetErrorDetails(token)
	if err != nil {
		var connErr *api.ConnectionError
		if !errors.As(err, &connErr) {
			return nil, err
		} else {
			log.Warn("Connection to API failed. Skipping error details.")
			log.Debugf("Connection error: %v (continiung gracefully)", connErr)
			return nil, nil
		}
	}

	return &errorDetails, nil
}

func (c *runCmd) uploadFindings(fuzzTarget string, firstMetrics *report.FuzzingMetric, lastMetrics *report.FuzzingMetric, numBuildJobs uint) error {
	token := login.GetToken(c.opts.Server)
	if token == "" {
		return errors.New("No access token found")
	}

	projects, err := c.apiClient.ListProjects(token)
	if err != nil {
		return err
	}

	project := c.opts.Project
	if project == "" {
		// ask user to select project
		project, err = c.selectProject(projects)
		if err != nil {
			return cmdutils.WrapSilentError(err)
		}

		// this will ask users via a y/N prompt if they want to persist the
		// project choice
		err = dialog.AskToPersistProjectChoice(project)
		if err != nil {
			return cmdutils.WrapSilentError(err)
		}
	} else {
		// check if project exists on server
		found := false
		project = "projects/" + project
		for _, p := range projects {
			if p.Name == project {
				found = true
				break
			}
		}

		if !found {
			message := fmt.Sprintf(`Project %s does not exist on server %s.
Findings have *not* been uploaded. Please check the 'project' entry in your cifuzz.yml.`, project, c.opts.Server)
			log.Error(errors.New(message))
			err = errors.Errorf(message)
			return cmdutils.WrapSilentError(err)
		}
	}

	// create campaign run on server for selected project
	campaignRunName, fuzzingRunName, err := c.apiClient.CreateCampaignRun(project, token, fuzzTarget, firstMetrics, lastMetrics, numBuildJobs)
	if err != nil {
		return err
	}

	// upload findings
	for _, finding := range c.reportHandler.Findings {
		err = c.apiClient.UploadFinding(project, fuzzTarget, campaignRunName, fuzzingRunName, finding, token)
		if err != nil {
			return err
		}
	}
	log.Notef("Uploaded %d findings to CI Fuzz Server at: %s", len(c.reportHandler.Findings), c.opts.Server)
	log.Infof("You can view the findings at %s/dashboard/%s/findings?origin=cli", c.opts.Server, campaignRunName)

	return nil
}

func executeRunner(runner runner) error {
	// Handle cleanup (terminating the fuzzer process) when receiving
	// termination signals
	signalHandlerCtx, cancelSignalHandler := context.WithCancel(context.Background())
	routines, routinesCtx := errgroup.WithContext(signalHandlerCtx)
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, os.Interrupt, syscall.SIGTERM, syscall.SIGINT, syscall.SIGQUIT)
	var signalErr error
	routines.Go(func() error {
		select {
		case <-routinesCtx.Done():
			return nil
		case s := <-sigs:
			log.Warnf("Received %s", s.String())
			signalErr = cmdutils.NewSignalError(s.(syscall.Signal))
			runner.Cleanup(routinesCtx)
			return signalErr
		}
	})

	// Run the fuzzer
	routines.Go(func() error {
		defer cancelSignalHandler()
		return runner.Run(routinesCtx)
	})

	err := routines.Wait()
	// We use a separate variable to pass signal errors, because when
	// a signal was received, the first goroutine terminates the second
	// one, resulting in a race of which returns an error first. In that
	// case, we always want to print the signal error, not the
	// "Unexpected exit code" error from the runner.
	if signalErr != nil {
		log.Error(signalErr, signalErr.Error())
		return cmdutils.WrapSilentError(signalErr)
	}

	var execErr *cmdutils.ExecError
	if errors.As(err, &execErr) {
		// It's expected that libFuzzer might fail due to user
		// configuration, so we print the error without the stack trace
		log.Error(err)
		return cmdutils.WrapSilentError(err)
	}

	return err
}

func countCorpusEntries(seedCorpusDirs []string) (uint, error) {
	var numSeeds uint
	for _, dir := range seedCorpusDirs {
		var seedsInDir uint
		err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				return nil
			}
			info, err := d.Info()
			if err != nil {
				return err
			}
			// Don't count empty files, same as libFuzzer
			if info.Size() != 0 {
				seedsInDir += 1
			}
			return nil
		})
		// Don't fail if the seed corpus dir doesn't exist
		if os.IsNotExist(err) {
			return 0, nil
		}
		if err != nil {
			return 0, errors.WithStack(err)
		}
		numSeeds += seedsInDir
	}
	return numSeeds, nil
}

func (c *runCmd) selectProject(projects []*api.Project) (string, error) {
	// Let the user select a project
	var displayNames []string
	var names []string
	for _, p := range projects {
		displayNames = append(displayNames, p.DisplayName)
		names = append(names, p.Name)
	}
	maxLen := stringutil.MaxLen(displayNames)
	items := map[string]string{}
	for i := range displayNames {
		key := fmt.Sprintf("%-*s [%s]", maxLen, displayNames[i], strings.TrimPrefix(names[i], "projects/"))
		items[key] = names[i]
	}

	// add option to create a new project
	items["<Create a new project>"] = "<<new>>"

	projectName, err := dialog.Select("Select the project you want to upload your findings to", items, true)
	if err != nil {
		return "", errors.WithStack(err)
	}

	if projectName == "<<new>>" {
		// ask user for project name
		projectName, err = dialog.Input("Enter the name of the project you want to create")
		if err != nil {
			return "", errors.WithStack(err)
		}

		token := tokenstorage.Get(c.opts.Server)
		project, err := c.apiClient.CreateProject(projectName, token)
		if err != nil {
			return "", errors.WithStack(err)
		}

		return project.Name, nil
	}

	return projectName, nil
}
