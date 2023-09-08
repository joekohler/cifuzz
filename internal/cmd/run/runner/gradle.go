package runner

import "code-intelligence.com/cifuzz/pkg/dependencies"

type GradleRunner struct {
}

func (r *GradleRunner) CheckDependencies(projectDir string) error {
	return dependencies.Check([]dependencies.Key{
		dependencies.Java,
		dependencies.Gradle,
	}, projectDir)
}

func (r *GradleRunner) Run() error {

	return nil
}
