package bundle

import (
	"os"
	"runtime"

	"github.com/pkg/errors"
	"github.com/pterm/pterm"
	"github.com/spf13/cobra"

	"code-intelligence.com/cifuzz/internal/bundler"
	"code-intelligence.com/cifuzz/internal/cmdutils"
	"code-intelligence.com/cifuzz/internal/cmdutils/resolve"
	"code-intelligence.com/cifuzz/internal/completion"
	"code-intelligence.com/cifuzz/internal/config"
	"code-intelligence.com/cifuzz/pkg/log"
)

type options struct {
	bundler.Opts `mapstructure:",squash"`
}

func (opts *options) Validate() error {
	err := config.ValidateBuildSystem(opts.BuildSystem)
	if err != nil {
		log.Error(err)
		return cmdutils.WrapSilentError(err)
	}

	if opts.BuildSystem == config.BuildSystemNodeJS {
		err = errors.Errorf(config.NotSupportedErrorMessage("bundle", opts.BuildSystem))
		log.Error(err)
		return cmdutils.WrapSilentError(err)
	}

	return opts.Opts.Validate()
}

func New() *cobra.Command {
	return newWithOptions(&options{})
}

func newWithOptions(opts *options) *cobra.Command {
	var bindFlags func()
	cmd := &cobra.Command{
		Use:   "bundle [flags] [<fuzz test>]...",
		Short: "Bundles fuzz tests into an archive",
		Long: `This command bundles all runtime artifacts required by the
given fuzz tests into a self-contained archive (bundle) that can be executed
by a CI Fuzz Server instance.

The inputs found in the inputs directory of the fuzz test are also added
to the bundle in addition to optional input directories specified with
the seed-corpus flag.
More details about the build system specific inputs directory location
can be found in the help message of the run command.

The usage of this command depends on the build system
configured for the project.

This command will select an appropriate Docker image for execution based
on the build system. This can be overridden with a docker-image flag.

` + pterm.Style{pterm.Reset, pterm.Bold}.Sprint("CMake") + `
  <fuzz test> is the name of the fuzz test defined in the add_fuzz_test
  command in your CMakeLists.txt.

  Command completion for the <fuzz test> argument is supported when the
  fuzz test was built before or after running 'cifuzz reload'.

  The --build-command flag is ignored.

  Additional CMake arguments can be passed after a "--". For example:

    cifuzz run my_fuzz_test -- -G Ninja

  If no fuzz tests are specified, all fuzz tests are added to the bundle.

` + pterm.Style{pterm.Reset, pterm.Bold}.Sprint("Bazel") + `
  <fuzz test> is the name of the cc_fuzz_test target as defined in your
  BUILD file, either as a relative or absolute Bazel label.

  Command completion for the <fuzz test> argument is supported.

  The '--build-command' flag is ignored.

  Additional Bazel arguments can be passed after a "--". For example:

    cifuzz run my_fuzz_test -- --sandbox_debug

` + pterm.Style{pterm.Reset, pterm.Bold}.Sprint("Maven/Gradle") + `
  <fuzz test> is the name of the class containing the fuzz test.

  Command completion for the <fuzz test> argument is supported.

  The --build-command flag is ignored.

  If no fuzz tests are specified, all fuzz tests are added to the bundle.

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

`,
		ValidArgsFunction: completion.ValidFuzzTests,
		Args:              cobra.ArbitraryArgs,
		PreRunE: func(cmd *cobra.Command, args []string) error {
			// Bind viper keys to flags. We can't do this in the New
			// function, because that would re-bind viper keys which
			// were bound to the flags of other commands before.
			bindFlags()

			var argsToPass []string
			if cmd.ArgsLenAtDash() != -1 {
				argsToPass = args[cmd.ArgsLenAtDash():]
				args = args[:cmd.ArgsLenAtDash()]
			}

			err := config.FindAndParseProjectConfig(opts)
			if err != nil {
				log.Errorf(err, "Failed to parse cifuzz.yaml: %v", err.Error())
				return cmdutils.WrapSilentError(err)
			}

			// Fail early if the platform is not supported. Creating the
			// bundle actually works on all platforms, but the backend
			// currently only supports running a bundle on Linux, so the
			// user can't do anything useful with a bundle created on
			// other platforms.
			//
			// We set CIFUZZ_ALLOW_UNSUPPORTED_PLATFORMS in tests to
			// still be able to test that creating the bundle works on
			// all platforms.
			// TODO: remove CIFUZZ_BUNDLE_ON_UNSUPPORTED_PLATFORMS after the new
			//  CIFUZZ_ALLOW_UNSUPPORTED_PLATFORMS is released
			isOSIndependent := opts.BuildSystem == config.BuildSystemMaven ||
				opts.BuildSystem == config.BuildSystemGradle
			if os.Getenv("CIFUZZ_BUNDLE_ON_UNSUPPORTED_PLATFORMS") == "" &&
				os.Getenv("CIFUZZ_ALLOW_UNSUPPORTED_PLATFORMS") == "" &&
				runtime.GOOS != "linux" &&
				!isOSIndependent {
				err = errors.Errorf(config.NotSupportedErrorMessage("bundle", runtime.GOOS))
				log.Error(err)
				return cmdutils.WrapSilentError(err)
			}

			fuzzTests, err := resolve.FuzzTestArgument(opts.ResolveSourceFilePath, args, opts.BuildSystem, opts.ProjectDir)
			if err != nil {
				log.Print(err.Error())
				return cmdutils.WrapSilentError(err)
			}
			opts.FuzzTests = fuzzTests
			opts.BuildSystemArgs = argsToPass

			opts.BuildStdout = cmd.OutOrStdout()
			opts.BuildStderr = cmd.OutOrStderr()
			if cmdutils.ShouldLogBuildToFile() {
				opts.BuildStdout, err = cmdutils.BuildOutputToFile(opts.ProjectDir, opts.FuzzTests)
				if err != nil {
					log.Errorf(err, "Failed to setup logging: %v", err.Error())
					return cmdutils.WrapSilentError(err)
				}
				opts.BuildStderr = opts.BuildStdout
			}

			return opts.Validate()
		},
		RunE: func(c *cobra.Command, args []string) error {
			if cmdutils.ShouldLogBuildToFile() {
				log.CreateCurrentProgressSpinner(nil, log.BundleInProgressMsg)
			}

			err := bundler.New(&opts.Opts).Bundle()
			if err != nil {
				if cmdutils.ShouldLogBuildToFile() {
					log.StopCurrentProgressSpinner(log.GetPtermErrorStyle(), log.BundleInProgressErrorMsg)
					printErr := cmdutils.PrintBuildLogOnStdout()
					if printErr != nil {
						log.Error(printErr)
					}
				}

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

			if cmdutils.ShouldLogBuildToFile() {
				log.StopCurrentProgressSpinner(log.GetPtermSuccessStyle(), log.BundleInProgressSuccessMsg)
				log.Info(cmdutils.GetMsgPathToBuildLog())
			}

			log.Successf("Successfully created bundle: %s", opts.OutputPath)

			return nil
		},
	}

	bindFlags = cmdutils.AddFlags(cmd,
		cmdutils.AddAdditionalFilesFlag,
		cmdutils.AddBranchFlag,
		cmdutils.AddBuildCommandFlag,
		cmdutils.AddCleanCommandFlag,
		cmdutils.AddBuildJobsFlag,
		cmdutils.AddCommitFlag,
		cmdutils.AddDictFlag,
		cmdutils.AddDockerImageFlag,
		cmdutils.AddEngineArgFlag,
		cmdutils.AddEnvFlag,
		cmdutils.AddProjectDirFlag,
		cmdutils.AddSeedCorpusFlag,
		cmdutils.AddTimeoutFlag,
		cmdutils.AddResolveSourceFileFlag,
	)
	cmd.Flags().StringVarP(&opts.OutputPath, "output", "o", "", "Output path of the bundle (.tar.gz)")

	return cmd
}
