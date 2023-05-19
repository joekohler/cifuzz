package logging

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/pkg/errors"
	"github.com/spf13/viper"
)

// region Log

var buildLogPath string

func BuildOutputToFile(projectDir string, fuzzTestNames []string) (io.Writer, error) {
	logFile := fmt.Sprintf("build-%s.log", SuffixForLog(fuzzTestNames))
	logDir, err := CreateLogDir(projectDir)
	if err != nil {
		return nil, err
	}

	buildLogPath = filepath.Join(logDir, logFile)
	return os.OpenFile(buildLogPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0755)
}

func ShouldLogBuildToFile() bool {
	return !viper.GetBool("verbose")
}

// PrintBuildLogOnStdout reads the build log file and prints it
// on stdout.
func PrintBuildLogOnStdout() error {
	fmt.Println()

	data, err := os.ReadFile(buildLogPath)
	if err != nil {
		return errors.WithStack(err)
	}

	_, err = os.Stdout.Write(data)
	if err != nil {
		return errors.WithStack(err)
	}

	return nil
}

func GetMsgPathToBuildLog() string {
	return fmt.Sprintf("Details of the building process can be found here:\n%s\n", buildLogPath)
}
