package runner

import "code-intelligence.com/cifuzz/pkg/dependencies"

type NodeJSRunner struct {
}

func (r *NodeJSRunner) CheckDependencies(projectDir string) error {
	return dependencies.Check([]dependencies.Key{
		dependencies.Node,
	}, projectDir)
}

func (r *NodeJSRunner) Run() error {

	return nil
}
