package remoterun

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/pkg/errors"
	"github.com/pterm/pterm"
	"github.com/spf13/cobra"

	"code-intelligence.com/cifuzz/internal/api"
	"code-intelligence.com/cifuzz/internal/bundler"
	"code-intelligence.com/cifuzz/internal/cmdutils"
	"code-intelligence.com/cifuzz/internal/cmdutils/auth"
	"code-intelligence.com/cifuzz/internal/cmdutils/logging"
	"code-intelligence.com/cifuzz/internal/cmdutils/resolve"
	"code-intelligence.com/cifuzz/internal/config"
	"code-intelligence.com/cifuzz/internal/container"
	"code-intelligence.com/cifuzz/pkg/dialog"
	"code-intelligence.com/cifuzz/pkg/log"
)

type containerRemoteRunOpts struct {
	bundler.Opts `mapstructure:",squash"`
	Interactive  bool   `mapstructure:"interactive"`
	Server       string `mapstructure:"server"` // CI Sense
	PrintJSON    bool   `mapstructure:"print-json"`
	Project      string `mapstructure:"project"` // CI Sense
	Registry     string `mapstructure:"registry"`
}

type containerRemoteRunCmd struct {
	*cobra.Command
	opts      *containerRemoteRunOpts
	apiClient *api.APIClient
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
			var err error
			opts.Server, err = api.ValidateAndNormalizeServerURL(opts.Server)
			if err != nil {
				return err
			}

			cmd := &containerRemoteRunCmd{Command: c, opts: opts}
			cmd.apiClient = api.NewClient(opts.Server)
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

	token, err := auth.EnsureValidToken(c.opts.Server)
	if err != nil {
		return err
	}

	if c.opts.Project == "" {
		projects, err := c.apiClient.ListProjects(token)
		if err != nil {
			log.Error(err)
			err = errors.New("Flag \"project\" must be set")
			return cmdutils.WrapIncorrectUsageError(err)
		}

		if c.opts.Interactive {
			c.opts.Project, err = dialog.ProjectPickerWithOptionNew(projects, "Select the project you want to start a fuzzing run for:", c.apiClient, token)
			if err != nil {
				return err
			}

			if c.opts.Project == "<<cancel>>" {
				log.Info("Container remote run cancelled.")
				return nil
			}

			// this will ask users via a y/N prompt if they want to persist the
			// project choice
			err = dialog.AskToPersistProjectChoice(c.opts.Project)
			if err != nil {
				return err
			}
		} else {
			err = errors.New("Flag 'project' must be set.")
			return cmdutils.WrapIncorrectUsageError(err)
		}
	}

	buildOutput := c.OutOrStdout()
	if c.opts.PrintJSON {
		// We only want JSON output on stdout, so we print the build
		// output to stderr.
		buildOutput = c.ErrOrStderr()
	}
	buildPrinter := logging.NewBuildPrinter(buildOutput, log.ContainerBuildInProgressMsg)
	imageID, err := c.buildImage()
	if err != nil {
		buildPrinter.StopOnError(log.ContainerBuildInProgressErrorMsg)
		return err
	}

	buildPrinter.StopOnSuccess(log.ContainerBuildInProgressSuccessMsg, false)

	//	if err != nil {
	//		err = container.UploadImage(imageID, c.opts.Registry)
	//		return err
	//	}

	imageID = c.opts.Registry + ":" + imageID
	response, err := c.apiClient.PostContainerRemoteRun(imageID, c.opts.Project, c.opts.FuzzTests, token)
	if err != nil {
		return err
	}

	responseJSON, err := json.MarshalIndent(response, "", "  ")
	if err != nil {
		return errors.WithStack(err)
	}

	if c.opts.PrintJSON {
		_, _ = fmt.Fprintln(os.Stdout, string(responseJSON))
	}

	addr, err := cmdutils.BuildURLFromParts(c.opts.Server, "dashboard", "projects", c.opts.Project, "runs")
	if err != nil {
		return err
	}

	log.Successf(`Successfully started fuzzing run. To view findings and coverage, open:
    %s`, addr)

	// show updating status here

	spinner, _ := pterm.DefaultSpinner.Start("Waiting for the run to finish...")
	var wg sync.WaitGroup
	wg.Add(1)

	go func() {
		for {
			status, err := c.apiClient.GetContainerRemoteRunStatus(response.Run.Nid, token)
			if err != nil {
				log.Error(err)
				spinner.Fail("Failed to get run status.")
				return
			}

			spinner.UpdateText(fmt.Sprintf("Run status: %s", status.Run.Status))

			if status.Run.Status == "finished" {
				spinner.Success("Run finished. Last status: " + status.Run.Status)
				wg.Done()
				break
			}

			// sleep for 5 seconds
			time.Sleep(5 * time.Second)
			wg.Done()
		}
	}()
	wg.Wait()
	spinner.Warning("Run monitoring stopped. Took too long to finish.")

	return nil
}

func (c *containerRemoteRunCmd) buildImage() (string, error) {
	b := bundler.New(&c.opts.Opts)
	bundlePath, err := b.Bundle()
	if err != nil {
		return "", err
	}

	return container.BuildImageFromBundle(bundlePath)
}

// monitorCampaignRun monitors the status of a campaign run on the CI Sense
// API. It returns when the run is finished or an error occurs.
// ported from core/cmd/cictl/monitor_campaign_run.go
func monitorCampaignRun(apiClient *api.APIClient, runNID string, token string) error {
	status, err := apiClient.GetContainerRemoteRunStatus(runNID, token)
	if err != nil {
		return err
	}

	if status.Run.Status == "finished" {
		log.Successf("Run finished.")
		return nil
	}

	var ticker *time.Ticker
	pullInterval := time.Duration(5) * time.Second

	// TODO: make this configurable via --monitor-duration
	runFor := time.Duration(300) * time.Second
	if runFor < pullInterval {
		ticker = time.NewTicker(1 * time.Second)
	} else {
		ticker = time.NewTicker(pullInterval)
	}
	defer ticker.Stop()
	stopChannel := make(chan struct{})
	time.AfterFunc(runFor, func() { close(stopChannel) })

	for {
		select {
		case <-ticker.C:
			status, err := apiClient.GetContainerRemoteRunStatus(runNID, token)
			if err != nil {
				return err
			}

			if status.Run.Status == "cancelled" {
				fmt.Println("Run cancelled.")
				return nil
			}

			if status.Run.Status == "STOPPED" || status.Run.Status == "SUCCEEDED" {
				// we can exit ear;y if the campaign run has stopped before the configuration duruation
				close(stopChannel)
			}
		case <-stopChannel:
			switch status.Run.Status {
			case "COMPILING", "UNKNOWN", "WAITING_FOR_FUZZING_AGENTS", "UNSPECIFIED":
				log.Infof("After the timeout of %s the test collection run still has status %s", runFor, status.Run.Status)
			case "INITIALIZING", "PENDING":
				notExecutedRunsCount := 0
				completedRunsCount := 0
				failedRunsCount := 0
				incompleteRunsCount := 0
				inProgressRunsCount := 0

			}
		}
	}

}

func checkErrors(targetRun api.ContainerRun) error {
	projectName, err := api.Remarshal(targetRun.Name, &projectUtils.Name{})
	if err != nil {
		log.Fatalf("failed to get project name from test collection run %q: %+v", targetRun.Name, err)
	}
	for _, fuzzingRun := range targetRun.Runs {
		res, err := getFuzzingRunFindings(context.Background(), conn, projectName, fuzzingRun.Name)
		if err != nil {
			log.Printf("failed to get findings for fuzzing run %q of test collection %q: %+v", fuzzingRun.Name, targetRun.Name, err)
		}
		if res == nil || res.Findings == nil {
			continue
		}

		for _, finding := range res.Findings {
			if _, ok := reportedFindings[finding.Name]; ok {
				// we have already dealt with this finding
				continue
			}

			if finding.ErrorReport != nil {
				findingType := finding.ErrorReport.Type.String()
				switch finding.State {
				case types.Finding_IGNORED:
					// Do nothing for ignored findings.
					continue
				case types.Finding_VERIFIED:
					firstOccurrenceMessage := ""
					if finding.FirstSeenFinding != "" && monitorCampaignRunCfg.DashboardAddress != "" {
						firstOccurrenceMessage = fmt.Sprintf("First occurrence of this finding: \n\t%s",
							generateFindingLink(finding.FirstSeenFinding))
					}
					log.Printf("Previously verified finding with type %s was found. %s\n",
						findingType, firstOccurrenceMessage)
					continue
				}
				if _, ok := findingsTypeMap[findingType]; !ok {
					log.Printf(
						"Finding with type %s was found, but its type is not being monitored. Monitored finding types: %v",
						findingType, monitorCampaignRunCfg.FindingsType)
					continue
				}
				findingSeverity := strings.ToUpper(finding.ErrorReport.GetMoreDetails().GetSeverity().GetDescription())
				if _, ok := ignoredFindingSeveritiesMap[findingSeverity]; findingSeverity != "" && ok {
					log.Printf(
						"Finding with severity %s was found, but its type is not being monitored. Monitored finding severities: %v",
						findingSeverity, monitorCampaignRunCfg.MinFindingsSeverity)
					continue
				}

				// get the full finding because we currently only have the name and type field
				fullFinding, err := findingsClient.GetFinding(context.Background(), &local.GetFindingRequest{Name: finding.Name})
				if err == nil {
					finding = fullFinding
				} else {
					log.Printf("failed to get the finding %v: %+v", finding.Name, err)
				}

				findingJSON, err := stringutils.ToJsonString(finding.ErrorReport)
				if err != nil {
					log.Fatalf("%+v", err)
				}
				findingJSONFilename := fmt.Sprintf("finding-%s.json", filepath.Base(finding.Name))
				err = ioutil.WriteFile(findingJSONFilename, []byte(findingJSON), 0o644)
				if err != nil {
					log.Fatalf("%+v", err)
				}
				msg := fmt.Sprintf("Found %s while fuzzing. Finding report:\n%s", findingType, findingJSON)
				if monitorCampaignRunCfg.DashboardAddress != "" {
					findingLink := generateFindingLink(finding.Name)
					msg += fmt.Sprintf("\n\nLink to finding in the dashboard: %s", findingLink)
				} else {
					log.Println("No dashboard address provided, so the link to the finding in the dashboard cannot be formed")
				}
				if finding.GetErrorReport().GetDebuggingInfo() != nil {
					debugURL := fmt.Sprintf("vscode://code-intelligence.ci-fuzzing-plugin/debug?finding=%s", finding.Name)
					msg += fmt.Sprintf("\n\nDebug crash in VS Code: %s", debugURL)
				}
				if finding.GetErrorReport().GetDebuggingInfo().GetBreakPoints() != nil {
					gotoURL := fmt.Sprintf("vscode://code-intelligence.ci-fuzzing-plugin/goto?finding=%s", finding.Name)
					msg += fmt.Sprintf("\n\nView line of crash in VS Code: %s", gotoURL)
				}
				reportedFindings[finding.Name] = struct{}{}
				if monitorCampaignRunCfg.KeepRunning {
					log.Println(msg)
				} else {
					return errors.New(msg)
				}
			}
		}
	}
	return nil
}
