package runner

import "code-intelligence.com/cifuzz/pkg/dependencies"

type BazelRunner struct {
}

func (r *BazelRunner) CheckDependencies(projectDir string) error {
	// All dependencies are managed via bazel but it should be checked
	// that the correct bazel version is installed
	return dependencies.Check([]dependencies.Key{
		dependencies.Bazel,
	}, projectDir)
}

func (r *BazelRunner) Run() error {

	return nil
}
