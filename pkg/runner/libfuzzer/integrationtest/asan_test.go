package integrationtest

import (
	"runtime"
	"testing"

	"code-intelligence.com/cifuzz/pkg/finding"
)

func TestIntegration_ASAN(t *testing.T) {
	if testing.Short() || runtime.GOOS == "windows" {
		t.Skip()
	}
	t.Parallel()

	buildDir := BuildFuzzTarget(t, "trigger_asan")

	TestWithAndWithoutMinijail(t, func(t *testing.T, disableMinijail bool) {
		test := NewLibfuzzerTest(t, buildDir, "trigger_asan", disableMinijail)

		_, reports := test.Run(t)

		CheckReports(t, reports, &CheckReportOptions{
			ErrorType:   finding.ErrorTypeCrash,
			SourceFile:  "trigger_asan.c",
			Details:     "heap-buffer-overflow",
			NumFindings: 1,
		})
	})
}
