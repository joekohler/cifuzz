package remoterun

import (
	"os"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	"code-intelligence.com/cifuzz/internal/bundler"
	"code-intelligence.com/cifuzz/internal/cmdutils"
	"code-intelligence.com/cifuzz/internal/cmdutils/logging"
	"code-intelligence.com/cifuzz/internal/cmdutils/resolve"
	"code-intelligence.com/cifuzz/internal/config"
	"code-intelligence.com/cifuzz/internal/container"
	"code-intelligence.com/cifuzz/pkg/log"
)

type containerRemoteRunOpts struct {
	bundler.Opts `mapstructure:",squash"`
	Registry     string `mapstructure:"registry"`
}

type containerRemoteRunCmd struct {
	*cobra.Command
	opts *containerRemoteRunOpts
}

func New() *cobra.Command {
	return newWithOptions(&containerRemoteRunOpts{})
}

func newWithOptions(opts *containerRemoteRunOpts) *cobra.Command {
	var bindFlags func()

	cmd := &cobra.Command{
		Use:   "remote-run",
		Short: "Build and run a Fuzz Test container image on a CI server",
		Args:  cobra.ExactArgs(1),
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

			// check for required registry flag
			err = cmd.MarkFlagRequired("registry")
			if err != nil {
				return errors.WithStack(err)
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
			cmd := &containerRemoteRunCmd{Command: c, opts: opts}
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
		cmdutils.AddDockerImageFlag,
		cmdutils.AddEngineArgFlag,
		cmdutils.AddEnvFlag,
		cmdutils.AddPrintJSONFlag,
		cmdutils.AddProjectDirFlag,
		cmdutils.AddProjectFlag,
		cmdutils.AddRegistryFlag,
		cmdutils.AddSeedCorpusFlag,
		cmdutils.AddServerFlag,
		cmdutils.AddTimeoutFlag,
		cmdutils.AddResolveSourceFileFlag,
	)

	return cmd
}

func (c *containerRemoteRunCmd) run() error {
	var err error

	buildPrinter := logging.NewBuildPrinter(os.Stdout, log.ContainerBuildInProgressMsg)
	imageID, err := c.buildImage()
	if err != nil {
		buildPrinter.StopOnError(log.ContainerBuildInProgressErrorMsg)
		return err
	}

	buildPrinter.StopOnSuccess(log.ContainerBuildInProgressSuccessMsg, false)

	return container.UploadImage(imageID, c.opts.Registry)
}

func (c *containerRemoteRunCmd) buildImage() (string, error) {
	b := bundler.New(&c.opts.Opts)
	bundlePath, err := b.Bundle()
	if err != nil {
		return "", err
	}

	return container.BuildImageFromBundle(bundlePath)
}
