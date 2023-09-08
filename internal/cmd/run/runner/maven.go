package runner

import "code-intelligence.com/cifuzz/pkg/dependencies"

type MavenRunner struct {
}

func (r *MavenRunner) CheckDependencies(projectDir string) error {
	return dependencies.Check([]dependencies.Key{
		dependencies.Java,
		dependencies.Maven,
	}, projectDir)
}

func (r *MavenRunner) Run() error {

	return nil
}
