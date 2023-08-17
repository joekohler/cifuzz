package logging

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/pkg/errors"
	"github.com/pterm/pterm"
	"github.com/spf13/viper"

	"code-intelligence.com/cifuzz/pkg/log"
)

var buildLogPath string

type BuildPrinter struct {
	spinnerPrinter *log.SpinnerPrinter
	output         io.Writer
}

func NewBuildPrinter(output io.Writer, msg string) *BuildPrinter {
	if !ShouldLogBuildToFile() {
		return nil
	}

	buildPrinter := &BuildPrinter{output: output}

	if log.ShouldUseSpinnerPrinter() {
		buildPrinter.spinnerPrinter = log.NewSpinnerPrinter(nil, output, msg)
	}

	return buildPrinter
}

func (p *BuildPrinter) StopOnSuccess(msg string, printPath bool) {
	if p == nil {
		return
	}

	if p.spinnerPrinter != nil {
		p.spinnerPrinter.Style = ptermSuccessStyle()
		p.spinnerPrinter.StopWithMessage(msg)
	}

	if printPath {
		log.Info(fmt.Sprintf("Details of the building process can be found here:\n%s\n", buildLogPath))
	}
}

func (p *BuildPrinter) StopOnError(msg string) {
	if p == nil {
		return
	}

	if p.spinnerPrinter != nil {
		p.spinnerPrinter.Style = ptermErrorStyle()
		p.spinnerPrinter.StopWithMessage(msg)
	}

	printErr := p.printBuildLog()
	if printErr != nil {
		log.Error(printErr)
	}
}

func (p *BuildPrinter) printBuildLog() error {
	_, _ = fmt.Fprintln(p.output)

	data, err := os.ReadFile(buildLogPath)
	if err != nil {
		return errors.WithStack(err)
	}

	_, err = p.output.Write(data)
	if err != nil {
		return errors.WithStack(err)
	}

	return nil
}

func BuildOutputToFile(projectDir string, fuzzTestNames []string) (io.Writer, error) {
	logFile := fmt.Sprintf("build-%s.log", SuffixForLog(fuzzTestNames))
	logDir, err := CreateLogDir(projectDir)
	if err != nil {
		return nil, err
	}

	buildLogPath = filepath.Join(logDir, logFile)
	var writer io.Writer
	writer, err = os.OpenFile(buildLogPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0755)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	return writer, nil
}

func ShouldLogBuildToFile() bool {
	return !viper.GetBool("verbose")
}

func ptermErrorStyle() *pterm.Style {
	return &pterm.Style{pterm.FgRed, pterm.Bold}
}

func ptermSuccessStyle() *pterm.Style {
	return &pterm.Style{pterm.FgGreen}
}
