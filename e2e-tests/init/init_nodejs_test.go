package e2e

import (
	"io/fs"
	"testing"

	"github.com/stretchr/testify/assert"

	"code-intelligence.com/cifuzz/e2e-tests"
)

var nodeInitTests = &[]e2e.TestCase{
	{
		Description:  "init command in Node.js project without prerelease flag prints error",
		Command:      "init",
		Args:         []string{"js"},
		SampleFolder: []string{"nodejs"},
		Assert: func(t *testing.T, output e2e.CommandOutput) {
			assert.EqualValues(t, 1, output.ExitCode)
			matches, _ := fs.Glob(output.Workdir, "cifuzz.yaml")
			assert.Len(t, matches, 0, "There shouldn't be a cifuzz.yaml config")
		},
	},
	{
		Description:  "init command in Node.js (JS) project with prerelease flag succeeds and creates a config file",
		Command:      "init",
		Args:         []string{"js"},
		Environment:  []string{"CIFUZZ_PRERELEASE=true"},
		SampleFolder: []string{"nodejs"},
		Assert: func(t *testing.T, output e2e.CommandOutput) {
			assert.EqualValues(t, 0, output.ExitCode)
			assert.Contains(t, output.Stdall, "To use jazzer.js, add a dev-dependency to @jazzer.js/jest-runner")
			assert.Contains(t, output.Stdall, "Configuration saved in cifuzz.yaml")
			assert.NotContains(t, output.Stdall, "Failed to create config")
			matches, _ := fs.Glob(output.Workdir, "cifuzz.yaml")
			assert.Len(t, matches, 1, "There should be a cifuzz.yaml config")
		},
	},
	{
		Description:  "init command in Node.js (TS) project with prerelease flag succeeds and creates a config file",
		Command:      "init",
		Args:         []string{"ts"},
		Environment:  []string{"CIFUZZ_PRERELEASE=true"},
		SampleFolder: []string{"node-typescript"},
		Assert: func(t *testing.T, output e2e.CommandOutput) {
			assert.EqualValues(t, 0, output.ExitCode)
			assert.Contains(t, output.Stdall, "To use jazzer.js, add a dev-dependency to @jazzer.js/jest-runner")
			assert.Contains(t, output.Stdall, "'jest.config.ts'")
			assert.Contains(t, output.Stdall, "To introduce the fuzz function types globally, add the following import to globals.d.ts:")
			assert.Contains(t, output.Stdall, "Configuration saved in cifuzz.yaml")
			assert.NotContains(t, output.Stdall, "Failed to create config")
			matches, _ := fs.Glob(output.Workdir, "cifuzz.yaml")
			assert.Len(t, matches, 1, "There should be a cifuzz.yaml config")
		},
	},
}

func TestInitForNodejs(t *testing.T) {
	e2e.RunTests(t, *nodeInitTests)
}
