package shared

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"code-intelligence.com/cifuzz/pkg/parser/coverage"
	"code-intelligence.com/cifuzz/util/executil"
)

func TestAdditionalJazzerParameters(t *testing.T, cifuzz, dir string) {
	t.Helper()

	testCases := map[string]struct {
		cmd *executil.Cmd
		arg string
	}{
		"run": {
			cmd: executil.Command(cifuzz,
				"run",
				"-v",
				"--interactive=false",
				"com.example.FuzzTestCase"),
			arg: "--instrumentation_includes=com.**",
		},
		"coverage": {
			cmd: executil.Command(cifuzz,
				"coverage",
				"-v",
				"--output=report",
				"com.example.FuzzTestCase"),
			arg: "-Djazzer.instrumentation_includes=com.**",
		},
		"bundle": {
			cmd: executil.Command(cifuzz,
				"bundle",
				"-v",
				"com.example.FuzzTestCase"),
			arg: "-Djazzer.instrumentation_includes=com.**",
		},
	}

	for _, tc := range testCases {
		tc.cmd.Dir = dir
		tc.cmd.Args = append(tc.cmd.Args, "--engine-arg="+tc.arg)
		out, err := tc.cmd.CombinedOutput()
		require.NoError(t, err)

		lines := strings.Split(string(out), "\n")
		for _, l := range lines {
			if strings.Contains(l, "com.code_intelligence.jazzer.Jazzer") {
				assert.Contains(t, l, tc.arg, "java call does not include jazzer flag")
				break
			}
		}
	}
}

func TestLCOVCoverageReport(t *testing.T, cifuzz, dir string) {
	cmd := executil.Command(cifuzz, "coverage", "-v",
		"--output", "report", "--format", "lcov", "com.example.FuzzTestCase::myFuzzTest")
	cmd.Dir = dir

	output, err := cmd.CombinedOutput()
	require.NoError(t, err)
	assert.Contains(t, string(output),
		fmt.Sprintf("Created coverage lcov report: %s", filepath.Join("report", "report.lcov")),
	)

	// Check that the coverage report was created
	reportPath := filepath.Join(dir, "report", "report.lcov")
	require.FileExists(t, reportPath)

	// Check that the coverage report contains coverage for
	// ExploreMe.java source file, but not for App.java.
	reportFile, err := os.Open(reportPath)
	require.NoError(t, err)
	defer reportFile.Close()
	summary, err := coverage.ParseLCOVReportIntoSummary(reportFile)
	require.NoError(t, err)
	for _, file := range summary.Files {
		if file.Filename == "com/example/ExploreMe.java" {
			assert.Equal(t, 2, file.Coverage.FunctionsHit)
			assert.Equal(t, 10, file.Coverage.LinesHit)
			assert.Equal(t, 8, file.Coverage.BranchesHit)

		} else if file.Filename == "com/example/App.java" {
			assert.Equal(t, 0, file.Coverage.FunctionsHit)
			assert.Equal(t, 0, file.Coverage.LinesHit)
			assert.Equal(t, 0, file.Coverage.BranchesHit)
		}
	}
}
