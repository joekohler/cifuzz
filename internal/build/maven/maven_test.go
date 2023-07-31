package maven

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"code-intelligence.com/cifuzz/integration-tests/shared"
)

func Test_GetTestDir(t *testing.T) {
	projectDir := shared.CopyTestdataDir(t, "maven")

	testDir, err := GetTestDir(projectDir)
	require.NoError(t, err)
	assert.Equal(t, testDir, filepath.Join(projectDir, "src", "test", "java"))

	// adjust pom.xml to include tag <testSourceDirectory>
	newTestDir := "fuzztests"
	shared.AddLinesToFileAtBreakPoint(t, filepath.Join(projectDir, "pom.xml"), []string{fmt.Sprintf("<testSourceDirectory>%s</testSourceDirectory>", newTestDir)}, "    <build>", true)
	testDir, err = GetTestDir(projectDir)
	require.NoError(t, err)
	assert.Equal(t, testDir, filepath.Join(projectDir, newTestDir))
}
