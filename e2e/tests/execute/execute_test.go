package execute_test

import (
	"testing"

	"code-intelligence.com/cifuzz/e2e"
)

var executeTests = &[]e2e.TestCase{
	{
		Description: "execute command is not available without CIFUZZ_PRERELEASE flag",
		Command:     "execute",
		Assert: func(t *testing.T, output e2e.CommandOutput) {
			output.Failed().ErrorContains("unknown command \"execute\" for \"cifuzz\"")
		},
	},
	{
		Description: "execute command is not available in --help output without CIFUZZ_PRERELEASE flag",
		Command:     "--help",
		Assert: func(t *testing.T, output e2e.CommandOutput) {
			output.Success().OutputNotContains("execute")
		},
	},
	{
		Description: "execute command is available in --help output with CIFUZZ_PRERELEASE flag",
		Command:     "--help",
		Environment: []string{"CIFUZZ_PRERELEASE=true"},
		Assert: func(t *testing.T, output e2e.CommandOutput) {
			output.Success().OutputContains("execute")
		},
	},
	{
		Description:  "execute command in a folder with bundle contents is available with CIFUZZ_PRERELEASE flag and prints a helpful message",
		Command:      "execute",
		Environment:  []string{"CIFUZZ_PRERELEASE=true"},
		SampleFolder: []string{"folder-with-unpacked-bundle"},
		Assert: func(t *testing.T, output e2e.CommandOutput) {
			output.Success().OutputContains("This container is based on:")
			output.Success().OutputContains("Available fuzzers:")
		},
	},
	// { // TODO: this is problematic, because from the nature of this command, we expect specific dependencies to be installed
	// // TODO: We will run into same thing with testing the cifuzz run command too
	// 	Description:  "execute command with a fuzz test argument in a folder with bundle contents runs the fuzz test",
	// 	Command:      "execute",
	// 	Args:         []string{"com.example.FuzzTestCase"},
	// 	Environment:  []string{"CIFUZZ_PRERELEASE=true"},
	// 	SampleFolder: []string{"folder-with-unpacked-bundle"},
	// 	Assert: func(t *testing.T, output e2e.CommandOutput) {
	// 		// TODO: should fail! Execute doesn't respect the libfuzzer findings today
	// 		output.Success().ErrorContains("Security Issue: Remote Code Execution in exploreMe (com.example.ExploreMe:19)")
	// 	},
	// },
	{
		Description:  "execute command with an invalid fuzz test argument in a folder with bundle contents fails",
		Command:      "execute",
		Args:         []string{"invalid.name"},
		Environment:  []string{"CIFUZZ_PRERELEASE=true"},
		SampleFolder: []string{"folder-with-unpacked-bundle"},
		Assert: func(t *testing.T, output e2e.CommandOutput) {
			output.Failed().ErrorContains("fuzzer 'invalid.name' not found in a bundle metadata file")
		},
	},
}

func TestExecute(t *testing.T) {
	e2e.RunTests(t, *executeTests)
}
