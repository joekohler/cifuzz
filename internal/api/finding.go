package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/url"
	"time"

	"github.com/pkg/errors"

	"code-intelligence.com/cifuzz/pkg/finding"
	"code-intelligence.com/cifuzz/pkg/log"
)

type Findings struct {
	Findings []Finding `json:"findings"`
}

type Finding struct {
	Name        string      `json:"name"`
	DisplayName string      `json:"display_name"`
	FuzzTarget  string      `json:"fuzz_target"`
	FuzzingRun  string      `json:"fuzzing_run"`
	CampaignRun string      `json:"campaign_run"`
	ErrorReport ErrorReport `json:"error_report"`
	Timestamp   string      `json:"timestamp"`
}

type ErrorReport struct {
	Logs      []string `json:"logs"`
	Details   string   `json:"details"`
	Type      string   `json:"type,omitempty"`
	InputData []byte   `json:"input_data,omitempty"`

	DebuggingInfo      *DebuggingInfo        `json:"debugging_info,omitempty"`
	HumanReadableInput string                `json:"human_readable_input,omitempty"`
	MoreDetails        *finding.ErrorDetails `json:"more_details,omitempty"`
	Tag                string                `json:"tag,omitempty"`
	ShortDescription   string                `json:"short_description,omitempty"`
}

type DebuggingInfo struct {
	ExecutablePath string        `json:"executable_path,omitempty"`
	RunArguments   []string      `json:"run_arguments,omitempty"`
	BreakPoints    []BreakPoint  `json:"break_points,omitempty"`
	Environment    []Environment `json:"environment,omitempty"`
}

type BreakPoint struct {
	SourceFilePath string           `json:"source_file_path,omitempty"`
	Location       *FindingLocation `json:"location,omitempty"`
	Function       string           `json:"function,omitempty"`
}

type FindingLocation struct {
	Line   uint32 `json:"line,omitempty"`
	Column uint32 `json:"column,omitempty"`
}

type Environment struct {
	Name  string `json:"name,omitempty"`
	Value string `json:"value,omitempty"`
}

type Severity struct {
	Description string  `json:"description,omitempty"`
	Score       float32 `json:"score,omitempty"`
}

func (client *APIClient) UploadFinding(project string, fuzzTarget string, campaignRunName string, fuzzingRunName string, finding *finding.Finding, token string) error {
	// loop through the stack trace and create a list of breakpoints
	breakPoints := []BreakPoint{}
	for _, stackFrame := range finding.StackTrace {
		breakPoints = append(breakPoints, BreakPoint{
			SourceFilePath: stackFrame.SourceFile,
			Location: &FindingLocation{
				Line:   stackFrame.Line,
				Column: stackFrame.Column,
			},
			Function: stackFrame.Function,
		})
	}

	errorDetails, err := client.GetErrorDetails(token)
	if err != nil {
		var connErr *ConnectionError
		if !errors.As(err, &connErr) {
			return err
		} else {
			log.Warn("Connection to API failed. Skipping error details.")
			log.Debugf("Connection error: %v (continuing gracefully)", err)
			return nil
		}
	}

	finding.EnhanceWithErrorDetails(&errorDetails)

	findings := &Findings{
		Findings: []Finding{
			{
				Name:        project + "/findings/cifuzz-" + finding.Name,
				DisplayName: finding.Name,
				FuzzTarget:  fuzzTarget,
				FuzzingRun:  fuzzingRunName,
				CampaignRun: campaignRunName,
				ErrorReport: ErrorReport{
					Logs:      finding.Logs,
					Details:   finding.Details,
					Type:      string(finding.Type),
					InputData: finding.InputData,
					DebuggingInfo: &DebuggingInfo{
						BreakPoints: breakPoints,
					},
					MoreDetails:      finding.MoreDetails,
					Tag:              fmt.Sprint(finding.Tag),
					ShortDescription: finding.ShortDescription(),
				},
				Timestamp: time.Now().Format(time.RFC3339),
			},
		},
	}

	body, err := json.Marshal(findings)
	if err != nil {
		return errors.WithStack(err)
	}

	url, err := url.JoinPath("/v1", project, "findings")
	if err != nil {
		return errors.WithStack(err)
	}
	resp, err := client.sendRequest("POST", url, bytes.NewReader(body), token)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return responseToAPIError(resp)
	}

	return nil
}
