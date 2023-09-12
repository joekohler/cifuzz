package run

import (
	"io"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	"code-intelligence.com/cifuzz/internal/api"
	"code-intelligence.com/cifuzz/internal/bundler"
	"code-intelligence.com/cifuzz/internal/cmd/bundle"
	"code-intelligence.com/cifuzz/internal/cmdutils"
	"code-intelligence.com/cifuzz/internal/cmdutils/auth"
	"code-intelligence.com/cifuzz/internal/cmdutils/logging"
	"code-intelligence.com/cifuzz/internal/cmdutils/resolve"
	"code-intelligence.com/cifuzz/internal/completion"
	"code-intelligence.com/cifuzz/internal/config"
	"code-intelligence.com/cifuzz/internal/container"
	"code-intelligence.com/cifuzz/pkg/log"
	"code-intelligence.com/cifuzz/pkg/messaging"
)

type containerRunOpts struct {
	bundler.Opts  `mapstructure:",squash"`
	PrintJSON     bool     `mapstructure:"print-json"`
	Interactive   bool     `mapstructure:"interactive"`
	Server        string   `mapstructure:"server"`
	ContainerPath string   `mapstructure:"container"`
	BindMounts    []string `mapstructure:"bind-mounts"`
}

type containerRunCmd struct {
	*cobra.Command
	opts *containerRunOpts
}

func New() *cobra.Command {
	return newWithOptions(&containerRunOpts{})
}

func (opts *containerRunOpts) Validate() error {
	return opts.Opts.Validate()
}

func newWithOptions(opts *containerRunOpts) *cobra.Command {
	var bindFlags func()

	cmd := &cobra.Command{
		Use:   "run",
		Short: "Build and run a Fuzz Test container image locally",
		Long: `This command builds and runs a Fuzz Test container image locally.
It can be used as a containerized version of the 'cifuzz bundle' command, where the
container is built and run locally instead of being pushed to a CI Sense server.`,
		ValidArgsFunction: completion.ValidFuzzTests,
		Args:              cobra.ExactArgs(1),
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
				return err
			}

			fuzzTests, err := resolve.FuzzTestArguments(opts.ResolveSourceFilePath, args, opts.BuildSystem, opts.ProjectDir)
			if err != nil {
				return err
			}
			opts.FuzzTests = fuzzTests
			opts.BuildSystemArgs = argsToPass

			return opts.Validate()
		},
		RunE: func(c *cobra.Command, args []string) error {
			cmd := &containerRunCmd{Command: c, opts: opts}
			return cmd.run()
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
		cmdutils.AddDockerImageFlagForContainerCommand,
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
	cmd.Flags().StringVar(&opts.ContainerPath, "container", "", "Path of an existing container to start a run with.")
	cmd.Flags().StringArrayVar(&opts.BindMounts, "bind", nil, "Bind mount a directory from the host into the container. "+
		"Format: --bind <src-path>:<dest-path>")

	// For now the --bind flag is only used for tests, so we hide it from the help output.
	err := cmd.Flags().MarkHidden("bind")
	if err != nil {
		panic(err)
	}

	return cmd
}

func (c *containerRunCmd) run() error {
	authenticated, err := auth.HasValidToken(c.opts.Server)
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

	buildOutput := c.OutOrStdout()
	if c.opts.PrintJSON {
		// We only want JSON output on stdout, so we print the build
		// output to stderr.
		buildOutput = c.ErrOrStderr()
	}
	buildPrinter := logging.NewBuildPrinter(buildOutput, log.ContainerBuildInProgressMsg)
	containerID, err := c.buildContainerFromImage(buildOutput)
	if err != nil {
		buildPrinter.StopOnError(log.ContainerBuildInProgressErrorMsg)
		return err
	}

	buildPrinter.StopOnSuccess(log.ContainerBuildInProgressSuccessMsg, false)

	err = container.Run(containerID, c.OutOrStdout(), c.ErrOrStderr())
	if err != nil {
		return err
	}

	return nil
}

func (c *containerRunCmd) buildContainerFromImage(buildOutput io.Writer) (string, error) {
	err := bundle.SetUpBundleLogging(buildOutput, c.ErrOrStderr(), &c.opts.Opts)
	if err != nil {
		return "", err
	}

	b := bundler.New(&c.opts.Opts)
	bundlePath, err := b.Bundle()
	if err != nil {
		return "", errors.WithMessage(err, "Failed to create bundle")
	}

	imageID, err := container.BuildImageFromBundle(bundlePath)
	if err != nil {
		return "", errors.WithMessagef(err, "Failed to build image from bundle %s", bundlePath)
	}

	return container.Create(imageID, c.opts.PrintJSON, c.opts.BindMounts)
}
