package utils

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/onsi/ginkgo/v2"
)

type GetBinaryParams struct {
	Filename    string // the name of the binary on the $PATH or in the docker container
	DockerImage string // the docker image to use if Env or Local are not present
	DockerPath  string // the location of the binary in the docker container, including the filename
	EnvKey      string // the environment var to look at for a user-specified service binary
	TmpDir      string // the temp directory to store a downloaded binary if needed
}

// GetBinary uses the passes params structure to get a binary for the service in the first found of 3 locations:
//
// 1. specified via environment variable
//
// 2. matching binary already on path TODO(jbohanon) assess the validity of this since we could inadvertently use a bad/old version
//
// 3. download the hard-coded version via docker and extract the binary from that
func GetBinary(params GetBinaryParams) (string, error) {
	// first check if an environment variable was specified for the binary location
	envPath := os.Getenv(params.EnvKey)
	if envPath != "" {
		log.Printf("Using %s specified in environment variable %s: %s", params.Filename, params.EnvKey, envPath)
		return envPath, nil
	}

	// next check if we have a matching binary on $PATH
	localPath, err := exec.LookPath(params.Filename)
	if err == nil {
		log.Printf("Using %s from PATH: %s", params.Filename, localPath)
		return localPath, nil
	}

	// finally, try to grab one from docker
	return dockerDownload(params.TmpDir, params)

}

func dockerDownload(tmpdir string, params GetBinaryParams) (string, error) {
	log.Printf("Using %s from docker image: %s", params.Filename, params.DockerImage)
	bash := fmt.Sprintf(`
set -ex
CID=$(docker run -d  %s /bin/sh -c exit)

# just print the image sha for repoducibility
echo "Using %s Image:"
docker inspect %s -f "{{.RepoDigests}}"

docker cp $CID:%s ./%s
docker rm -f $CID
    `, params.DockerImage, params.Filename, params.DockerImage, params.DockerPath, params.Filename)
	scriptFile := filepath.Join(tmpdir, "get_binary.sh")

	err := os.WriteFile(scriptFile, []byte(bash), 0755)
	if err != nil {
		return "", err
	}

	cmd := exec.Command("bash", scriptFile)
	cmd.Dir = tmpdir
	cmd.Stdout = ginkgo.GinkgoWriter
	cmd.Stderr = ginkgo.GinkgoWriter
	if err := cmd.Run(); err != nil {
		return "", err
	}

	return filepath.Join(tmpdir, params.Filename), nil

}
