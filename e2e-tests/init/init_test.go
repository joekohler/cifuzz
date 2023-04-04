package e2e

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"code-intelligence.com/cifuzz/e2e-tests"
)

var initTests = &[]e2e.TestCase{
	{
		Description:  "init command in empty CMake project succeeds and creates a config file",
		Command:      "init",
		SampleFolder: []string{"cmake"},
		Assert: func(t *testing.T, output e2e.CommandOutput) {
			assert.EqualValues(t, 0, output.ExitCode)
			assert.Contains(t, output.Stdall, "Configuration saved in cifuzz.yaml")
		},
	},
}

func TestInit(t *testing.T) {
	e2e.RunTests(t, *initTests)
}
