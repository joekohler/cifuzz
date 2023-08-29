package run

import (
	"context"
	"fmt"
	"io"
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
	"golang.org/x/term"

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
	"code-intelligence.com/cifuzz/internal/cmdutils/logging"
	"code-intelligence.com/cifuzz/internal/cmdutils/resolve"
	"code-intelligence.com/cifuzz/internal/completion"
	"code-intelligence.com/cifuzz/internal/config"
	"code-intelligence.com/cifuzz/internal/ldd"
	"code-intelligence.com/cifuzz/internal/tokenstorage"
	"code-intelligence.com/cifuzz/pkg/dependencies"
	"code-intelligence.com/cifuzz/pkg/dialog"
	"code-intelligence.com/cifuzz/pkg/finding"
	"code-intelligence.com/cifuzz/pkg/log"
	"code-intelligence.com/cifuzz/pkg/messaging"
	"code-intelligence.com/cifuzz/pkg/report"
	"code-intelligence.com/cifuzz/pkg/runner/jazzer"
	"code-intelligence.com/cifuzz/pkg/runner/jazzerjs"
	"code-intelligence.com/cifuzz/pkg/runner/libfuzzer"
	"code-intelligence.com/cifuzz/util/fileutil"
	"code-intelligence.com/cifuzz/util/sliceutil"
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

	ProjectDir      string
	fuzzTest        string
	targetMethod    string
	testNamePattern string
	argsToPass      []string

	buildStdout io.Writer
	buildStderr io.Writer
}

func (opts *runOptions) validate() error {
	var err error

	opts.SeedCorpusDirs, err = cmdutils.ValidateSeedCorpusDirs(opts.SeedCorpusDirs)
	if err != nil {
		return err
	}

	if opts.Dictionary != "" {
		// Check if the dictionary exists and can be accessed
		_, err = os.Stat(opts.Dictionary)
		if err != nil {
			return errors.Wrapf(err, "Failed to access dictionary %s", opts.Dictionary)
		}
	}

	if opts.BuildSystem == "" {
		opts.BuildSystem, err = config.DetermineBuildSystem(opts.ProjectDir)
		if err != nil {
			return err
		}
	}

	err = config.ValidateBuildSystem(opts.BuildSystem)
	if err != nil {
		return err
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

type Runner interface {
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
  <fuzz test> is the name of the class containing the fuzz test(s).
  If the fuzz test class contains multiple fuzz tests,
  you can use <fuzz test>::<method name> to specify a single fuzz
  test.

  Command completion for the <fuzz test> argument is supported.

  The --build-command flag is ignored.

  The inputs found in the directory

    src/test/resources/.../<fuzz test>Inputs

  are used as a starting point for the fuzzing run.

` + pterm.Style{pterm.Reset, pterm.Bold}.Sprint("Node.js") + `
  <fuzz test> is a regex pattern that matches against all paths
  containing fuzz test files.
  If the matched fuzz test file contains multiple fuzz tests,
  you can use <fuzz test>:<test name>
  to specify a regex that matches the fuzz test name.

  Command completion for the <fuzz test> argument is supported.

  The --build-command flag is ignored.

  The inputs found in the directory

    <fuzz test>.fuzz

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
				return err
			}

			if sliceutil.Contains(
				[]string{config.BuildSystemMaven, config.BuildSystemGradle},
				opts.BuildSystem,
			) {
				// Check if the fuzz test is a method of a class
				// And remove method from fuzz test argument
				if strings.Contains(args[0], "::") {
					split := strings.Split(args[0], "::")
					args[0], opts.targetMethod = split[0], split[1]
				}
			} else if opts.BuildSystem == config.BuildSystemNodeJS {
				// Check if the fuzz test contains a filter for the test name
				if strings.Contains(args[0], ":") {
					split := strings.Split(args[0], ":")
					args[0], opts.testNamePattern = split[0], strings.ReplaceAll(split[1], "\"", "")
				}
			}

			fuzzTests, err := resolve.FuzzTestArguments(opts.ResolveSourceFilePath, args, opts.BuildSystem, opts.ProjectDir)
			if err != nil {
				return err
			}
			opts.fuzzTest = fuzzTests[0]

			opts.argsToPass = argsToPass

			if opts.PrintJSON {
				// We only want JSON output on stdout, so we print the build
				// output to stderr.
				opts.buildStdout = cmd.ErrOrStderr()
			} else {
				opts.buildStdout = cmd.OutOrStdout()
			}
			opts.buildStderr = cmd.OutOrStderr()
			if logging.ShouldLogBuildToFile() {
				opts.buildStdout, err = logging.BuildOutputToFile(opts.ProjectDir, []string{opts.fuzzTest})
				if err != nil {
					return err
				}
				opts.buildStderr = opts.buildStdout
			}

			return opts.validate()
		},
		RunE: func(c *cobra.Command, args []string) error {
			var err error
			opts.Server, err = api.ValidateAndNormalizeServerURL(opts.Server)
			if err != nil {
				return err
			}

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
	authenticated, err := auth.GetAuthStatus(c.apiClient.Server)
	if err != nil {
		var connErr *api.ConnectionError
		if !errors.As(err, &connErr) {
			return err
		} else {
			log.Debugf("Connection error: %v", connErr)
		}
	}
	if !authenticated {
		log.Infof(messaging.UsageWarning())
	}

	var errorDetails *[]finding.ErrorDetails
	if authenticated {
		errorDetails, err = c.errorDetails()
		if err != nil {
			return err
		}
	}

	err = c.checkDependencies()
	if err != nil {
		return err
	}

	// Create a temporary directory which the builder can use to create
	// temporary files
	c.tempDir, err = os.MkdirTemp("", "cifuzz-run-")
	if err != nil {
		return errors.WithStack(err)
	}
	defer fileutil.Cleanup(c.tempDir)

	var buildResult *build.BuildResult
	buildResult, err = c.buildFuzzTest()
	if err != nil {
		return err
	}

	if c.opts.BuildOnly {
		return nil
	}

	err = c.prepareCorpusDirs(buildResult)
	if err != nil {
		return err
	}

	// Initialize the report handler. Only do this right before we start
	// the fuzz test, because this is storing a timestamp which is used
	// to figure out how long the fuzzing run is running.
	c.reportHandler, err = reporthandler.NewReportHandler(
		c.opts.fuzzTest,
		&reporthandler.ReportHandlerOptions{
			ProjectDir:           c.opts.ProjectDir,
			GeneratedCorpusDir:   buildResult.GeneratedCorpus,
			ManagedSeedCorpusDir: buildResult.SeedCorpus,
			UserSeedCorpusDirs:   c.opts.SeedCorpusDirs,
			PrintJSON:            c.opts.PrintJSON,
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
	err = c.reportHandler.PrintFinalMetrics()
	if err != nil {
		return err
	}

	// We need this check, otherwise we might hang forever in CI
	if c.opts.Project == "" && !c.opts.Interactive {
		log.Info("Skipping upload of findings because no project was specified and running in non-interactive mode.")
		return nil
	}
	if c.opts.Project == "" && !term.IsTerminal(int(os.Stdout.Fd())) {
		log.Info("Skipping upload of findings because no project was specified and stdout is not a terminal.")
		return nil
	}

	// check if there are findings that should be uploaded
	if authenticated && len(c.reportHandler.Findings) > 0 {
		err = c.uploadFindings(c.opts.fuzzTest, c.opts.BuildSystem, c.reportHandler.FirstMetrics, c.reportHandler.LastMetrics)
		if err != nil {
			return err
		}
	}

	return nil
}

func (c *runCmd) buildFuzzTest() (*build.BuildResult, error) {
	// Note that the build printer should *not* print to c.opts.buildStdout,
	// because that could be a file which is used to store the build log.
	// We don't want the messages of the build printer to be printed to
	// the build log file, so we let it print to stdout or stderr instead.
	var buildPrinterOutput io.Writer
	if c.opts.PrintJSON {
		buildPrinterOutput = c.Command.OutOrStdout()
	} else {
		buildPrinterOutput = c.Command.ErrOrStderr()
	}
	buildPrinter := logging.NewBuildPrinter(buildPrinterOutput, log.BuildInProgressMsg)

	buildResult, err := c._buildFuzzTest()
	if err != nil {
		buildPrinter.StopOnError(log.BuildInProgressErrorMsg)
	} else {
		buildPrinter.StopOnSuccess(log.BuildInProgressSuccessMsg, true)
	}
	return buildResult, err
}

func (c *runCmd) _buildFuzzTest() (*build.BuildResult, error) {
	var err error

	// TODO: Do not hardcode these values.
	sanitizers := []string{"address", "undefined"}

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

		var buildResults []*build.BuildResult
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

		cBuildResults, err := builder.Build([]string{c.opts.fuzzTest})
		if err != nil {
			return nil, err
		}
		// TODO: Maybe it would be more elegant to let builder.Build return
		//       an empty build result so that this check is not needed.
		if c.opts.BuildOnly {
			return nil, nil
		}

		return cBuildResults[0].BuildResult, nil

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

		var buildResult *build.BuildResult
		buildResult, err = builder.Build()
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

		var buildResult *build.BuildResult
		buildResult, err = builder.Build()
		if err != nil {
			return nil, err
		}
		return buildResult, err
	case config.BuildSystemNodeJS:
		// Node.js doesn't require a build step, so we just return an empty result.
		// We return an empty result to proceed with the fuzzing step (which
		// requires a build result).
		// *Possible* TODO: refactor runFuzzTest to not require a build result?
		return &build.BuildResult{}, nil
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

		cBuildResult, err := builder.Build(c.opts.fuzzTest)
		if err != nil {
			return nil, err
		}
		return cBuildResult.BuildResult, nil
	}

	return nil, errors.Errorf("Unsupported build system \"%s\"", c.opts.BuildSystem)
}

// runFuzzTest runs the fuzz test with the given build result.
func (c *runCmd) runFuzzTest(buildResult *build.BuildResult) error {
	var err error

	style := pterm.Style{pterm.Reset, pterm.FgLightBlue}
	if c.opts.targetMethod != "" {
		log.Infof("Running %s", style.Sprintf(c.opts.fuzzTest+"::"+c.opts.targetMethod))
	} else if c.opts.testNamePattern != "" {
		log.Infof("Running %s", style.Sprintf(c.opts.fuzzTest+":"+c.opts.testNamePattern))
	} else {
		log.Infof("Running %s", style.Sprintf(c.opts.fuzzTest))
	}

	if buildResult.Executable != "" {
		log.Debugf("Executable: %s", buildResult.Executable)
	}

	if c.opts.BuildSystem == config.BuildSystemBazel {
		// The install base directory contains e.g. the script generated
		// by bazel via --script_path and must therefore be accessible
		// inside the sandbox.
		cmd := exec.Command("bazel", "info", "install_base")
		err = cmd.Run()
		if err != nil {
			return cmdutils.WrapExecError(errors.WithStack(err), cmd)
		}
	}

	var libraryPaths []string
	if runtime.GOOS != "windows" && buildResult.Executable != "" {
		var err error
		libraryPaths, err = ldd.LibraryPaths(buildResult.Executable)
		if err != nil {
			return err
		}
	}

	// Use user-specified seed corpus dirs (if any) and the default seed
	// corpus (if it exists).
	exists, err := fileutil.Exists(buildResult.SeedCorpus)
	if err != nil {
		return err
	}
	if exists {
		c.opts.SeedCorpusDirs = append(c.opts.SeedCorpusDirs, buildResult.SeedCorpus)
	}

	runnerOpts := &libfuzzer.RunnerOptions{
		Dictionary:         c.opts.Dictionary,
		EngineArgs:         c.opts.EngineArgs,
		EnvVars:            []string{"NO_CIFUZZ=1"},
		FuzzTarget:         buildResult.Executable,
		LibraryDirs:        libraryPaths,
		GeneratedCorpusDir: buildResult.GeneratedCorpus,
		KeepColor:          !c.opts.PrintJSON && !log.PlainStyle(),
		ProjectDir:         c.opts.ProjectDir,
		ReadOnlyBindings:   []string{buildResult.BuildDir},
		ReportHandler:      c.reportHandler,
		SeedCorpusDirs:     c.opts.SeedCorpusDirs,
		Timeout:            c.opts.Timeout,
		UseMinijail:        c.opts.UseSandbox,
		Verbose:            viper.GetBool("verbose"),
	}

	// TODO: Only set ReadOnlyBindings if buildResult.BuildDir != ""

	var runner Runner

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
	case config.BuildSystemNodeJS:
		runnerOpts := &jazzerjs.RunnerOptions{
			TestPathPattern:  c.opts.fuzzTest,
			TestNamePattern:  c.opts.testNamePattern,
			LibfuzzerOptions: runnerOpts,
			PackageManager:   "npm",
		}
		runner = jazzerjs.NewRunner(runnerOpts)
	}

	return ExecuteRunner(runner)
}

func (c *runCmd) checkDependencies() error {
	var deps []dependencies.Key
	switch c.opts.BuildSystem {
	case config.BuildSystemCMake:
		deps = []dependencies.Key{
			dependencies.CMake,
			dependencies.LLVMSymbolizer,
		}
		switch runtime.GOOS {
		case "linux", "darwin":
			deps = append(deps, dependencies.Clang)
		case "windows":
			deps = append(deps, dependencies.VisualStudio)
		}
	case config.BuildSystemMaven:
		deps = []dependencies.Key{
			dependencies.Java,
			dependencies.Maven,
		}
	case config.BuildSystemGradle:
		deps = []dependencies.Key{
			dependencies.Java,
			dependencies.Gradle,
		}
	case config.BuildSystemNodeJS:
		deps = []dependencies.Key{
			dependencies.Node,
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

	err := dependencies.Check(deps, c.opts.ProjectDir)
	if err != nil {
		return err
	}
	return nil
}

func (c *runCmd) errorDetails() (*[]finding.ErrorDetails, error) {
	token := auth.GetToken(c.opts.Server)
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

func (c *runCmd) uploadFindings(fuzzTarget, buildSystem string, firstMetrics *report.FuzzingMetric, lastMetrics *report.FuzzingMetric) error {
	token := auth.GetToken(c.opts.Server)
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

		// if the user cancels the project selection, we don't want to upload
		if project == "<<cancel>>" {
			log.Info("Upload cancelled by user.")
			return nil
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
			return errors.New(message)
		}
	}

	// create campaign run on server for selected project
	campaignRunName, fuzzingRunName, err := c.apiClient.CreateCampaignRun(project, token, fuzzTarget, buildSystem, firstMetrics, lastMetrics)
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
	log.Notef("Uploaded %d findings to CI Sense at: %s", len(c.reportHandler.Findings), c.opts.Server)
	log.Infof("You can view the findings at %s/dashboard/%s/findings?origin=cli", c.opts.Server, campaignRunName)

	return nil
}

func ExecuteRunner(runner Runner) error {
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
		return signalErr
	}

	var execErr *cmdutils.ExecError
	if errors.As(err, &execErr) {
		// If the error is expected because libFuzzer might fail due to user
		// configuration, we return the execErr directly
		return execErr
	}

	// Routines.Wait() returns an error created by us so it already has a
	// stack trace and we don't want to add another one here
	// nolint: wrapcheck
	return err
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

	// add option to cancel
	items["<Cancel>"] = "<<cancel>>"

	projectName, err := dialog.Select("Select the project you want to upload your findings to", items, true)
	if err != nil {
		return "", errors.WithStack(err)
	}

	switch projectName {
	case "<<new>>":
		// ask user for project name
		projectName, err = dialog.Input("Enter the name of the project you want to create")
		if err != nil {
			return "", errors.WithStack(err)
		}

		token := tokenstorage.Get(c.opts.Server)
		project, err := c.apiClient.CreateProject(projectName, token)
		if err != nil {
			return "", err
		}
		return project.Name, nil

	case "<<cancel>>":
		return "<<cancel>>", nil
	}

	return projectName, nil
}

func (c *runCmd) prepareCorpusDirs(buildResult *build.BuildResult) error {
	switch c.opts.BuildSystem {
	case config.BuildSystemCMake, config.BuildSystemBazel, config.BuildSystemOther:
		// The generated corpus dir has to be created before starting the fuzzing run.
		err := os.MkdirAll(buildResult.GeneratedCorpus, 0o755)
		if err != nil {
			return errors.WithStack(err)
		}
		log.Infof("Storing generated corpus in %s", fileutil.PrettifyPath(buildResult.GeneratedCorpus))

		// Ensure that symlinks are resolved to be able to add minijail
		// bindings for the corpus dirs.
		exists, err := fileutil.Exists(buildResult.SeedCorpus)
		if err != nil {
			return err
		}
		if exists {
			buildResult.SeedCorpus, err = filepath.EvalSymlinks(buildResult.SeedCorpus)
			if err != nil {
				return errors.WithStack(err)
			}
		}

		buildResult.GeneratedCorpus, err = filepath.EvalSymlinks(buildResult.GeneratedCorpus)
		if err != nil {
			return errors.WithStack(err)
		}

		for i, dir := range c.opts.SeedCorpusDirs {
			c.opts.SeedCorpusDirs[i], err = filepath.EvalSymlinks(dir)
			if err != nil {
				return errors.WithStack(err)
			}
		}
	case config.BuildSystemMaven, config.BuildSystemGradle:
		// The seed corpus dir has to be created before starting the fuzzing run.
		// Otherwise jazzer will store the findings in the project dir.
		// It is not necessary to create the corpus dir. Jazzer will do that for us.
		err := os.MkdirAll(cmdutils.JazzerSeedCorpus(c.opts.fuzzTest, c.opts.ProjectDir), 0o755)
		if err != nil {
			return errors.WithStack(err)
		}
	}

	return nil
}
