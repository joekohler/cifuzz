package main

import (
	"fmt"
	"os"
	"os/exec"
	"regexp"

	"github.com/Masterminds/semver"
	"github.com/pkg/errors"
	"github.com/spf13/pflag"

	"code-intelligence.com/cifuzz/pkg/log"
)

func main() {

	flags := pflag.NewFlagSet("updater", pflag.ExitOnError)
	deps := flags.String("dependency", "", "which dependency to update eg. gradle-plugin, jazzerjs")
	version := flags.String("version", "", "target version to update to, for example 1.2.3")

	if err := flags.Parse(os.Args); err != nil {
		log.Error(errors.WithStack(err))
		os.Exit(1)
	}

	_, err := semver.NewVersion(*version)
	if err != nil {
		log.Error(errors.WithStack(err))
		os.Exit(1)
	}

	switch *deps {
	case "gradle-plugin":
		updateGradlePluginVersion("examples/gradle/build.gradle", *version)
		updateGradlePluginVersion("examples/gradle-kotlin/build.gradle.kts", *version)
		updateGradlePluginVersion("examples/gradle-multi/testsuite/build.gradle.kts", *version)
		updateGradlePluginVersion("pkg/messaging/instructions/gradle", *version)
		updateGradlePluginVersion("pkg/messaging/instructions/gradlekotlin", *version)
		updateGradlePluginVersion("internal/bundler/testdata/jazzer/gradle/multi-custom/testsuite/build.gradle.kts", *version)
	case "jazzerjs":
		updateJazzerNpm("examples/nodejs", *version)
		updateJazzerNpm("examples/nodejs-typescript", *version)
	default:
		log.Error(errors.New("unsupported dependency selected"))
		os.Exit(1)
	}
}

func updateJazzerNpm(path string, version string) {
	cmd := exec.Command("npm", "install", "--save-dev", fmt.Sprintf("@jazzer.js/jest-runner@%s", version))
	cmd.Dir = path
	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stdout
	err := cmd.Run()
	if err != nil {
		log.Error(err)
		os.Exit(1)
	}
}

func updateGradlePluginVersion(path string, version string) {
	content, err := os.ReadFile(path)
	if err != nil {
		log.Error(err)
		os.Exit(1)
	}
	buildFile := string(content)

	re := regexp.MustCompile(`("com.code-intelligence.cifuzz"\)? version ")(?P<version>\d+.\d+.\d+.*)"`)
	s := re.ReplaceAllString(buildFile, fmt.Sprintf(`${1}%s"`, version))

	err = os.WriteFile(path, []byte(s), 0x644)
	if err != nil {
		log.Error(err)
		os.Exit(1)
	}

	fmt.Printf("updated %s to %s\n", path, version)
}
