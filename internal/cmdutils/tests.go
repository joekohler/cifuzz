package cmdutils

import (
	"bytes"
	"io"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"code-intelligence.com/cifuzz/pkg/log"
)

func ExecuteCommand(t *testing.T, cmd *cobra.Command, in io.Reader, args ...string) (string, string, error) {
	t.Helper()

	errBuf := new(bytes.Buffer)
	outBuf := new(bytes.Buffer)
	// use buffer for cobra out/err
	cmd.SetOut(outBuf)
	cmd.SetErr(errBuf)
	// send output of log package to buffer
	log.Output = errBuf
	cmd.SetIn(in)
	cmd.SetArgs(args)

	err := cmd.Execute()
	// trim outBuf to avoid line breaks in json output
	return strings.TrimSpace(outBuf.String()), errBuf.String(), err
}
