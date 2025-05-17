package fsutils

import (
	"os"
	"path/filepath"
	"testing"
	"github.com/stretchr/testify/assert"
)

// test that content can be written to a temporary file and read back correctly
func TestToTempFile(t *testing.T) {
	content := "test content"
	filename, err := ToTempFile(content)
	assert.NoError(t, err)
	defer os.Remove(filename)

	data, err := os.ReadFile(filename)
	assert.NoError(t, err)
	assert.Equal(t, content, string(data))
}

// test directory detection for temporary dirs, non-existent dirs, and regular files
func TestIsDirectory(t *testing.T) {
	// Test with a Temporary Directory
	tempDir, err := os.MkdirTemp("", "test")
	assert.NoError(t, err)
	defer os.RemoveAll(tempDir)

	assert.True(t, IsDirectory(tempDir))

	// Test with a non existant directory
	assert.False(t, IsDirectory("/testDir"))

	// Test with file instead of directory
	f, err := os.CreateTemp("", "test")
	assert.NoError(t, err)
	defer os.Remove(f.Name())
	assert.False(t, IsDirectory(f.Name()))
}

// test that the current directory path can be retrieved and is valid
func TestMustGetThisDir(t *testing.T) {
	dir := MustGetThisDir()
	assert.NotEmpty(t, dir)
	assert.True(t, IsDirectory(dir))
}

// test that the go.mod file path can be retrieved and points to a valid go.mod file
func TestGoModPath(t *testing.T) {
	path := GoModPath()
	assert.NotEmpty(t, path)
	assert.Equal(t, "go.mod", filepath.Base(path))
}

// test that the module root directory can be found and contains a go.mod file
func TestGetModuleRoot(t *testing.T) {
	root := GetModuleRoot()
	assert.NotEmpty(t, root)
	assert.True(t, IsDirectory(root))
	
	// Verify go.mod exists in root
	_, err := os.Stat(filepath.Join(root, "go.mod"))
	assert.NoError(t, err)
}
