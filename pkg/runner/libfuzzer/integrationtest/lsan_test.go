package integrationtest

import (
	"runtime"
	"testing"

	"code-intelligence.com/cifuzz/pkg/finding"
)

func TestIntegration_LSAN(t *testing.T) {
	if testing.Short() || runtime.GOOS == "windows" {
		t.Skip()
	}
	t.Parallel()

	buildDir := BuildFuzzTarget(t, "trigger_lsan")

	TestWithAndWithoutMinijail(t, func(t *testing.T, disableMinijail bool) {
		if runtime.GOOS == "windows" {
			t.Skip()
		}

		test := NewLibfuzzerTest(t, buildDir, "trigger_lsan", disableMinijail)
		test.FuzzerEnv = []string{"ASAN_OPTIONS=detect_leaks=1"}

		_, reports := test.Run(t)

		CheckReports(t, reports, &CheckReportOptions{
			ErrorType:   finding.ErrorTypeCrash,
			SourceFile:  "trigger_lsan.c",
			Details:     "detected memory leaks",
			NumFindings: 1,
		})
	})
}
