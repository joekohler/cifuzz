package java

import (
	"archive/zip"
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"code-intelligence.com/cifuzz/util/archiveutil"
	"code-intelligence.com/cifuzz/util/fileutil"
)

func TestCreateManifestJar(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "manifest-*")
	require.NoError(t, err)
	defer fileutil.Cleanup(tempDir)
	require.NoError(t, err)

	entries := map[string]string{
		"Hello": "World",
		"Foo":   "Bar",
	}
	jarPath, err := CreateManifestJar(entries, tempDir)
	require.NoError(t, err)
	assert.FileExists(t, jarPath)

	// check file header ...
	zipReader, err := zip.OpenReader(jarPath)
	require.NoError(t, err)
	defer zipReader.Close()
	// ... it should contain 2 entries
	// and no matter what OS they should contain
	// forward slashes
	require.Len(t, zipReader.File, 2)
	assert.Equal(t, "META-INF/", zipReader.File[0].Name)
	assert.Equal(t, "META-INF/MANIFEST.MF", zipReader.File[1].Name)

	// unzip the jar and inspect content
	err = archiveutil.Unzip(jarPath, tempDir)
	require.NoError(t, err)
	manifestPath := filepath.Join(tempDir, "META-INF", "MANIFEST.MF")
	require.FileExists(t, manifestPath)
	content, err := os.ReadFile(manifestPath)
	require.NoError(t, err)
	assert.Contains(t, string(content), "Hello: World\n")
	assert.Contains(t, string(content), "Foo: Bar\n")
}

func TestWriteManifest(t *testing.T) {
	var b bytes.Buffer
	target := io.Writer(&b)
	entries := map[string]string{
		"Hello": "World",
	}
	err := writeManifest(entries, target)
	require.NoError(t, err)
	assert.Contains(t, b.String(), "Hello")
	assert.Contains(t, b.String(), "World")
}

func TestEntriesToString_EOF(t *testing.T) {
	entries := map[string]string{
		"Hello": "World",
		"Foo":   "Bar",
	}
	result, err := entriesToString(entries)
	require.NoError(t, err)
	assert.Equal(t, 2, strings.Count(result, "\n"))
	assert.Equal(t, "\n", result[len(result)-1:])
}

func TestEntriesToString_Empty(t *testing.T) {
	entries := map[string]string{}
	result, err := entriesToString(entries)
	require.NoError(t, err)
	assert.Equal(t, "\n", result)
}

func TestEntriesToString_SplitLines(t *testing.T) {
	entries := map[string]string{
		"Test1": strings.Repeat("A", 70),
		"Test2": "ABC",
		"Test3": strings.Repeat("A", 250),
		"Test4": "ABC",
	}
	result, err := entriesToString(entries)
	require.NoError(t, err)

	assert.Equal(t, 8, strings.Count(result, "\n"))

	lines := strings.SplitAfter(result, "\n")
	for _, line := range lines {
		assert.LessOrEqual(t, len(line), 72)
	}
}

func TestEntriesToString_LineLimit(t *testing.T) {
	// Make sure we do not break a single line when
	// exactly at the byte length limit
	entries := map[string]string{
		"Hello": strings.Repeat("A", 63),
	}
	result, err := entriesToString(entries)
	require.NoError(t, err)
	assert.Equal(t, 1, strings.Count(result, "\n"))
}

func TestEntriesToString_LongHeader(t *testing.T) {
	// header should not be longer than 70 chars
	header := strings.Repeat("A", 71)
	entries := map[string]string{
		header: "World",
	}
	_, err := entriesToString(entries)
	require.Error(t, err)

	header = strings.Repeat("A", 70)
	entries = map[string]string{
		header: "World",
	}
	_, err = entriesToString(entries)
	require.NoError(t, err)
}

func TestEntriesToString_LongHeaderAndLine(t *testing.T) {
	header := strings.Repeat("A", 70)
	value := strings.Repeat("B", 140)
	entries := map[string]string{
		header: value,
	}
	result, err := entriesToString(entries)
	require.NoError(t, err)
	assert.Equal(t, 4, strings.Count(result, "\n"))
}
