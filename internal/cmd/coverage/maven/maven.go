package maven

import (
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/pkg/errors"

	"code-intelligence.com/cifuzz/internal/build/maven"
	"code-intelligence.com/cifuzz/internal/cmd/coverage/summary"
	"code-intelligence.com/cifuzz/internal/cmdutils"
	"code-intelligence.com/cifuzz/internal/coverage"
	"code-intelligence.com/cifuzz/pkg/log"
	"code-intelligence.com/cifuzz/pkg/runfiles"
	"code-intelligence.com/cifuzz/util/executil"
	"code-intelligence.com/cifuzz/util/stringutil"
)

type CoverageGenerator struct {
	OutputFormat string
	OutputPath   string
	FuzzTest     string
	ProjectDir   string
	EngineArgs   []string

	Parallel maven.ParallelOptions

	Stderr      io.Writer
	BuildStdout io.Writer
	BuildStderr io.Writer

	runfilesFinder runfiles.RunfilesFinder
}

func (cov *CoverageGenerator) BuildFuzzTestForCoverage() error {
	// ensure a finder is set
	if cov.runfilesFinder == nil {
		cov.runfilesFinder = runfiles.Finder
	}

	// Maven tests fail if fuzz tests fail, so we ignore the error here,
	// so we can still generate the coverage report
	mavenTestArgs := []string{"-Dmaven.test.failure.ignore=true"}

	// Flags for jazzer
	mavenTestArgs = append(mavenTestArgs, "-Djazzer.hooks=false")
	mavenTestArgs = append(mavenTestArgs, cov.EngineArgs...)

	// Flags for cifuzz
	mavenTestArgs = append(mavenTestArgs,
		"-Pcifuzz",
		fmt.Sprintf("-Dtest=%s", cov.FuzzTest),
		"test")

	if cov.Parallel.Enabled {
		mavenTestArgs = append(mavenTestArgs, "-T")
		if cov.Parallel.NumJobs != 0 {
			mavenTestArgs = append(mavenTestArgs, fmt.Sprint(cov.Parallel.NumJobs))
		} else {
			// Use one thread per cpu core
			mavenTestArgs = append(mavenTestArgs, "1C")
		}
	}
	err := cov.runMavenCommand(mavenTestArgs)
	if err != nil {
		return err
	}

	if cov.OutputPath == "" {
		// We are using the .cifuzz-build directory
		// because the build directory is unknown at this point
		cov.OutputPath = filepath.Join(cov.ProjectDir, ".cifuzz-build", "report")
	}
	mavenReportArgs := []string{
		"-Pcifuzz",
		"jacoco:report",
		fmt.Sprintf("-Dcifuzz.report.output=%s", cov.OutputPath),
	}

	if cov.OutputFormat == coverage.FormatJacocoXML {
		mavenReportArgs = append(mavenReportArgs, "-Dcifuzz.report.format=XML")
	} else {
		mavenReportArgs = append(mavenReportArgs, "-Dcifuzz.report.format=XML,HTML")
	}

	return cov.runMavenCommand(mavenReportArgs)
}

func (cov *CoverageGenerator) GenerateCoverageReport() (string, error) {
	reportPath := filepath.Join(cov.OutputPath, "jacoco.xml")
	reportFile, err := os.Open(reportPath)
	if err != nil {
		return "", errors.WithStack(err)
	}
	defer reportFile.Close()
	summary.ParseJacocoXML(reportFile).PrintTable(cov.Stderr)

	if cov.OutputFormat == coverage.FormatJacocoXML {
		return filepath.Join(cov.OutputPath, "jacoco.xml"), nil
	}

	return cov.OutputPath, nil
}

func (cov *CoverageGenerator) runMavenCommand(args []string) error {
	mavenCmd, err := cov.runfilesFinder.MavenPath()
	if err != nil {
		return err
	}

	cmdArgs := []string{mavenCmd}
	cmdArgs = append(cmdArgs, args...)

	cmd := executil.Command(cmdArgs[0], cmdArgs[1:]...)
	cmd.Dir = cov.ProjectDir
	cmd.Stdout = cov.BuildStdout
	cmd.Stderr = cov.BuildStderr
	log.Debugf("Running maven command: %s", strings.Join(stringutil.QuotedStrings(cmd.Args), " "))

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, os.Interrupt, syscall.SIGTERM, syscall.SIGINT, syscall.SIGQUIT)
	go func() {
		_ = <-sigs
		err = cmd.TerminateProcessGroup()
		if err != nil {
			log.Error(err, err.Error())
		}
	}()

	err = cmd.Run()
	if err != nil {
		return cmdutils.WrapExecError(errors.WithStack(err), cmd.Cmd)
	}
	return nil
}
