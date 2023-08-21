package container_test

import (
	"testing"

	"code-intelligence.com/cifuzz/e2e"
)

var containerTests = &[]e2e.TestCase{
	{
		Description: "container command is not available without CIFUZZ_PRERELEASE flag",
		Command:     "container",
		Assert: func(t *testing.T, output e2e.CommandOutput) {
			output.Failed().ErrorContains("unknown command \"container\" for \"cifuzz\"")
		},
	},
	{
		Description: "container command is not available in --help output without CIFUZZ_PRERELEASE flag",
		Command:     "--help",
		Assert: func(t *testing.T, output e2e.CommandOutput) {
			output.Success().OutputNotContains("container")
		},
	},
	{
		Description: "container command is available in --help output with CIFUZZ_PRERELEASE flag",
		Command:     "--help",
		Environment: []string{"CIFUZZ_PRERELEASE=true"},
		Assert: func(t *testing.T, output e2e.CommandOutput) {
			output.Success().OutputContains("container")
		},
	},
	{
		Description:  "container command in a project folder is available with CIFUZZ_PRERELEASE flag",
		Command:      "container",
		Environment:  []string{"CIFUZZ_PRERELEASE=true"},
		SampleFolder: []string{"project-with-empty-cifuzz-yaml"},
		Assert: func(t *testing.T, output e2e.CommandOutput) {
			output.Success().OutputContains("cifuzz container")
		},
	},
}

func TestContainer(t *testing.T) {
	e2e.RunTests(t, *containerTests)
}
