package finding

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"code-intelligence.com/cifuzz/internal/cmdutils"
	"code-intelligence.com/cifuzz/internal/testutil"
	"code-intelligence.com/cifuzz/pkg/finding"
	"code-intelligence.com/cifuzz/pkg/log"
	"code-intelligence.com/cifuzz/util/fileutil"
	"code-intelligence.com/cifuzz/util/stringutil"
)

var logOutput io.ReadWriter

func TestMain(m *testing.M) {
	logOutput = bytes.NewBuffer([]byte{})
	log.Output = logOutput

	m.Run()
}

func TestFindingCmd_FailsIfNoCIFuzzProject(t *testing.T) {
	// Create an empty directory
	projectDir, err := os.MkdirTemp("", "test-findings-cmd-fails-")
	require.NoError(t, err)
	defer fileutil.Cleanup(projectDir)

	opts := &options{
		ProjectDir: projectDir,
		ConfigDir:  projectDir,
	}

	// Check that the command produces the expected error when not
	// called below a cifuzz project directory.
	_, err = cmdutils.ExecuteCommand(t, newWithOptions(opts), os.Stdin)
	require.Error(t, err)
	testutil.CheckOutput(t, logOutput, "Failed to parse cifuzz.yaml")
}

func TestListFindings(t *testing.T) {
	projectDir := testutil.BootstrapEmptyProject(t, "test-list-findings-")
	opts := &options{
		ProjectDir: projectDir,
		ConfigDir:  projectDir,
	}

	// Check that the command lists no findings in the empty project
	output, err := cmdutils.ExecuteCommand(t, newWithOptions(opts), os.Stdin, "--json")
	require.NoError(t, err)
	require.Equal(t, "[]", output)

	// Create a finding
	f := &finding.Finding{
		Name: "test_finding",
		MoreDetails: &finding.ErrorDetails{
			Name:         "test_finding",
			Severity:     nil,
			OwaspDetails: &finding.ExternalDetail{},
			CweDetails:   &finding.ExternalDetail{},
		},
	}
	err = f.Save(projectDir)
	require.NoError(t, err)

	// Check that the command lists the finding
	output, err = cmdutils.ExecuteCommand(t, newWithOptions(opts), os.Stdin, "--json")
	require.NoError(t, err)
	jsonString, err := stringutil.ToJSONString([]*finding.Finding{f})
	require.NoError(t, err)
	require.Equal(t, jsonString, output)
}

func TestPrintFinding(t *testing.T) {
	// Create a finding
	f := &finding.Finding{
		Name: "test_finding",
		MoreDetails: &finding.ErrorDetails{
			Name:         "test_finding",
			Severity:     nil,
			OwaspDetails: &finding.ExternalDetail{},
			CweDetails:   &finding.ExternalDetail{},
		},
	}

	projectDir := testutil.BootstrapEmptyProject(t, "test-list-findings-")
	opts := &options{
		ProjectDir: projectDir,
		ConfigDir:  projectDir,
	}

	// Check that the command produces the expected error when the
	// specified finding does not exist
	_, err := cmdutils.ExecuteCommand(t, newWithOptions(opts), os.Stdin, f.Name, "--json")
	require.Error(t, err)
	testutil.CheckOutput(t, logOutput, fmt.Sprintf("Finding %s does not exist", f.Name))

	// Create the finding
	err = f.Save(projectDir)
	require.NoError(t, err)

	// Check that the command prints the finding
	output, err := cmdutils.ExecuteCommand(t, newWithOptions(opts), os.Stdin, f.Name, "--json")
	require.NoError(t, err)
	jsonString, err := stringutil.ToJSONString(f)
	require.NoError(t, err)
	require.Equal(t, jsonString, output)
}
