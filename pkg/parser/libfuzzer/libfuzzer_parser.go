package libfuzzer_output_parser

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/pkg/errors"
	"github.com/pterm/pterm"

	"code-intelligence.com/cifuzz/pkg/log"
	"code-intelligence.com/cifuzz/pkg/parser/errorid"
	"code-intelligence.com/cifuzz/pkg/parser/sanitizer"
	"code-intelligence.com/cifuzz/pkg/report"
	"code-intelligence.com/cifuzz/util/regexutil"
)

var (
	libfuzzerTimeoutErrorPattern = regexp.MustCompile(
		`ALARM: working on the last Unit for (?P<timeout_seconds>\d+) seconds`,
	)
	libfuzzerErrorPattern = regexp.MustCompile(
		`==\d+== ERROR: libFuzzer: (?P<error_type>.+)`)

	jazzerSecurityIssuePattern = regexp.MustCompile(
		`== Java Exception:.*com.code_intelligence.jazzer.api.FuzzerSecurityIssue(?P<type>Low|Medium|High|Critical):\s*(?P<message>.*)$`)
	javaExceptionErrorPattern = regexp.MustCompile(
		`== Java Exception:\s*(?P<error_type>.*)`)
	javaAssertionErrorPattern = regexp.MustCompile(
		`== Java Assertion Error`)

	coveragePattern = regexp.MustCompile(
		`#(?P<total_execs>\d+).*cov:\s(?P<edges>\d+)\sft:\s(?P<features>\d+)\scorp:\s(?P<corpus_size>\d+)/.*exec/s:\s(?P<executions_per_second>\d+)\s`)
	testInputFilePattern = regexp.MustCompile(
		`Test unit written to\s*(?P<test_input_file>.*)`)
	slowInputPattern = regexp.MustCompile(
		`\s*Slowest unit: (?P<duration>\d+) s.*`)
	goPanicPattern = regexp.MustCompile(`^panic:\s+\S+`)
)

type parser struct {
	*Options

	FindingReported bool

	reportsCh chan *report.Report

	// A finding that was found in the libfuzzer output but wasn't sent
	// yet, because we keep reading more output lines for some time and
	// attach them to the finding if they seem to belong to it
	pendingFinding                       *report.Finding
	numMetricsLinesSinceFindingIsPending int

	lastNewFeatureTime time.Time // Timestamp representing the point when the last new feature was reported
	lastFeatures       int       // Last features reported by Libfuzzer
	lastNewEdgeTime    time.Time // Timestamp representing the point when the last new edge was reported
	lastEdges          int       // Last edges reported by Libfuzzer
}

type Options struct {
	SupportJazzer bool
}

func NewLibfuzzerOutputParser(options *Options) *parser {
	if options == nil {
		options = &Options{}
	}
	return &parser{Options: options}
}

func (p *parser) Parse(ctx context.Context, input io.Reader, reportsCh chan *report.Report) error {
	p.reportsCh = reportsCh
	defer close(p.reportsCh)
	scanner := bufio.NewScanner(input)

	// Send an initial report, to notify the server that the fuzzer is now running
	log.Debug("Sending initial report")
	err := p.sendReport(ctx, &report.Report{Status: report.RunStatus_INITIALIZING})
	if err != nil {
		return err
	}

	for scanner.Scan() {
		err := p.parseLine(ctx, scanner.Text())
		if err != nil {
			return err
		}
	}

	// The fuzzer output was closed, which means that the fuzzer exited.
	// If there is still a pending finding, send it now.
	err = p.sendPendingFindingIfAny(ctx)
	if err != nil {
		return err
	}

	return nil
}

func (p *parser) sendReport(ctx context.Context, report *report.Report) error {
	select {
	case p.reportsCh <- report:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (p *parser) parseLine(ctx context.Context, possiblyColorizedLine string) error {
	// Sanitizer reports can be colorized, but the ANSI escapes used for colors
	// should not be included in logs.
	line := pterm.RemoveColorFromString(possiblyColorizedLine)

	metric := p.parseAsFuzzingMetric(line)
	if metric != nil {
		log.Debug("Sending metric report")
		err := p.sendReport(ctx, &report.Report{
			Status: report.RunStatus_RUNNING,
			Metric: metric,
		})
		if err != nil {
			return err
		}

		if p.pendingFinding != nil {
			p.numMetricsLinesSinceFindingIsPending += 1
		}
		if p.numMetricsLinesSinceFindingIsPending == 2 {
			// We saw two metric lines since the line that marked the
			// beginning of the error report. We assume that this means
			// that no more lines from the error report are following.
			// A single metrics line could be in between the lines of
			// the error report, because the metrics and error report
			// printed asynchronously, but the time between two metrics
			// lines are printed is long enough that it should be safe
			// to assume that the error report was printed out completely
			// by now. We therefore send the pending finding now.
			err = p.sendPendingFinding(ctx)
			if err != nil {
				return err
			}
		}

		return nil
	}

	finding := p.parseAsNewFinding(line)
	if finding != nil && !p.libFuzzerErrorFollowingGoPanic(finding) {
		// If there is still a pending finding, send it now, because
		// we'll treat all further output lines as belonging to the new
		// finding.
		err := p.sendPendingFindingIfAny(ctx)
		if err != nil {
			return err
		}

		// Store the finding but don't send it yet, because we keep
		// parsing more output lines which might belong to the error
		// report and contain relevant info.
		p.pendingFinding = finding

		return nil
	}

	if p.pendingFinding != nil {
		// The line is not a metrics line and doesn't mark a new finding,
		// so we append it to the pending finding
		p.pendingFinding.Logs = append(p.pendingFinding.Logs, line)
	}

	// Check if the line contains the path to the test input file (which
	// we expect when we have a pending finding)
	testInputFilePath, ok := parseAsTestInputFilePath(line)
	if ok {
		testInput, err := ioutil.ReadFile(testInputFilePath)
		if err != nil {
			return errors.WithStack(err)
		}

		// Attach the input data to the pending finding
		if p.pendingFinding == nil {
			return errors.Errorf("Unexpected test input line: %s", line)
		}
		p.pendingFinding.InputData = testInput
		return nil
	}

	return nil
}

func (p *parser) parseAsNewFinding(line string) *report.Finding {
	if p.SupportJazzer {
		finding := p.parseAsJazzerFinding(line)
		if finding != nil {
			return finding
		}
	}

	finding := p.parseAsGoFinding(line)
	if finding != nil {
		return finding
	}

	finding = p.parseAsLibfuzzerFinding(line)
	if finding != nil {
		return finding
	}

	finding = sanitizer.ParseAsFinding(line)
	if finding != nil {
		return finding
	}

	finding = parseAsSlowInput(line)
	if finding != nil {
		return finding
	}

	return nil
}

func parseAsTestInputFilePath(logLine string) (string, bool) {
	result, found := regexutil.FindNamedGroupsMatch(testInputFilePattern, logLine)
	if found {
		return result["test_input_file"], true
	}
	return "", false
}

func (p *parser) parseAsGoFinding(line string) *report.Finding {
	if _, found := regexutil.FindNamedGroupsMatch(goPanicPattern, line); found {
		return &report.Finding{
			Type:    report.ErrorType_CRASH,
			Details: "Go Panic",
			Logs:    []string{line},
		}
	}
	return nil
}

func (p *parser) libFuzzerErrorFollowingGoPanic(report *report.Finding) bool {
	return p.pendingFinding.GetDetails() == "Go Panic" && report.GetDetails() != "Go Panic"
}

func (p *parser) parseAsLibfuzzerFinding(line string) *report.Finding {
	// For timeout errors, the first output line belonging to the error
	// report is *not* the "ERROR:" line, but the "ALARM:" line, so we
	// match that pattern first
	result, found := regexutil.FindNamedGroupsMatch(libfuzzerTimeoutErrorPattern, line)
	if found {
		return &report.Finding{
			Type:    report.ErrorType_CRASH, // aka Vulnerability
			Details: fmt.Sprintf("timeout after %s seconds", result["timeout_seconds"]),
			Logs:    []string{line},
		}
	}

	// All other libfuzzer errors start with the "ERROR:" line
	result, found = regexutil.FindNamedGroupsMatch(libfuzzerErrorPattern, line)
	if found {
		if strings.HasPrefix(result["error_type"], "timeout") {
			// This the "ERROR:" line of a timeout report. We already
			// created a finding for that.
			return nil
		}

		if strings.HasPrefix(result["error_type"], "out-of-memory") &&
			strings.HasPrefix(p.pendingFinding.GetDetails(), "out-of-memory") {
			// libFuzzer sometimes reports OOM errors twice, once
			// because a single malloc call exceeds the rss limit and
			// once because the rss limit was exceeded. We don't want
			// to create two findings in that case, so we ignore the
			// second error.
			return nil
		}

		return &report.Finding{
			Type:    report.ErrorType_CRASH, // aka Vulnerability
			Details: result["error_type"],
			Logs:    []string{line},
		}
	}

	return nil
}

func (p *parser) parseAsJazzerFinding(line string) *report.Finding {
	matches, found := regexutil.FindNamedGroupsMatch(jazzerSecurityIssuePattern, line)
	if found {
		issueSeverity := matches["type"]
		severityScore := 0.0
		switch issueSeverity {
		case "Critical":
			severityScore = 9.0
		case "High":
			severityScore = 7.0
		case "Medium":
			severityScore = 5.0
		case "Low":
			severityScore = 1.0
		}
		exceptionMessage := strings.TrimSpace(matches["message"])
		description := "Security Issue: " + exceptionMessage
		uiDescription := "Security Issue Raised"
		if len(exceptionMessage) > 0 {
			uiDescription = exceptionMessage
		}

		return &report.Finding{
			Type:    report.ErrorType_CRASH, // aka Vulnerability
			Details: description,
			MoreDetails: &report.ErrorDetails{
				Name: uiDescription, // This field is shown in the UI
				Severity: &report.Severity{
					Description: issueSeverity,
					Score:       float32(severityScore),
				},
			},
			Logs: []string{line},
		}
	}

	_, found = regexutil.FindNamedGroupsMatch(javaAssertionErrorPattern, line)
	if found {
		return &report.Finding{
			Type:    report.ErrorType_WARNING, // aka Bug
			Details: "Java Assertion Error",
			Logs:    []string{line},
		}
	}

	matches, found = regexutil.FindNamedGroupsMatch(javaExceptionErrorPattern, line)
	if found {
		return &report.Finding{
			Type:    report.ErrorType_WARNING, // aka Bug
			Details: matches["error_type"],
			Logs:    []string{line},
		}
	}

	return nil
}

func (p *parser) parseAsFuzzingMetric(log string) *report.FuzzingMetric {
	if result, found := regexutil.FindNamedGroupsMatch(coveragePattern, log); found {
		totalExecs, err := strconv.ParseUint(result["total_execs"], 10, 64)
		if err != nil {
			return nil
		}
		features, err := strconv.Atoi(result["features"])
		if err != nil {
			return nil
		}
		edges, err := strconv.Atoi(result["edges"])
		if err != nil {
			return nil
		}
		execsPerSec, err := strconv.Atoi(result["executions_per_second"])
		if err != nil {
			return nil
		}
		corpusSize, err := strconv.Atoi(result["corpus_size"])
		if err != nil {
			return nil
		}
		now := time.Now()
		secondsSinceLastFeature := now.Unix() - p.lastNewFeatureTime.Unix()
		if features > p.lastFeatures {
			p.lastNewFeatureTime = now
			p.lastFeatures = features
			secondsSinceLastFeature = 0
		}
		secondsSinceLastEdge := now.Unix() - p.lastNewEdgeTime.Unix()
		if edges > p.lastEdges {
			p.lastNewEdgeTime = now
			p.lastEdges = edges
			secondsSinceLastEdge = 0
		}
		return &report.FuzzingMetric{
			Timestamp:               time.Now(),
			ExecutionsPerSecond:     int32(execsPerSec),
			Features:                int32(features),
			Edges:                   int32(edges),
			CorpusSize:              int32(corpusSize),
			TotalExecutions:         totalExecs,
			SecondsSinceLastFeature: uint64(secondsSinceLastFeature),
			SecondsSinceLastEdge:    uint64(secondsSinceLastEdge),
		}
	}
	return nil
}

func parseAsSlowInput(log string) *report.Finding {
	if res, ok := regexutil.FindNamedGroupsMatch(slowInputPattern, log); ok {
		return &report.Finding{
			Type:    report.ErrorType_WARNING,
			Details: fmt.Sprintf("Slow input detected. Processing time: %s s", res["duration"]),
			Logs:    []string{fmt.Sprintf("Slow input: %s seconds for processing", res["duration"])},
			MoreDetails: &report.ErrorDetails{
				Id:   "Slow Input Detected",
				Name: "Slow Input Detected",
				Severity: &report.Severity{
					Description: "Low",
					Score:       2,
				},
			},
		}
	}
	return nil
}

func (p *parser) sendPendingFindingIfAny(ctx context.Context) error {
	if p.pendingFinding == nil {
		return nil
	}
	return p.sendPendingFinding(ctx)
}

func (p *parser) sendPendingFinding(ctx context.Context) error {
	p.pendingFinding.MoreDetails = &report.ErrorDetails{
		Id: errorid.ForFinding(p.pendingFinding),
	}
	err := p.sendFinding(ctx, p.pendingFinding)
	if err != nil {
		return err
	}
	p.pendingFinding = nil
	p.numMetricsLinesSinceFindingIsPending = 0
	return nil
}

func (p *parser) sendFinding(ctx context.Context, finding *report.Finding) error {
	log.Debug("Sending finding")
	p.FindingReported = true

	return p.sendReport(ctx, &report.Report{
		Status:  report.RunStatus_RUNNING,
		Finding: finding,
	})
}
