package services

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"time"

	"github.com/solo-io/gloo/test/testutils"

	"github.com/avast/retry-go"

	. "github.com/onsi/gomega"

	"github.com/onsi/ginkgo/v2"
	"github.com/pkg/errors"
)

const (
	// defaultNetwork is the default docker network driver
	// https://docs.docker.com/network/drivers/bridge/
	defaultNetwork = "bridge"

	// cloudbuildNetwork is the docker network driver used by Google Cloudbuild
	// https://cloud.google.com/build/docs/build-config-file-schema#network
	cloudbuildNetwork = "cloudbuild"
)

func RunContainer(containerName string, args []string) error {
	runArgs := append([]string{
		"run",
		"--rm",
		"--name",
		containerName,
	}, args...)
	cmd := exec.Command("docker", runArgs...)
	cmd.Stdout = ginkgo.GinkgoWriter
	cmd.Stderr = ginkgo.GinkgoWriter
	err := cmd.Run()
	if err != nil {
		return errors.Wrap(err, "Unable to start "+containerName+" container")
	}
	return nil
}

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
	Eventually(ContainerExistsWithName(containerName), "30s", "2s").Should(BeEmpty())
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

// WaitUntilContainerRemoved polls docker for removal of the container named containerName - block until
// successful or fail after a small number of retries
func WaitUntilContainerRemoved(containerName string) error {
	return retry.Do(func() error {
		inspectErr := exec.Command("docker", "inspect", containerName).Run()
		if inspectErr == nil {
			// If there is no error, it means the container still exists, so we want to retry
			return errors.Errorf("container %s still exists", containerName)
		}
		return nil
	},
		retry.RetryIf(func(err error) bool {
			return err != nil
		}),
		retry.Attempts(10),
		retry.Delay(time.Millisecond*200),
		retry.DelayType(retry.BackOffDelay),
		retry.LastErrorOnly(true),
	)
}

func RunningInDocker() bool {
	if _, err := os.Stat("/.dockerenv"); os.IsNotExist(err) {
		// magic docker env file doesn't exist. not running in docker
		return false
	}
	return true
}

func GetDockerHost(containerName string) string {
	if RunningInDocker() {
		return containerName
	} else {
		return "127.0.0.1"
	}
}

func GetContainerNetwork() string {
	network := defaultNetwork
	if RunningInDocker() {
		if !testutils.IsRunningInCloudbuild() {
			// We error loudly here so that if/when we move off of Google Cloudbuild, we can clean up this logic
			ginkgo.Fail("Running in docker but not in cloudbuild. Could not determine docker network")
		}
		network = cloudbuildNetwork
	}
	return network
}
