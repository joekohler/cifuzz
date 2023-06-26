package login

import (
	"io"
	"os"
	"strings"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"golang.org/x/term"

	"code-intelligence.com/cifuzz/internal/api"
	"code-intelligence.com/cifuzz/internal/cmdutils"
	"code-intelligence.com/cifuzz/internal/cmdutils/auth"
	"code-intelligence.com/cifuzz/internal/tokenstorage"
)

type loginOpts struct {
	Interactive bool   `mapstructure:"interactive"`
	Server      string `mapstructure:"server"`
}

type loginCmd struct {
	*cobra.Command
	opts      *loginOpts
	apiClient *api.APIClient
}

func New() *cobra.Command {
	var bindFlags func()

	cmd := &cobra.Command{
		Use:   "login",
		Short: "Authenticate with CI Sense",
		Long: `This command is used to authenticate with CI Sense.
To learn more, visit https://www.code-intelligence.com.`,
		Example: "$ cifuzz login",
		PreRunE: func(cmd *cobra.Command, args []string) error {
			// Bind viper keys to flags. We can't do this in the New
			// function, because that would re-bind viper keys which
			// were bound to the flags of other commands before.
			bindFlags()
			return nil
		},
		RunE: func(c *cobra.Command, args []string) error {
			opts := &loginOpts{
				Interactive: viper.GetBool("interactive"),
				Server:      viper.GetString("server"),
			}

			var err error
			opts.Server, err = api.ValidateAndNormalizeServerURL(opts.Server)
			if err != nil {
				return err
			}

			cmd := loginCmd{Command: c, opts: opts}
			cmd.apiClient = api.NewClient(opts.Server, cmd.Command.Root().Version)
			return cmd.run()
		},
	}
	bindFlags = cmdutils.AddFlags(cmd,
		cmdutils.AddInteractiveFlag,
		cmdutils.AddServerFlag,
	)

	cmdutils.DisableConfigCheck(cmd)

	return cmd
}

func (c *loginCmd) run() error {
	// Obtain the API access token
	var token string
	var err error

	// First, if stdin is *not* a TTY, we try to read it from stdin,
	// in case it was provided via `cifuzz login < token-file`
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		// This should never block because stdin is not a TTY.
		b, err := io.ReadAll(os.Stdin)
		if err != nil {
			return errors.WithStack(err)
		}
		token = strings.TrimSpace(string(b))
		return auth.CheckAndStoreToken(c.apiClient, token)
	}

	// Try the access tokens config file
	token = tokenstorage.Get(c.opts.Server)
	if token != "" {
		return auth.EnsureValidToken(*c.apiClient, token)
	}

	// Try reading it interactively
	if c.opts.Interactive && term.IsTerminal(int(os.Stdin.Fd())) {
		_, err = auth.ReadCheckAndStoreTokenInteractively(c.apiClient, nil)
		return err
	}

	err = errors.Errorf(`No API access token provided. Please pass a valid token via stdin or run
in interactive mode. You can generate a token here:
%s/dashboard/settings/account/tokens?create&origin=cli.`+"\n", c.opts.Server)
	return cmdutils.WrapIncorrectUsageError(err)
}
