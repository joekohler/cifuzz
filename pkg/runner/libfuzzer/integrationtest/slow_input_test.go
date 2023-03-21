package integrationtest

import (
	"runtime"
	"testing"

	"code-intelligence.com/cifuzz/pkg/finding"
)

func TestIntegration_SlowInput(t *testing.T) {
	if testing.Short() || runtime.GOOS == "windows" {
		t.Skip()
	}
	t.Parallel()

	buildDir := BuildFuzzTarget(t, "trigger_slow_input")

	TestWithAndWithoutMinijail(t, func(t *testing.T, disableMinijail bool) {
		test := NewLibfuzzerTest(t, buildDir, "trigger_slow_input", disableMinijail)
		// The input timeout should be reported on the first input
		test.RunsLimit = 1
		test.EngineArgs = append(test.EngineArgs, "-report_slow_units=1")

		_, reports := test.Run(t)

		CheckReports(t, reports, &CheckReportOptions{
			ErrorType:   finding.ErrorTypeWarning,
			Details:     "Slow input detected",
			NumFindings: 1,
		})
	})
}
