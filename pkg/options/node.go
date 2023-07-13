package options

import (
	"fmt"
)

const JazzerJSTestNamePattern string = "--testNamePattern"
const JazzerJSTestPathPattern string = "--testPathPattern"
const JazzerJSCoverageDirectory string = "--coverageDirectory"
const JazzerJSCoverageReporters string = "--coverageReporters"

func JazzerJSTestNamePatternFlag(value string) string {
	return JazzerJSTestNamePattern + fmt.Sprintf("='%s'", value)
}

func JazzerJSTestPathPatternFlag(value string) string {
	return JazzerJSTestPathPattern + fmt.Sprintf("='%s'", value)
}

func JazzerJSCoverageDirectoryFlag(value string) string {
	return JazzerJSCoverageDirectory + fmt.Sprintf("='%s'", value)
}

func JazzerJSCoverageReportersFlag(value string) string {
	return JazzerJSCoverageReporters + fmt.Sprintf("='%s'", value)
}
