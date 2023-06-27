package services

import (
    "fmt"
    "log"
    "os/exec"
    "time"

    . "github.com/onsi/gomega"

    "github.com/onsi/ginkgo/v2"
    "github.com/pkg/errors"
)

// ContainerExistsWithName returns an empty string if the container does not exist
func ContainerExistsWithName(containerName string) string {
    cmd := exec.Command("docker", "ps", "-aq", "-f", "name=^/"+containerName+"$")
    out, err := cmd.CombinedOutput()
    if err != nil {
        fmt.Printf("cmd.Run() [%s %s] failed with %s\n", cmd.Path, cmd.Args, err)
    }
    return string(out)
}

func ExecOnContainer(containerName string, args []string) ([]byte, error) {
    arguments := []string{"exec", containerName}
    arguments = append(arguments, args...)
    cmd := exec.Command("docker", arguments...)
    out, err := cmd.CombinedOutput()
    if err != nil {
        return nil, errors.Wrapf(err, "Unable to execute command %v on [%s] container [%s]", arguments, containerName, out)
    }
    return out, nil
}

func MustStopAndRemoveContainer(containerName string) {
    StopContainer(containerName)

    // We assumed that the container was run with auto-remove, and thus stopping the container will cause it to be removed
    err := WaitUntilContainerRemoved(containerName)
    Expect(err).ToNot(HaveOccurred())

    // CI host may be extremely CPU-bound as it's often building test assets in tandem with other tests,
    // as well as other CI builds running in parallel. When that happens, the tests can run much slower,
    // thus they need a longer timeout. see https://github.com/solo-io/solo-projects/issues/1701#issuecomment-620873754
    Eventually(ContainerExistsWithName(containerName), "30s", ".2s").Should(BeEmpty())
}

func StopContainer(containerName string) {
    cmd := exec.Command("docker", "stop", containerName)
    cmd.Stdout = ginkgo.GinkgoWriter
    cmd.Stderr = ginkgo.GinkgoWriter
    err := cmd.Run()
    if err != nil {
        log.Printf("Error stopping container %s: %v", containerName, err)
    }
}

// poll docker for removal of the container named containerName - block until
// successful or fail after a small number of retries
func WaitUntilContainerRemoved(containerName string) error {
    // if this function returns nil, it means the container is still running
    isContainerRemoved := func() bool {
        cmd := exec.Command("docker", "inspect", containerName)
        return cmd.Run() != nil
    }
    for i := 0; i < 5; i++ {
        if isContainerRemoved() {
            return nil
        }
        fmt.Println("Waiting for removal of container " + containerName)
        time.Sleep(1 * time.Second)
    }
    return errors.New("Unable to delete container " + containerName)
}
