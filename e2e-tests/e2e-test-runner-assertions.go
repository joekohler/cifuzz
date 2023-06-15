package e2e

import "github.com/stretchr/testify/assert"

func (co *CommandOutput) Success() *CommandOutput {
	assert.EqualValues(co.t, 0, co.ExitCode)
	return co
}

func (co *CommandOutput) Failed() *CommandOutput {
	assert.NotEqualValues(co.t, 0, co.ExitCode)
	return co
}

func (co *CommandOutput) OutputContains(expected string) *CommandOutput {
	assert.Contains(co.t, co.Stdout, expected)
	return co
}

func (co *CommandOutput) OutputNotContains(expected string) *CommandOutput {
	assert.NotContains(co.t, co.Stdout, expected)
	return co
}

func (co *CommandOutput) ErrorContains(expected string) *CommandOutput {
	assert.Contains(co.t, co.Stderr, expected)
	return co
}

func (co *CommandOutput) NoOutput() *CommandOutput {
	assert.Empty(co.t, co.Stdout)
	return co
}

func (co *CommandOutput) NoError() *CommandOutput {
	assert.Empty(co.t, co.Stderr)
	return co
}
