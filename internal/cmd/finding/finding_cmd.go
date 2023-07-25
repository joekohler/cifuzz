package finding

import (
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/pkg/errors"
	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"golang.org/x/term"

	"code-intelligence.com/cifuzz/internal/api"
	"code-intelligence.com/cifuzz/internal/cmdutils"
	"code-intelligence.com/cifuzz/internal/cmdutils/auth"
	"code-intelligence.com/cifuzz/internal/completion"
	"code-intelligence.com/cifuzz/internal/config"
	"code-intelligence.com/cifuzz/pkg/finding"
	"code-intelligence.com/cifuzz/pkg/log"
	"code-intelligence.com/cifuzz/pkg/messaging"
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
				return err
			}
			return nil
		},
		RunE: func(c *cobra.Command, args []string) error {
			opts.Interactive = viper.GetBool("interactive")
			// Command should not be interactive when stdin is not a terminal.
			// TODO: Should this be global? Set on a Viper level for all commands?
			if opts.Interactive {
				opts.Interactive = term.IsTerminal(int(os.Stdin.Fd())) && term.IsTerminal(int(os.Stdout.Fd()))
			}
			opts.Server = viper.GetString("server")

			var err error
			opts.Server, err = api.ValidateAndNormalizeServerURL(opts.Server)
			if err != nil {
				return err
			}
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
	if !authenticated {
		log.Infof(messaging.UsageWarning())
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
			{"Severity", "Name", "Description", "Fuzz Test", "Location"},
		}

		for _, f := range findings {
			score := "n/a"
			locationInfo := "n/a"
			// add location (file, function, line) if available
			if len(f.ShortDescriptionColumns()) > 1 {
				locationInfo = f.ShortDescriptionColumns()[1]
			}
			// check if MoreDetails exists to avoid nil pointer errors
			if f.MoreDetails != nil {
				// check if we have a severity and if we have a severity score
				if f.MoreDetails.Severity != nil {
					colorFunc := getColorFunctionForSeverity(f.MoreDetails.Severity.Score)
					score = colorFunc(fmt.Sprintf("%.1f", f.MoreDetails.Severity.Score))
				}
			}
			data = append(data, []string{
				score,
				f.Name,
				// FIXME: replace f.ShortDescriptionColumns()[0] with
				// f.MoreDetails.Name once we cover all bugs with our
				// error-details.json
				f.ShortDescriptionColumns()[0],
				f.FuzzTest,
				locationInfo,
			})
		}
		err = pterm.DefaultTable.WithHasHeader().WithData(data).Render()
		if err != nil {
			return errors.WithStack(err)
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
		return errors.Wrapf(err, "Finding %s does not exist", findingName)
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
			return errors.WithStack(err)
		}
	} else {
		s := pterm.Style{pterm.Reset, pterm.Bold}.Sprint(f.ShortDescriptionWithName())
		s += fmt.Sprintf("\nDate: %s\n", f.CreatedAt)
		s += fmt.Sprintf("\n  %s\n", strings.Join(f.Logs, "\n  "))
		_, err := fmt.Fprint(cmd.OutOrStdout(), s)
		if err != nil {
			return errors.WithStack(err)
		}
		PrintMoreDetails(f)
	}
	return nil
}

func PrintMoreDetails(f *finding.Finding) {
	if f.MoreDetails == nil {
		return
	}
	// the finding might have non-nil MoreDetails, but no information
	if f.MoreDetails.Name == "" || f.MoreDetails.Severity == nil {
		return
	}

	log.Info("\ncifuzz found more extensive information about this finding:")
	log.Debugf("Error ID: %s", f.MoreDetails.ID)
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
			data = append(data, []string{"OWASP Description", wrapLongStringToMultiline(f.MoreDetails.OwaspDetails.Description, 80)})
		}
	}
	if f.MoreDetails.CweDetails != nil {
		if f.MoreDetails.CweDetails.Description != "" {
			data = append(data, []string{"CWE Name", f.MoreDetails.CweDetails.Name})
			data = append(data, []string{"CWE Description", wrapLongStringToMultiline(f.MoreDetails.CweDetails.Description, 80)})
		}
	}

	tableString, err := pterm.DefaultTable.WithData(data).WithBoxed().Srender()
	if err != nil {
		log.Error(err)
	}
	log.Print(tableString)

	if f.MoreDetails.Description != "" {
		log.Print(pterm.Blue("Description:"))
		log.Print(f.MoreDetails.Description)
	}
	if f.MoreDetails.Mitigation != "" {
		log.Print(pterm.Blue("\nMitigation:"))
		log.Print(f.MoreDetails.Mitigation)
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

// wrapLongStringToMultiline wraps a long string to multiple lines.
// It tries to wrap at the last space before the maxLineLength to avoid
// breaking words.
func wrapLongStringToMultiline(s string, maxLineLength int) string {
	var result string
	var currentLine string
	var currentLineLength int

	for _, word := range strings.Split(s, " ") {
		if currentLineLength+len(word)+1 > maxLineLength {
			result += currentLine + "\n"
			currentLine = ""
			currentLineLength = 0
		}
		currentLine += word + " "
		currentLineLength += len(word) + 1
	}
	result += currentLine
	return result
}

// checkForErrorDetails tries to get error details from the API.
// If the API is available and the user is logged in, it returns the error details.
// If the API is not available or the user is not logged in, it returns nil.
func (cmd *findingCmd) checkForErrorDetails() (*[]finding.ErrorDetails, error) {
	var errorDetails []finding.ErrorDetails
	var err error

	token := auth.GetToken(cmd.opts.Server)
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
