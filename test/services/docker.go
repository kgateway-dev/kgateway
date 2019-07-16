package services

import (
	"fmt"
	"os/exec"

	"github.com/onsi/ginkgo"
	"github.com/solo-io/go-utils/errors"
)

type EchoContainerFactory struct {
	containers map[string]*echoContainer
}

type echoContainer struct {
	name    string
	port    int
	message string
}

func NewEchoContainerFactory() (*EchoContainerFactory, error) {
	_, err := exec.LookPath("docker")
	if err != nil {
		return nil, errors.Wrapf(err, "failed to find docker executable")
	}
	return &EchoContainerFactory{
		containers: map[string]*echoContainer{},
	}, nil
}

func (e *EchoContainerFactory) RunEchoContainer(name string, message string) (int, error) {
	if _, ok := e.containers[name]; ok {
		return 0, errors.Errorf("already have container with name")
	}

	port := int(NextBindPort())

	cmd := exec.Command("docker", "run",
		"--name", name,
		"-p", fmt.Sprintf("%d:8080", port),
		"hashicorp/http-echo",
		"-listen=:8080",
		fmt.Sprintf("-text=%s", message))
	cmd.Stdout = ginkgo.GinkgoWriter
	cmd.Stderr = ginkgo.GinkgoWriter
	err := cmd.Start()
	if err != nil {
		return 0, err
	}

	e.containers[name] = &echoContainer{
		name:    name,
		port:    port,
		message: message,
	}

	return port, err
}

func (e *EchoContainerFactory) CleanUp() error {
	if len(e.containers) == 0 {
		return nil
	}

	var names []string
	for containerName := range e.containers {
		names = append(names, containerName)
	}

	stopCmd := exec.Command("docker", append([]string{"stop", "--time", "1"}, names...)...)
	stopCmd.Stdout = ginkgo.GinkgoWriter
	stopCmd.Stderr = ginkgo.GinkgoWriter
	err := stopCmd.Run()
	if err != nil {
		return err
	}

	removeCmd := exec.Command("docker", append([]string{"rm"}, names...)...)
	removeCmd.Stdout = ginkgo.GinkgoWriter
	removeCmd.Stderr = ginkgo.GinkgoWriter
	return removeCmd.Run()
}
