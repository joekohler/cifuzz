package integrationtest

import (
	"runtime"
	"testing"

	"code-intelligence.com/cifuzz/pkg/finding"
)

func TestIntegration_UBSANRecoverable(t *testing.T) {
	if testing.Short() || runtime.GOOS == "windows" {
		t.Skip()
	}
	t.Parallel()

	buildDir := BuildFuzzTarget(t, "trigger_ubsan")

	TestWithAndWithoutMinijail(t, func(t *testing.T, disableMinijail bool) {
		test := NewLibfuzzerTest(t, buildDir, "trigger_ubsan", disableMinijail)

		_, reports := test.Run(t)

		CheckReports(t, reports, &CheckReportOptions{
			ErrorType:           finding.ErrorTypeRuntimeError,
			Details:             "undefined behavior",
			SourceFile:          "trigger_ubsan.cpp",
			AllowEmptyInputData: runtime.GOOS == "windows",
			NumFindings:         1,
		})

		// We don't check here that the seed corpus is non-empty because
		// the fuzz target triggers the undefined behavior immediately,
		// so that no interesting inputs can be tested and stored before
		// the crash is triggered.
	})
}
