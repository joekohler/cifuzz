package root

import (
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/alessio/shellescape"
	"github.com/pkg/errors"
	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"

	bundleCmd "code-intelligence.com/cifuzz/internal/cmd/bundle"
	coverageCmd "code-intelligence.com/cifuzz/internal/cmd/coverage"
	createCmd "code-intelligence.com/cifuzz/internal/cmd/create"
	findingCmd "code-intelligence.com/cifuzz/internal/cmd/finding"
	initCmd "code-intelligence.com/cifuzz/internal/cmd/init"
	integrateCmd "code-intelligence.com/cifuzz/internal/cmd/integrate"
	loginCmd "code-intelligence.com/cifuzz/internal/cmd/login"
	reloadCmd "code-intelligence.com/cifuzz/internal/cmd/reload"
	remoteRunCmd "code-intelligence.com/cifuzz/internal/cmd/remoterun"
	runCmd "code-intelligence.com/cifuzz/internal/cmd/run"
	"code-intelligence.com/cifuzz/internal/cmdutils"
	"code-intelligence.com/cifuzz/internal/config"
	"code-intelligence.com/cifuzz/pkg/log"
)

var version string

func New() (*cobra.Command, error) {
	rootCmd := &cobra.Command{
		Use:     "cifuzz",
		Version: version,
		// We are using our custom ErrSilent instead to support a more specific
		// error handling
		SilenceErrors: true,
		SilenceUsage:  true,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {

			log.Infof("cifuzz version %s", version)

			err := cmdutils.Chdir()
			if err != nil {
				log.Error(err, err.Error())
				return cmdutils.ErrSilent
			}

			if cmdutils.NeedsConfig(cmd) {
				_, err = config.FindConfigDir()
				if errors.Is(err, os.ErrNotExist) {
					// The project directory doesn't exist, this is an expected
					// error, so we print it and return a silent error to avoid
					// printing a stack trace
					log.Error(err, fmt.Sprintf("%s\nUse 'cifuzz init' to set up a project for use with cifuzz.", err.Error()))
					return cmdutils.ErrSilent
				}
				if err != nil {
					return err
				}
			}

			return nil
		},
	}

	rootCmd.PersistentFlags().Bool("help", false, "Show help for command")
	rootCmd.PersistentFlags().BoolP("verbose", "v", false,
		"Show verbose output on console, can be helpful for debugging")
	if err := viper.BindPFlag("verbose", rootCmd.PersistentFlags().Lookup("verbose")); err != nil {
		return nil, errors.WithStack(err)
	}

	rootCmd.PersistentFlags().StringP("directory", "C", "",
		"Change the directory before performing any operations")
	if err := viper.BindPFlag("directory", rootCmd.PersistentFlags().Lookup("directory")); err != nil {
		return nil, errors.WithStack(err)
	}

	rootCmd.PersistentFlags().Bool("no-notifications", false,
		"Turn off desktop notifications")
	if err := viper.BindPFlag("no-notifications", rootCmd.PersistentFlags().Lookup("no-notifications")); err != nil {
		return nil, errors.WithStack(err)
	}

	rootCmd.SetFlagErrorFunc(rootFlagErrorFunc)

	cobra.EnableCommandSorting = false
	rootCmd.AddCommand(loginCmd.New())
	rootCmd.AddCommand(initCmd.New())
	rootCmd.AddCommand(createCmd.New())
	rootCmd.AddCommand(runCmd.New())
	rootCmd.AddCommand(remoteRunCmd.New())
	rootCmd.AddCommand(reloadCmd.New())
	rootCmd.AddCommand(bundleCmd.New())
	rootCmd.AddCommand(coverageCmd.New())
	rootCmd.AddCommand(findingCmd.New())
	rootCmd.AddCommand(integrateCmd.New())

	return rootCmd, nil
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	rootCmd, err := New()
	if err != nil {
		fmt.Printf("error while creating root command: %+v", err)
		os.Exit(1)
	}

	if cmd, err := rootCmd.ExecuteC(); err != nil {
		// Errors that are not ErrSilent are not expected and we want to show their full stacktrace
		var silentErr *cmdutils.SilentError
		if !errors.As(err, &silentErr) {
			_, _ = fmt.Fprint(cmd.ErrOrStderr(), pterm.Style{pterm.Bold, pterm.FgRed}.Sprintf("%+v\n", err))
		}

		// We only want to print the usage message if an ErrIncorrectUsage
		// was returned or it's an error produced by cobra which was
		// caused by incorrect usage
		var usageErr *cmdutils.IncorrectUsageError
		if errors.As(err, &usageErr) ||
			strings.HasPrefix(err.Error(), "unknown command") ||
			regexp.MustCompile(`(accepts|requires).*arg\(s\)`).MatchString(err.Error()) {

			// Ensure that there is an extra newline between the error
			// and the usage message
			if !strings.HasSuffix(err.Error(), "\n") {
				_, _ = fmt.Fprintln(cmd.ErrOrStderr())
			}

			// Make cmd.Help() print to stderr
			cmd.SetOut(cmd.ErrOrStderr())
			// Print the usage message of the command. We use cmd.Help()
			// here instead of cmd.UsageString() because the latter
			// doesn't include the long description.
			_ = cmd.Help()
		}

		var couldBeSandboxError *cmdutils.CouldBeSandboxError
		if errors.As(err, &couldBeSandboxError) {
			// Ensure that there is an extra newline between the error
			// and the following message
			if !strings.HasSuffix(err.Error(), "\n") {
				_, _ = fmt.Fprintln(cmd.ErrOrStderr())
			}
			msg := `Note: If you don't expect this fuzz test to do any harm to the system
accidentally (like overwriting files), you might want to try
running it without sandboxing:

    %s --use-sandbox=false

For more information on cifuzz sandboxing, see:

    https://github.com/CodeIntelligenceTesting/cifuzz/blob/main/docs/Getting-Started.md#sandboxing

`
			log.Notef(msg, shellescape.QuoteCommand(os.Args))
		}

		var signalErr *cmdutils.SignalError
		if errors.As(err, &signalErr) {
			os.Exit(128 + int(signalErr.Signal))
		}

		os.Exit(1)
	}
}

func rootFlagErrorFunc(cmd *cobra.Command, err error) error {
	if err == pflag.ErrHelp {
		return err
	}
	return cmdutils.WrapIncorrectUsageError(err)
}
