package e2e

import (
	"testing"

	"code-intelligence.com/cifuzz/e2e-tests"
)

var containerRunTests = &[]e2e.TestCase{
	{
		Description: "container run command is not available without CIFUZZ_PRERELEASE flag",
		Command:     "container run",
		Assert: func(t *testing.T, output e2e.CommandOutput) {
			output.Failed().ErrorContains("unknown command \"container\" for \"cifuzz\"")
		},
	},
	{
		Description: "container run command is available in --help output with CIFUZZ_PRERELEASE flag",
		Command:     "container run",
		Args:        []string{"--help"},
		Environment: []string{"CIFUZZ_PRERELEASE=true"},
		Assert: func(t *testing.T, output e2e.CommandOutput) {
			output.Success().OutputContains("container")
		},
	},
	{
		Description:   "container run command in a maven/gradle example folder is available with CIFUZZ_PRERELEASE flag and fuzzer finds an RCE",
		Command:       "container run",
		Args:          []string{"com.example.FuzzTestCase::myFuzzTest"},
		Environment:   []string{"CIFUZZ_PRERELEASE=true"},
		SampleFolder:  []string{"examples/maven", "examples/gradle"},
		ToolsRequired: []string{"docker", "java", "maven"},
		SkipOnOS:      "windows",
		Assert: func(t *testing.T, output e2e.CommandOutput) {
			output.Success().OutputContains("Remote Code Execution in exploreMe")
		},
	},
}

func TestContainerRun(t *testing.T) {
	e2e.RunTests(t, *containerRunTests)
}
