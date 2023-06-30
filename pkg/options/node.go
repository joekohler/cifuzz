package options

import (
	"fmt"
)

const NodeTestNamePattern string = "--testNamePattern"
const NodeTestPathPattern string = "--testPathPattern"

func JazzerJSTestNamePatternFlag(value string) string {
	return NodeTestNamePattern + fmt.Sprintf("='%s'", value)
}

func JazzerJSTestPathPatternFlag(value string) string {
	return NodeTestPathPattern + fmt.Sprintf("='%s'", value)
}
