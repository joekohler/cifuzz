package runner

import (
	"runtime"

	"code-intelligence.com/cifuzz/pkg/dependencies"
)

type CMakeRunner struct {
}

func (r *CMakeRunner) CheckDependencies(projectDir string) error {
	var deps []dependencies.Key
	deps = []dependencies.Key{
		dependencies.CMake,
		dependencies.LLVMSymbolizer,
	}
	switch runtime.GOOS {
	case "linux", "darwin":
		deps = append(deps, dependencies.Clang)
	case "windows":
		deps = append(deps, dependencies.VisualStudio)
	}

	return dependencies.Check(deps, projectDir)
}

func (r *CMakeRunner) Run() error {

	return nil
}
