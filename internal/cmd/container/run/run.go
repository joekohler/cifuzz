package run

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/docker/docker/pkg/stdcopy"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	"code-intelligence.com/cifuzz/internal/bundler"
	"code-intelligence.com/cifuzz/internal/cmdutils"
	"code-intelligence.com/cifuzz/internal/cmdutils/logging"
	"code-intelligence.com/cifuzz/internal/cmdutils/resolve"
	"code-intelligence.com/cifuzz/internal/completion"
	"code-intelligence.com/cifuzz/internal/config"
	"code-intelligence.com/cifuzz/internal/container"
	"code-intelligence.com/cifuzz/pkg/log"
)

type containerRunOpts struct {
	bundler.Opts  `mapstructure:",squash"`
	Interactive   bool   `mapstructure:"interactive"`
	ContainerPath string `mapstructure:"container"`
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

			// check if the fuzz tests contain a method of a class
			// And remove methods from fuzz test arguments
			for _, arg := range args {
				if strings.Contains(arg, "::") {
					split := strings.Split(arg, "::")
					opts.FuzzTests = append(opts.FuzzTests, split[0])
					opts.TargetMethods = append(opts.TargetMethods, split[1])
				} else {
					opts.FuzzTests = append(opts.FuzzTests, arg)
					opts.TargetMethods = append(opts.TargetMethods, "")
				}
			}

			err := config.FindAndParseProjectConfig(opts)
			if err != nil {
				log.Errorf(err, "Failed to parse cifuzz.yaml: %v", err.Error())
				return cmdutils.WrapSilentError(err)
			}

			opts.FuzzTests, err = resolve.FuzzTestArguments(opts.ResolveSourceFilePath, opts.FuzzTests, opts.BuildSystem, opts.ProjectDir)
			if err != nil {
				log.Print(err.Error())
				return cmdutils.WrapSilentError(err)
			}
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
		cmdutils.AddDockerImageFlag,
		cmdutils.AddEngineArgFlag,
		cmdutils.AddEnvFlag,
		cmdutils.AddPrintJSONFlag,
		cmdutils.AddProjectDirFlag,
		cmdutils.AddProjectFlag,
		cmdutils.AddSeedCorpusFlag,
		cmdutils.AddServerFlag,
		cmdutils.AddTimeoutFlag,
		cmdutils.AddResolveSourceFileFlag,
	)
	cmd.Flags().StringVar(&opts.ContainerPath, "container", "", "Path of an existing container to start a run with.")

	return cmd
}

func (c *containerRunCmd) run() error {
	var err error

	logging.StartBuildProgressSpinner(log.ContainerBuildInProgressMsg)
	containerID, err := c.buildContainerFromImage()
	if err != nil {
		logging.StopBuildProgressSpinnerOnError(log.ContainerBuildInProgressErrorMsg)
		return err
	}

	logging.StopBuildProgressSpinnerOnSuccess(log.ContainerBuildInProgressSuccessMsg, false)

	logging.StartBuildProgressSpinner(log.ContainerRunInProgressMsg)

	// Handle signal interrupts
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigChan
		logging.StopBuildProgressSpinnerOnError("Received interrupt, stopping container and cifuzz...")
		err := container.Stop(containerID)
		if err != nil {
			log.Error(errors.Wrap(err, "container could not be stopped"))
		}
	}()

	out, err := container.Start(containerID)
	if err != nil {
		logging.StopBuildProgressSpinnerOnError(log.ContainerRunInProgressErrorMsg)
		return err
	}
	logging.StopBuildProgressSpinnerOnSuccess(log.ContainerRunInProgressSuccessMsg, false)

	// Copy the logs to two different vars, so that we can pass them around
	// independently.
	containerStdOut := new(bytes.Buffer)
	containerStdErr := new(bytes.Buffer)
	_, err = stdcopy.StdCopy(containerStdOut, containerStdErr, out)
	if err != nil && err != io.EOF {
		return err
	}

	// TODO: make output pretty
	//  Remove 'cifuzz version' from output
	fmt.Println(containerStdOut.String())
	fmt.Println(containerStdErr.String())

	return nil
}

func (c *containerRunCmd) buildContainerFromImage() (string, error) {
	b := bundler.New(&c.opts.Opts)
	bundlePath, err := b.Bundle()
	if err != nil {
		return "", err
	}

	err = container.BuildImageFromBundle(bundlePath)
	if err != nil {
		return "", err
	}

	selectedFuzzer := c.opts.FuzzTests[0]
	if c.opts.TargetMethods[0] != "" {
		selectedFuzzer = fmt.Sprintf("%s::%s", selectedFuzzer, c.opts.TargetMethods[0])
	}

	return container.Create(selectedFuzzer)
}
