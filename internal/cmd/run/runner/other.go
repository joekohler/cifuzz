package runner

import (
	"runtime"

	"code-intelligence.com/cifuzz/pkg/dependencies"
)

type OtherRunner struct {
}

func (r *OtherRunner) CheckDependencies(projectDir string) error {

	var deps []dependencies.Key
	switch runtime.GOOS {
	case "linux", "darwin":
		deps = []dependencies.Key{
			dependencies.Clang,
			dependencies.LLVMSymbolizer,
		}
	case "windows":
		deps = []dependencies.Key{
			dependencies.VisualStudio,
		}
	}
	return dependencies.Check(deps, projectDir)
}

func (r *OtherRunner) Run() error {

	return nil
}
