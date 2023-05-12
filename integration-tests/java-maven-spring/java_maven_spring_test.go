package spring

import (
	"path/filepath"
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"

	"code-intelligence.com/cifuzz/integration-tests/shared"
	builderPkg "code-intelligence.com/cifuzz/internal/builder"
	"code-intelligence.com/cifuzz/internal/testutil"
	"code-intelligence.com/cifuzz/util/fileutil"
)

func TestIntegration_MavenSpring(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}

	testutil.RegisterTestDepOnCIFuzz()
	installDir := shared.InstallCIFuzzInTemp(t)
	cifuzz := builderPkg.CIFuzzExecutablePath(filepath.Join(installDir, "bin"))

	// Copy testdata
	projectDir := shared.CopyTestdataDir(t, "spring-maven")
	defer fileutil.Cleanup(projectDir)

	cifuzzRunner := shared.CIFuzzRunner{
		CIFuzzPath:      cifuzz,
		DefaultWorkDir:  projectDir,
		DefaultFuzzTest: "com.example.GreeterApplicationTests",
	}

	// Run the fuzz test
	expectedOutputsExp := []*regexp.Regexp{
		regexp.MustCompile(`High: SQL Injection`),
	}
	cifuzzRunner.Run(t, &shared.RunOptions{
		ExpectedOutputs: expectedOutputsExp,
	})

	// Check that the findings command lists the finding
	findings := shared.GetFindings(t, cifuzz, projectDir)
	assert.Len(t, findings, 1)
	assert.Contains(t, findings[0].Details, "SQL Injection")
}
