package integrationtest

import (
	"runtime"
	"testing"

	"code-intelligence.com/cifuzz/pkg/finding"
)

func TestIntegration_OOM(t *testing.T) {
	if testing.Short() || runtime.GOOS == "windows" {
		t.Skip()
	}
	t.Parallel()

	buildDir := BuildFuzzTarget(t, "trigger_oom")

	TestWithAndWithoutMinijail(t, func(t *testing.T, disableMinijail bool) {
		test := NewLibfuzzerTest(t, buildDir, "trigger_oom", disableMinijail)
		test.EngineArgs = append(test.EngineArgs, "-malloc_limit_mb=1")

		_, reports := test.Run(t)

		CheckReports(t, reports, &CheckReportOptions{
			ErrorType:   finding.ErrorTypeCrash,
			SourceFile:  "trigger_oom.cpp",
			Details:     "out-of-memory",
			NumFindings: 1,
		})

		// We don't check here that the seed corpus is non-empty because
		// the trigger_oom fuzz target triggers the OOM immediately, so
		// that no interesting inputs can be tested and stored before
		// the crash is triggered.
	})
}
