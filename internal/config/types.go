package config

import "os"

type FuzzTestType string

const (
	CPP        FuzzTestType = "cpp"
	Java       FuzzTestType = "java"
	Kotlin     FuzzTestType = "kotlin"
	JavaScript FuzzTestType = "js"
	TypeScript FuzzTestType = "ts"
	Rust       FuzzTestType = "rust"
)

// map of supported test types -> label:value
var supportedTestTypes = map[string]string{
	"C/C++":  string(CPP),
	"Java":   string(Java),
	"Kotlin": string(Kotlin),
}

type GradleBuildLanguage string

const (
	GradleGroovy GradleBuildLanguage = "groovy"
	GradleKotlin GradleBuildLanguage = "kotlin"
)

type Engine string

const (
	Libfuzzer Engine = "libfuzzer"
)

// SupportedTestTypes returns the supported test types depending on the
// environment variable CIFUZZ_PRERELEASE.
func SupportedTestTypes() map[string]string {
	if os.Getenv("CIFUZZ_PRERELEASE") != "" {
		supportedTestTypes["JavaScript"] = string(JavaScript)
		supportedTestTypes["TypeScript"] = string(TypeScript)
		supportedTestTypes["Rust"] = string(Rust)
	}
	return supportedTestTypes
}
