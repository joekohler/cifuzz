package finding

import (
	"fmt"
	"net/url"
	"strings"
	"text/tabwriter"

	"github.com/pkg/errors"
	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"code-intelligence.com/cifuzz/internal/api"
	"code-intelligence.com/cifuzz/internal/cmdutils"
	"code-intelligence.com/cifuzz/internal/cmdutils/auth"
	"code-intelligence.com/cifuzz/internal/cmdutils/login"
	"code-intelligence.com/cifuzz/internal/completion"
	"code-intelligence.com/cifuzz/internal/config"
	"code-intelligence.com/cifuzz/pkg/finding"
	"code-intelligence.com/cifuzz/pkg/log"
	"code-intelligence.com/cifuzz/util/stringutil"
)

type options struct {
	PrintJSON   bool   `mapstructure:"print-json"`
	ProjectDir  string `mapstructure:"project-dir"`
	ConfigDir   string `mapstructure:"config-dir"`
	Interactive bool   `mapstructure:"interactive"`
	Server      string `mapstructure:"server"`
}

type findingCmd struct {
	*cobra.Command
	opts *options
}

func New() *cobra.Command {
	return newWithOptions(&options{})
}

func newWithOptions(opts *options) *cobra.Command {
	var bindFlags func()

	cmd := &cobra.Command{
		Use:               "finding [name]",
		Aliases:           []string{"findings"},
		Short:             "List and show findings",
		Args:              cobra.MaximumNArgs(1),
		ValidArgsFunction: completion.ValidFindings,
		PreRunE: func(cmd *cobra.Command, args []string) error {
			// Bind viper keys to flags. We can't do this in the New
			// function, because that would re-bind viper keys which
			// were bound to the flags of other commands before.
			bindFlags()
			err := config.FindAndParseProjectConfig(opts)
			if err != nil {
				log.Errorf(err, "Failed to parse cifuzz.yaml: %v", err.Error())
				return cmdutils.WrapSilentError(err)
			}
			return nil
		},
		RunE: func(c *cobra.Command, args []string) error {
			opts.Interactive = viper.GetBool("interactive")
			opts.Server = viper.GetString("server")

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
			cmd := findingCmd{Command: c, opts: opts}
			return cmd.run(args)
		},
	}

	// Note: If a flag should be configurable via viper as well (i.e.
	//       via cifuzz.yaml and CIFUZZ_* environment variables), bind
	//       it to viper in the PreRun function.
	bindFlags = cmdutils.AddFlags(cmd,
		cmdutils.AddPrintJSONFlag,
		cmdutils.AddProjectDirFlag,
		cmdutils.AddInteractiveFlag,
		cmdutils.AddServerFlag,
	)

	return cmd
}

func (cmd *findingCmd) run(args []string) error {
	authenticated, err := auth.GetAuthStatus(cmd.opts.Server)
	if err != nil {
		return err
	}
	if !authenticated && cmd.opts.Interactive {
		_, err = auth.ShowServerConnectionDialog(cmd.opts.Server)
		if err != nil {
			return err
		}
	}

	errorDetails, err := cmd.checkForErrorDetails()
	if err != nil {
		return err
	}

	if len(args) == 0 {
		// If called without arguments, `cifuzz findings` lists short
		// descriptions of all findings
		findings, err := finding.ListFindings(cmd.opts.ProjectDir, errorDetails)
		if err != nil {
			return err
		}

		if cmd.opts.PrintJSON {
			s, err := stringutil.ToJSONString(findings)
			if err != nil {
				return err
			}
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), s)
			return nil
		}

		if len(findings) == 0 {
			log.Print("This project doesn't have any findings yet")
			return nil
		}

		w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 1, ' ', 0)

		data := [][]string{
			{"Severity", "Name", "Description", "Location"},
		}

		// showing the fuzz test name is a SaaS only feature...
		if authenticated {
			data = [][]string{
				{"Severity", "Name", "Description", "FuzzTest", "Location"},
			}
		}

		for _, f := range findings {
			if f.MoreDetails != nil {
				colorFunc := getColorFunctionForSeverity(f.MoreDetails.Severity.Score)

				if authenticated {
					data = append(data, []string{
						colorFunc(fmt.Sprintf("%.1f", f.MoreDetails.Severity.Score)),
						f.Name,
						// FIXME: replace f.ShortDescriptionColumns()[0] with
						// f.MoreDetails.Name once we cover all bugs with our
						// error-details.json
						f.ShortDescriptionColumns()[0],
						// showing the fuzz test name is a SaaS only feature...
						f.FuzzTest,
						f.ShortDescriptionColumns()[1],
					})
				} else {
					data = append(data, []string{
						colorFunc(fmt.Sprintf("%.1f", f.MoreDetails.Severity.Score)),
						f.Name,
						// FIXME: replace f.ShortDescriptionColumns()[0] with
						// f.MoreDetails.Name once we cover all bugs with our
						// error-details.json
						f.ShortDescriptionColumns()[0],
						f.ShortDescriptionColumns()[1],
					})
				}
			} else {
				if authenticated {
					data = append(data, []string{
						"n/a",
						f.Name,
						f.ShortDescriptionColumns()[0],
						f.FuzzTest,
						f.ShortDescriptionColumns()[1],
					})
				} else {
					data = append(data, []string{
						"n/a",
						f.Name,
						f.ShortDescriptionColumns()[0],
						f.ShortDescriptionColumns()[1],
					})
				}
			}
		}
		err = pterm.DefaultTable.WithHasHeader().WithData(data).Render()
		if err != nil {
			return err
		}

		err = w.Flush()
		if err != nil {
			return errors.WithStack(err)
		}
		return nil
	}

	// If called with one argument, `cifuzz finding <finding name>`
	// prints the information available for the specified finding
	findingName := args[0]
	f, err := finding.LoadFinding(cmd.opts.ProjectDir, findingName, errorDetails)
	if finding.IsNotExistError(err) {
		log.Errorf(err, "Finding %s does not exist", findingName)
		return cmdutils.WrapSilentError(err)
	}
	if err != nil {
		return err
	}
	return cmd.printFinding(f)
}

func (cmd *findingCmd) printFinding(f *finding.Finding) error {
	if cmd.opts.PrintJSON {
		s, err := stringutil.ToJSONString(f)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintln(cmd.OutOrStdout(), s)
		if err != nil {
			return err
		}
	} else {
		s := pterm.Style{pterm.Reset, pterm.Bold}.Sprint(f.ShortDescriptionWithName())
		s += fmt.Sprintf("\nDate: %s\n", f.CreatedAt)
		s += fmt.Sprintf("\n  %s\n", strings.Join(f.Logs, "\n  "))
		_, err := fmt.Fprint(cmd.OutOrStdout(), s)
		if err != nil {
			return err
		}
		PrintMoreDetails(f)

	}
	return nil
}

func PrintMoreDetails(f *finding.Finding) {
	if f.MoreDetails == nil {
		return
	}

	log.Info("\ncifuzz found more extensive information about this finding:")

	data := [][]string{
		{"Name", f.MoreDetails.Name},
	}

	if f.MoreDetails.Severity != nil {
		data = append(data, []string{"Severity Level", string(f.MoreDetails.Severity.Level)})
		data = append(data, []string{"Severity Score", fmt.Sprintf("%.1f", f.MoreDetails.Severity.Score)})

	}
	if f.MoreDetails.Links != nil {
		for _, link := range f.MoreDetails.Links {
			data = append(data, []string{link.Description, link.URL})
		}
	}
	if f.MoreDetails.OwaspDetails != nil {
		if f.MoreDetails.OwaspDetails.Description != "" {
			data = append(data, []string{"OWASP Name", f.MoreDetails.OwaspDetails.Name})
			data = append(data, []string{"OWASP Description", f.MoreDetails.OwaspDetails.Description})
		}
	}
	if f.MoreDetails.CweDetails != nil {
		if f.MoreDetails.CweDetails.Description != "" {
			data = append(data, []string{"CWE Name", f.MoreDetails.CweDetails.Name})
			data = append(data, []string{"CWE Description", f.MoreDetails.CweDetails.Description})
		}
	}

	err := pterm.DefaultTable.WithData(data).WithBoxed().Render()
	if err != nil {
		log.Error(err)
	}

	if f.MoreDetails.Description != "" {
		pterm.Println(pterm.Blue("Description:"))
		fmt.Println(f.MoreDetails.Description)
	}
	if f.MoreDetails.Mitigation != "" {
		pterm.Println(pterm.Blue("\nMitigation:"))
		fmt.Println(f.MoreDetails.Mitigation)
	}
}

func getColorFunctionForSeverity(severity float32) func(a ...interface{}) string {
	switch {
	case severity >= 7.0:
		return pterm.Red
	case severity >= 4.0:
		return pterm.Yellow
	default:
		return pterm.Gray
	}
}

// checkForErrorDetails tries to get error details from the API.
// If the API is available and the user is logged in, it returns the error details.
// If the API is not available or the user is not logged in, it returns nil.
func (cmd *findingCmd) checkForErrorDetails() (*[]finding.ErrorDetails, error) {
	var errorDetails []finding.ErrorDetails
	var err error

	token := login.GetToken(cmd.opts.Server)
	log.Debugf("Checking for error details on server %s", cmd.opts.Server)

	apiClient := api.NewClient(cmd.opts.Server, cmd.Command.Root().Version)
	errorDetails, err = apiClient.GetErrorDetails(token)
	if err != nil {
		var connErr *api.ConnectionError
		if !errors.As(err, &connErr) {
			return nil, err
		} else {
			log.Warn("Skipping error details.")
			log.Debugf("Connection error: %v (continiung gracefully)", connErr)
			return nil, nil
		}
	}
	return &errorDetails, nil
}
