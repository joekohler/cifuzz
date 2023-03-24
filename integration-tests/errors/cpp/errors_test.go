package cpp

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"code-intelligence.com/cifuzz/integration-tests/shared"
	builderPkg "code-intelligence.com/cifuzz/internal/builder"
	"code-intelligence.com/cifuzz/internal/testutil"
	"code-intelligence.com/cifuzz/pkg/finding"
	"code-intelligence.com/cifuzz/util/fileutil"
)

func TestIntegration_CPPErrors(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}

	// Install cifuzz
	testutil.RegisterTestDepOnCIFuzz()
	installDir := shared.InstallCIFuzzInTemp(t)
	t.Cleanup(func() { fileutil.Cleanup(installDir) })
	cifuzz := builderPkg.CIFuzzExecutablePath(filepath.Join(installDir, "bin"))
	t.Setenv("CMAKE_PREFIX_PATH", filepath.Join(installDir, "share", "cmake"))

	// Copy testdata to tmp dir to avoid creating cifuzz folders in repo while testing
	testdataTmp := shared.CopyTestdataDir(t, "errors_cpp")
	t.Cleanup(func() { fileutil.Cleanup(testdataTmp) })

	cifuzzRunner := &shared.CIFuzzRunner{
		CIFuzzPath:     cifuzz,
		DefaultWorkDir: testdataTmp,
	}

	// TODO: missing odr violation
	//  + deadly signal is commented out since it doesn't trigger on macos
	testCases := []struct {
		id   string
		env  []string
		args []string
	}{
		{id: "alloc_dealloc_mismatch"},
		{id: "double_free"},
		//{id: "deadly_signal"},
		{id: "global_buffer_overflow"},
		{id: "heap_buffer_overflow"},
		{id: "heap_use_after_free"},
		{
			id:  "memory_leak",
			env: []string{"ASAN_OPTIONS=detect_leaks=1"},
		},
		{
			id:  "out_of_bounds",
			env: []string{"UBSAN_OPTIONS=halt_on_error=1"},
		},
		{id: "out_of_memory"},
		{id: "segmentation_fault"},
		{
			id:   "slow_input",
			args: []string{"--engine-arg=-report_slow_units=1", "--engine-arg=-timeout=3s", "--timeout=3s"},
		},
		{id: "stack_buffer_overflow"},
		{id: "stack_exhaustion"},
		{
			id:   "timeout",
			args: []string{"--engine-arg=-timeout=1s", "--engine-arg=-runs=-1"},
		},
		{
			id:  "use_after_return",
			env: []string{"ASAN_OPTIONS=detect_stack_use_after_return=1"},
		},
		{id: "use_after_scope"},
	}

	for _, tc := range testCases {
		t.Run(tc.id, func(t *testing.T) {
			// Run the test
			runArgs := append([]string{
				"--interactive=false",
				"--no-notifications",
				"--use-sandbox=false",
			}, tc.args...)
			cifuzzRunner.Run(t, &shared.RunOptions{
				FuzzTest: fmt.Sprintf("%s_fuzztest", tc.id),
				Env:      append(os.Environ(), tc.env...),
				Args:     runArgs,
			})

			// Call findings command, get json output and check for finding id
			_, findingsJSON := cifuzzRunner.CommandOutput(t, "findings", &shared.CommandOptions{
				Args: []string{
					"--json",
					"--interactive=false",
				},
			})

			var findings []finding.Finding
			err := json.Unmarshal([]byte(findingsJSON), &findings)
			require.NoError(t, err)
			idFound := false
			for _, f := range findings {
				if f.MoreDetails.ID == tc.id {
					idFound = true
					break
				}
			}
			assert.True(t, idFound)
		})
	}
}
