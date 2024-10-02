package utils_test

import (
	"archive/zip"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/solo-io/gloo/test/services/utils"
	"github.com/solo-io/gloo/test/testutils"
	"github.com/stretchr/testify/assert"
)

func TestGetBinaryFromEnv(t *testing.T) {
	// Set up a temporary environment variable
	os.Setenv(testutils.VaultBinary, "/usr/local/bin/vault")
	defer os.Unsetenv(testutils.VaultBinary)

	params := utils.GetBinaryParams{
		Filename:    testutils.VaultBinaryName,
		EnvKey:      testutils.VaultBinary,
		TmpDir:      t.TempDir(),
		DockerImage: testutils.VaultDockerImage,
	}

	// Test that GetBinary reads the binary from the environment variable
	binaryPath, err := utils.GetBinary(params)
	assert.NoError(t, err)
	assert.Equal(t, "/usr/local/bin/vault", binaryPath)
}

func TestGetBinaryFromPath(t *testing.T) {
	// Make sure there is no environment variable set
	os.Unsetenv(testutils.VaultBinary)

	// Mock ExecLookPathWrapper to always return a path
	utils.ExecLookPathWrapper = func(file string) (string, error) {
		if file == testutils.VaultBinaryName {
			return "/usr/bin/vault", nil
		}
		return "", errors.New("not found")
	}
	// Reset after test
	defer func() { utils.ExecLookPathWrapper = exec.LookPath }()

	params := utils.GetBinaryParams{
		Filename:    testutils.VaultBinaryName,
		TmpDir:      t.TempDir(),
		DockerImage: testutils.VaultDockerImage,
	}

	// Test that GetBinary finds the binary in $PATH
	binaryPath, err := utils.GetBinary(params)
	assert.NoError(t, err)
	assert.Equal(t, "/usr/bin/vault", binaryPath)
}

func TestGetBinaryDownloadDarwin(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("This test is only for Darwin systems")
	}

	// Mock downloading and extracting binary
	utils.DownloadAndExtractBinary = func(tmpDir, filename, version string) (string, error) {
		return filepath.Join(tmpDir, filename), nil
	}

	params := utils.GetBinaryParams{
		Filename:    testutils.VaultBinaryName,
		TmpDir:      t.TempDir(),
		DockerImage: testutils.VaultDockerImage,
	}

	// Test that GetBinary downloads and extracts the binary on Darwin systems
	binaryPath, err := utils.GetBinary(params)
	assert.NoError(t, err)
	assert.Contains(t, binaryPath, testutils.VaultBinaryName)
}

func TestGetBinaryDockerDownload(t *testing.T) {
	// Mock exec.LookPath to simulate missing binary on $PATH
	utils.ExecLookPathWrapper = func(file string) (string, error) {
		return "", errors.New("not found")
	}
	// Reset after test
	defer func() { utils.ExecLookPathWrapper = exec.LookPath }()

	params := utils.GetBinaryParams{
		Filename:    testutils.VaultBinaryName,
		TmpDir:      t.TempDir(),
		DockerImage: testutils.VaultDockerImage,
	}

	// Mock the dockerDownload function
	utils.DockerDownload = func(tmpdir string, params utils.GetBinaryParams) (string, error) {
		return filepath.Join(tmpdir, testutils.VaultBinaryName), nil
	}

	// Test that GetBinary falls back to downloading from Docker
	binaryPath, err := utils.GetBinary(params)
	assert.NoError(t, err)
	assert.Contains(t, binaryPath, testutils.VaultBinaryName)
}

func TestDownloadAndExtractBinary(t *testing.T) {
	// Use a temporary directory for the test
	tmpDir := t.TempDir()

	// Replace with real implementation to test
	binaryPath, err := utils.DownloadAndExtractBinary(tmpDir, testutils.VaultBinaryName, testutils.VaultBinaryVersion)
	assert.NoError(t, err)
	assert.Contains(t, binaryPath, "vault")
}

func TestUnzip(t *testing.T) {
	// Use a temporary directory for the test
	tmpDir := t.TempDir()

	// Create a zip file in the temporary directory
	zipFilePath := filepath.Join(tmpDir, "test.zip")
	zipFile, err := os.Create(zipFilePath)
	assert.NoError(t, err)
	defer zipFile.Close()

	// Write a valid zip file
	zipWriter := zip.NewWriter(zipFile)
	fileInZip, err := zipWriter.Create("test.txt")
	assert.NoError(t, err)

	_, err = fileInZip.Write([]byte("This is a test file inside the zip archive"))
	assert.NoError(t, err)
	err = zipWriter.Close()
	assert.NoError(t, err)

	// Create another directory to unzip into
	unzipDir := filepath.Join(tmpDir, "unzipped")
	err = os.Mkdir(unzipDir, 0755)
	assert.NoError(t, err)

	// Test unzipping the file
	err = utils.Unzip(zipFilePath, unzipDir)
	assert.NoError(t, err)

	// Verify that the file was unzipped correctly
	unzippedFilePath := filepath.Join(unzipDir, "test.txt")
	_, err = os.Stat(unzippedFilePath)
	assert.NoError(t, err, "unzipped file not found")

	// Read the contents of the unzipped file
	unzippedFileContent, err := os.ReadFile(unzippedFilePath)
	assert.NoError(t, err)
	assert.Equal(t, "This is a test file inside the zip archive", string(unzippedFileContent))
}
