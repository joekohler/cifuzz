package runner

type Runner interface {
	Run() error
	CheckDependencies(string) error
}
