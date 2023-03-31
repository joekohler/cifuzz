package main

import (
	"fmt"
	"log"
	"os"
	"regexp"
)

const GradlePluginVersion = "1.0.0"

func main() {
	updateGradlePluginVersion("examples/gradle/build.gradle")
	updateGradlePluginVersion("examples/gradle-kotlin/build.gradle.kts")
	updateGradlePluginVersion("examples/gradle-multi/testsuite/build.gradle.kts")
	updateGradlePluginVersion("pkg/messaging/instructions/gradle")
	updateGradlePluginVersion("pkg/messaging/instructions/gradlekotlin")
}

func updateGradlePluginVersion(path string) {
	content, err := os.ReadFile(path)
	if err != nil {
		log.Fatal(err)
	}
	buildFile := string(content)

	re := regexp.MustCompile(`("com.code-intelligence.cifuzz"\)? version ")(?P<version>\d+.\d+.\d+.*)"`)
	s := re.ReplaceAllString(buildFile, fmt.Sprintf(`${1}%s"`, GradlePluginVersion))

	err = os.WriteFile(path, []byte(s), 0x644)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("updated %s to %s\n", path, GradlePluginVersion)

}
