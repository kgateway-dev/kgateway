package utils

import (
	"archive/zip"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/onsi/ginkgo/v2"

	"github.com/solo-io/gloo/test/testutils"
)

// ExecLookPathWrapper is a wrapper around exec.LookPath so it can be mocked in tests.
var ExecLookPathWrapper = exec.LookPath

// DownloadAndExtractBinary downloads and extracts the Vault or Consul binary for darwin OS
var DownloadAndExtractBinary = func(tmpDir, filename, version string) (string, error) {
	goos := runtime.GOOS
	goarch := runtime.GOARCH

	url := fmt.Sprintf("https://releases.hashicorp.com/%s/%s/%s_%s_%s_%s.zip", filename, version, filename, version, goos, goarch)
	log.Printf("Downloading %s binary from: %s", filename, url)

	// Create temp zip file
	zipPath := filepath.Join(tmpDir, fmt.Sprintf("%s.zip", filename))
	out, err := os.Create(zipPath)
	if err != nil {
		return "", fmt.Errorf("failed to create temp zip file: %w", err)
	}
	defer out.Close()

	// Download the zip file
	resp, err := http.Get(url)
	if err != nil {
		return "", fmt.Errorf("failed to download %s binary: %w", filename, err)
	}
	defer resp.Body.Close()

	_, err = out.ReadFrom(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to download %s binary: %w", filename, err)
	}

	// Extract the zip file
	err = Unzip(zipPath, tmpDir)
	if err != nil {
		return "", fmt.Errorf("failed to unzip %s binary: %w", filename, err)
	}

	// Add extracted binary to PATH
	binaryPath := filepath.Join(tmpDir, filename)
	if err := os.Chmod(binaryPath, 0755); err != nil {
		return "", fmt.Errorf("failed to make %s binary executable: %w", filename, err)
	}

	log.Printf("%s binary extracted to: %s", filename, binaryPath)
	return binaryPath, nil
}

// DockerDownload extracts a binary from a Docker image by running a temporary
// Docker container, copying the binary from the container's filesystem, and
// saving it to a local temporary directory. This function is primarily used
// when a binary is not available via environment variables or on the system's PATH.
//
// tmpdir: The temporary directory where the binary will be saved.
// params: A struct containing parameters such as the filename, Docker image,
//
//	and Docker path to locate the binary in the container.
//
// Returns the path to the saved binary on success or an error if the operation fails.
var DockerDownload = func(tmpdir string, params GetBinaryParams) (string, error) {
	goos := runtime.GOOS

	if goos == "darwin" {
		return "", fmt.Errorf("unsupported operating system: %s", goos)
	}

	log.Printf("Using %s from docker image: %s", params.Filename, params.DockerImage)

	// use bash to run a docker container and extract the binary file from the running container
	bash := fmt.Sprintf(`
set -ex
CID=$(docker run -d  %s /bin/sh -c exit)

# just print the image sha for reproducibility
echo "Using %s Image:"
docker inspect %s -f "{{.RepoDigests}}"

docker cp $CID:%s ./%s
docker rm -f $CID
    `, params.DockerImage, params.Filename, params.DockerImage, params.DockerPath, params.Filename)
	scriptFile := filepath.Join(tmpdir, "get_binary.sh")

	// write out our custom script to the filesystem
	err := os.WriteFile(scriptFile, []byte(bash), 0755)
	if err != nil {
		return "", err
	}

	// run our script to extract a binary from a docker container
	cmd := exec.Command("bash", scriptFile)
	cmd.Dir = tmpdir
	cmd.Stdout = ginkgo.GinkgoWriter
	cmd.Stderr = ginkgo.GinkgoWriter
	if err := cmd.Run(); err != nil {
		return "", err
	}

	return filepath.Join(tmpdir, params.Filename), nil
}

type GetBinaryParams struct {
	Filename    string // the name of the binary on the $PATH or in the docker container
	DockerImage string // the docker image to use if Env or Local are not present
	DockerPath  string // the location of the binary in the docker container, including the filename
	EnvKey      string // the environment var to look at for a user-specified service binary
	TmpDir      string // the temp directory to store a downloaded binary if needed
}

// GetBinary checks for a binary in the following order:
// 1. From the environment variable if specified
// 2. Locally available on the $PATH
// 3. If `darwin` is the OS, download the Vault or Consul binary from the HashiCorp release page
// 4. As a last resort, pull the binary from a Docker image
func GetBinary(params GetBinaryParams) (string, error) {
	// first check if an environment variable was specified for the binary location
	envPath := os.Getenv(params.EnvKey)
	if envPath != "" {
		log.Printf("Using %s specified in environment variable %s: %s", params.Filename, params.EnvKey, envPath)
		return envPath, nil
	}

	// next check if we have a matching binary on $PATH
	localPath, err := ExecLookPathWrapper(params.Filename)
	if err == nil {
		log.Printf("Using %s from PATH: %s", params.Filename, localPath)
		return localPath, nil
	}

	// if GOOS is darwin and the Filename is either vault or consul, download from HashiCorp releases
	if runtime.GOOS == "darwin" {
		switch params.Filename {
		case "vault":
			log.Printf("Downloading %s for darwin", params.Filename)
			return DownloadAndExtractBinary(params.TmpDir, testutils.VaultBinaryName, testutils.VaultBinaryVersion)
		case "consul":
			log.Printf("Downloading %s for darwin", params.Filename)
			return DownloadAndExtractBinary(params.TmpDir, testutils.ConsulBinaryName, testutils.ConsulBinaryVersion)
		}
	}

	// finally, try to grab one from docker
	return DockerDownload(params.TmpDir, params)
}

// Unzip unzips the given archive to the specified destination
func Unzip(src, dest string) error {
	r, err := zip.OpenReader(src)
	if err != nil {
		return err
	}
	defer r.Close()

	for _, f := range r.File {
		fpath := filepath.Join(dest, f.Name)

		// Create directories or files
		if f.FileInfo().IsDir() {
			os.MkdirAll(fpath, os.ModePerm)
			continue
		}

		// Ensure the directory exists
		if err := os.MkdirAll(filepath.Dir(fpath), os.ModePerm); err != nil {
			return err
		}

		// Extract the file
		outFile, err := os.OpenFile(fpath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
		if err != nil {
			return err
		}

		rc, err := f.Open()
		if err != nil {
			return err
		}

		_, err = outFile.ReadFrom(rc)
		outFile.Close()
		rc.Close()

		if err != nil {
			return err
		}
	}
	return nil
}
