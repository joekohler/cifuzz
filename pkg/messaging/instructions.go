package messaging

import (
	_ "embed"

	"code-intelligence.com/cifuzz/internal/config"
)

//go:embed instructions/bazel
var bazelSetup string

//go:embed instructions/cmake
var cmakeSetup string

//go:embed instructions/maven
var mavenSetup string

//go:embed instructions/gradle
var gradleGroovySetup string

//go:embed instructions/gradlekotlin
var gradleKotlinSetup string

//go:embed instructions/nodejs
var nodejsSetup string

func Instructions(buildSystem string) string {
	switch buildSystem {
	case config.BuildSystemBazel:
		return bazelSetup
	case config.BuildSystemCMake:
		return cmakeSetup
	case config.BuildSystemNodeJS:
		return nodejsSetup
	case config.BuildSystemMaven:
		return mavenSetup
	case string(config.GradleGroovy):
		return gradleGroovySetup
	case string(config.GradleKotlin):
		return gradleKotlinSetup
	default:
		return ""
	}
}
