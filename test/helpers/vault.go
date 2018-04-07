package helpers

import (
	"os"
	"os/exec"

	"github.com/onsi/ginkgo"
)

func DockerRunVault(containerName, rootToken string) error {
	cmd := exec.Command("docker",
		"run",
		"--rm",
		"--cap-add=IPC_LOCK",
		"-e", "VAULT_DEV_ROOT_TOKEN_ID="+rootToken,
		"-p", "8200:8200",
		"--name="+containerName, "vault")
	if os.Getenv("DEBUG") == "1" {
		cmd.Stdout = ginkgo.GinkgoWriter
		cmd.Stderr = ginkgo.GinkgoWriter
	}
	return cmd.Start()
}

func DockerRm(containerName string) error {
	return exec.Command("docker", "rm", "-f", containerName).Run()
}
