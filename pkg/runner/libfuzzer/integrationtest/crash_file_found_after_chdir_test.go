package integrationtest

import (
	"runtime"
	"testing"

	"code-intelligence.com/cifuzz/pkg/finding"
)

func TestIntegration_CrashFileFoundAfterChdir(t *testing.T) {
	if testing.Short() || runtime.GOOS == "windows" {
		t.Skip()
	}
	t.Parallel()

	buildDir := BuildFuzzTarget(t, "trigger_asan_after_chdir")

	TestWithAndWithoutMinijail(t, func(t *testing.T, disableMinijail bool) {
		test := NewLibfuzzerTest(t, buildDir, "trigger_asan_after_chdir", disableMinijail)

		_, reports := test.Run(t)

		CheckReports(t, reports, &CheckReportOptions{
			ErrorType:   finding.ErrorTypeCrash,
			SourceFile:  "trigger_asan_after_chdir.c",
			Details:     "heap-buffer-overflow",
			NumFindings: 1,
		})
	})
}
