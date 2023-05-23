package remoterun

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"golang.org/x/term"

	"code-intelligence.com/cifuzz/internal/api"
	"code-intelligence.com/cifuzz/internal/bundler"
	"code-intelligence.com/cifuzz/internal/cmdutils"
	"code-intelligence.com/cifuzz/internal/cmdutils/login"
	"code-intelligence.com/cifuzz/internal/cmdutils/resolve"
	"code-intelligence.com/cifuzz/internal/completion"
	"code-intelligence.com/cifuzz/internal/config"
	"code-intelligence.com/cifuzz/pkg/dialog"
	"code-intelligence.com/cifuzz/pkg/log"
	"code-intelligence.com/cifuzz/util/fileutil"
	"code-intelligence.com/cifuzz/util/stringutil"
)

type remoteRunOpts struct {
	bundler.Opts `mapstructure:",squash"`
	Interactive  bool   `mapstructure:"interactive"`
	PrintJSON    bool   `mapstructure:"print-json"`
	ProjectName  string `mapstructure:"project"`
	Server       string `mapstructure:"server"`

	// Fields which are not configurable via viper (i.e. via cifuzz.yaml
	// and CIFUZZ_* environment variables), by setting
	// mapstructure:"-"
	BundlePath            string `mapstructure:"-"`
	ResolveSourceFilePath bool
}

func (opts *remoteRunOpts) Validate() error {
	err := config.ValidateBuildSystem(opts.BuildSystem)
	if err != nil {
		log.Error(err)
		return cmdutils.WrapSilentError(err)
	}

	if opts.BuildSystem == config.BuildSystemNodeJS && !config.AllowUnsupportedPlatforms() {
		err = errors.Errorf(config.NotSupportedErrorMessage("remote run", opts.BuildSystem))
		log.Error(err)
		return cmdutils.WrapSilentError(err)
	}

	if opts.BundlePath == "" {
		// We need to build a bundle, so we validate the bundler options
		// as well
		err := opts.Opts.Validate()
		if err != nil {
			return err
		}
	}

	if opts.Interactive {
		opts.Interactive = term.IsTerminal(int(os.Stdin.Fd())) && term.IsTerminal(int(os.Stdout.Fd()))
	}

	return nil
}

type runRemoteCmd struct {
	*cobra.Command

	opts      *remoteRunOpts
	apiClient *api.APIClient
}

func New() *cobra.Command {
	return newWithOptions(&remoteRunOpts{})
}

func newWithOptions(opts *remoteRunOpts) *cobra.Command {
	var bindFlags func()

	cmd := &cobra.Command{
		Use:   "remote-run [flags] [<fuzz test>]...",
		Short: "Build fuzz tests and run them on CI Sense",
		Long: `This command builds fuzz tests, packages all runtime artifacts into a
bundle and uploads it to CI Sense to start a remote
fuzzing run.

The inputs found in the inputs directory of the fuzz test are also added
to the bundle in addition to optional input directories specified with
the seed-corpus flag.
More details about the build system specific inputs directory location
can be found in the help message of the run command.

If the --bundle flag is used, building and bundling is skipped and the
specified bundle is uploaded to start a remote fuzzing run instead.

This command needs a token to access the API of the remote fuzzing
server. You can specify this token via the CIFUZZ_API_TOKEN environment
variable or by running 'cifuzz login' first.
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

			cmdutils.ViperMustBindPFlag("bundle", cmd.Flags().Lookup("bundle"))
			err := config.FindAndParseProjectConfig(opts)
			if err != nil {
				log.Errorf(err, "Failed to parse cifuzz.yaml: %v", err.Error())
				return cmdutils.WrapSilentError(err)
			}

			// Fail early if the platform is not supported
			isOSIndependent := opts.BuildSystem == config.BuildSystemMaven || opts.BuildSystem == config.BuildSystemGradle
			if runtime.GOOS != "linux" && !isOSIndependent && !config.AllowUnsupportedPlatforms() {
				err = errors.Errorf(config.NotSupportedErrorMessage("remote run", runtime.GOOS))
				log.Error(err)
				return cmdutils.WrapSilentError(err)
			}

			fuzzTests, err := resolve.FuzzTestArgument(opts.ResolveSourceFilePath, args, opts.BuildSystem, opts.ProjectDir)
			if err != nil {
				log.Error(err)
				return cmdutils.WrapSilentError(err)
			}
			opts.FuzzTests = fuzzTests
			opts.BuildSystemArgs = argsToPass

			if opts.ProjectName != "" && !strings.HasPrefix(opts.ProjectName, "projects/") {
				opts.ProjectName = "projects/" + opts.ProjectName
			}

			// If --json was specified, print all build output to stderr
			if opts.PrintJSON {
				opts.Stdout = cmd.ErrOrStderr()
			} else {
				opts.Stdout = cmd.OutOrStdout()
			}
			opts.Stderr = cmd.ErrOrStderr()

			opts.Server, err = api.ValidateAndNormalizeServerURL(opts.Server)
			if err != nil {
				return cmdutils.WrapSilentError(err)
			}

			// Print warning that flags which only effect the build of
			// the bundle are ignored when an existing bundle is specified
			if opts.BundlePath != "" {
				for _, flag := range cmdutils.BundleFlags {
					if cmd.Flags().Lookup(flag).Changed {
						log.Warnf("Flag --%s is ignored when --bundle is used", flag)
					}
				}
			}

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
			cmd := runRemoteCmd{Command: c, opts: opts}
			cmd.apiClient = api.NewClient(opts.Server, cmd.Command.Root().Version)
			return cmd.run()
		},
	}

	bindFlags = cmdutils.AddFlags(cmd,
		cmdutils.AddBranchFlag,
		cmdutils.AddBuildCommandFlag,
		cmdutils.AddCleanCommandFlag,
		cmdutils.AddBuildJobsFlag,
		cmdutils.AddCommitFlag,
		cmdutils.AddDictFlag,
		cmdutils.AddDockerImageFlag,
		cmdutils.AddEngineArgFlag,
		cmdutils.AddEnvFlag,
		cmdutils.AddInteractiveFlag,
		cmdutils.AddPrintJSONFlag,
		cmdutils.AddProjectDirFlag,
		cmdutils.AddProjectFlag,
		cmdutils.AddSeedCorpusFlag,
		cmdutils.AddServerFlag,
		cmdutils.AddTimeoutFlag,
		cmdutils.AddResolveSourceFileFlag,
	)
	cmd.Flags().StringVar(&opts.BundlePath, "bundle", "", "Path of an existing bundle to start a remote run with.")

	return cmd
}

func (c *runRemoteCmd) run() error {
	var err error

	token := login.GetToken(c.opts.Server)
	if token == "" {
		log.Print("You need to authenticate to CI Sense to use this command.")

		if !c.opts.Interactive {
			log.Print("Please set CIFUZZ_API_TOKEN or run 'cifuzz login'.")
			return cmdutils.ErrSilent
		}

		yes, err := dialog.Confirm("Log in now?", true)
		if err != nil {
			return err
		}
		if !yes {
			log.Print("Please set CIFUZZ_API_TOKEN or run 'cifuzz login'.")
			return cmdutils.ErrSilent
		}
		token, err = login.ReadCheckAndStoreTokenInteractively(c.apiClient, nil)
		if err != nil {
			return err
		}
	} else {
		err = login.CheckValidToken(c.apiClient, token)
		if err != nil {
			return err
		}
	}

	if c.opts.ProjectName == "" {
		projects, err := c.apiClient.ListProjects(token)
		if err != nil {
			log.Error(err)
			err = errors.New("Flag \"project\" must be set")
			return cmdutils.WrapIncorrectUsageError(err)
		}

		if c.opts.Interactive {
			c.opts.ProjectName, err = c.selectProject(projects)
			if err != nil {
				return err
			}

			// this will ask users via a y/N prompt if they want to persist the
			// project choice
			err = dialog.AskToPersistProjectChoice(c.opts.ProjectName)
			if err != nil {
				return cmdutils.WrapSilentError(err)
			}

		} else {
			var projectNames []string
			for _, p := range projects {
				projectNames = append(projectNames, strings.TrimPrefix(p.Name, "projects/"))
			}
			if len(projectNames) == 0 {
				log.Warnf("No projects found. Please create a project first at %s.", c.opts.Server)
				err = errors.New("Flag \"project\" must be set")
				return cmdutils.WrapIncorrectUsageError(err)
			}
			err = errors.New("Flag \"project\" must be set. Valid projects:\n  " + strings.Join(projectNames, "\n  "))
			return cmdutils.WrapIncorrectUsageError(err)
		}
	}

	if c.opts.BundlePath == "" {
		tempDir, err := os.MkdirTemp("", "cifuzz-bundle-")
		if err != nil {
			return errors.WithStack(err)
		}
		defer fileutil.Cleanup(tempDir)
		bundlePath := filepath.Join(tempDir, "fuzz_tests.tar.gz")
		c.opts.BundlePath = bundlePath
		c.opts.OutputPath = bundlePath

		if cmdutils.ShouldLogBuildToFile() {
			log.CreateCurrentProgressSpinner(nil, log.BundleInProgressMsg)
		}

		b := bundler.New(&c.opts.Opts)
		err = b.Bundle()
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
	}

	artifact, err := c.apiClient.UploadBundle(c.opts.BundlePath, c.opts.ProjectName, token)
	if err != nil {
		var apiErr *api.APIError
		if !errors.As(err, &apiErr) {
			// API calls might fail due to network issues, invalid server
			// responses or similar. We don't want to print a stack trace
			// in those cases.
			log.Error(err)
			return cmdutils.WrapSilentError(err)
		}
		return err
	}

	campaignRunName, err := c.apiClient.StartRemoteFuzzingRun(artifact, token)
	if err != nil {
		// API calls might fail due to network issues, invalid server
		// responses or similar. We don't want to print a stack trace
		// in those cases.
		log.Error(err)
		return cmdutils.WrapSilentError(err)
	}

	if c.opts.PrintJSON {
		result := struct{ CampaignRun string }{campaignRunName}
		s, err := stringutil.ToJSONString(result)
		if err != nil {
			return err
		}
		_, _ = fmt.Fprintln(os.Stdout, s)
	} else {
		// TODO: Would be nice to be able to link to a page which immediately
		//       shows details about the run, but currently details are only
		//       shown on the "<fuzz target>/edit" page, which lists all runs
		//       of the fuzz target.
		path, err := url.JoinPath(c.opts.Server, "dashboard", campaignRunName, "overview")
		if err != nil {
			return err
		}

		values := url.Values{}
		values.Add("origin", "cli")

		url, err := url.Parse(path)
		if err != nil {
			return err
		}
		url.RawQuery = values.Encode()

		log.Successf(`Successfully started fuzzing run. To view findings and coverage, open:
    %s
`, url.String())
	}

	return nil
}

func (c *runRemoteCmd) selectProject(projects []*api.Project) (string, error) {
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

	if len(items) == 0 {
		err := errors.Errorf("No projects found. Please create a project first at %s.", c.opts.Server)
		log.Error(err)
		return "", cmdutils.WrapSilentError(err)
	}

	projectName, err := dialog.Select("Select the project you want to start a fuzzing run for", items, true)
	if err != nil {
		return "", errors.WithStack(err)
	}

	return projectName, nil
}
