package envoy

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/solo-io/skv2/codegen/util"

	"github.com/solo-io/gloo/test/testutils"
	"github.com/solo-io/gloo/test/testutils/version"

	"github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	errors "github.com/rotisserie/eris"

	"github.com/solo-io/go-utils/log"
)

type Factory interface {
	MustEnvoyInstance() *Instance
	NewEnvoyInstance() (*Instance, error)
	MustClean()
}

var _ Factory = new(factoryImpl)

type factoryImpl struct {
	instanceManager *InstanceManager
}

func MustEnvoyFactory() *factoryImpl {
	return &factoryImpl{
		// Load the instance manager during initialization to error loudly if we don't
		// have necessary configuration
		instanceManager: mustGetInstanceManager(),
	}
}

func (g *factoryImpl) MustEnvoyInstance() *Instance {
	return g.instanceManager.MustEnvoyInstance()
}

func (g *factoryImpl) NewEnvoyInstance() (*Instance, error) {
	return g.instanceManager.NewEnvoyInstance()
}

func (g *factoryImpl) MustClean() {
	if err := g.instanceManager.Clean(); err != nil {
		ginkgo.Fail(fmt.Sprintf("failed to clean up envoy instances: %v", err))
	}
}

func mustGetInstanceManager() *InstanceManager {
	var err error

	// if an envoy binary is explicitly specified, use it
	envoyPath := os.Getenv(testutils.EnvoyBinary)
	if envoyPath != "" {
		log.Printf("Using envoy from environment variable: %s", envoyPath)
		return NewLinuxInstanceManager(bootstrapTemplate, envoyPath, "")
	}

	// maybe it is in the path?!
	// only try to use local path if FETCH_ENVOY_BINARY is not set;
	// there are two options:
	// - you are using local envoy binary you just built and want to test (don't set the variable)
	// - you want to use the envoy version gloo is shipped with (set the variable)
	shouldFetchBinary := os.Getenv(testutils.FetchEnvoyBinary) != ""
	if shouldFetchBinary {
		envoyPath, err = exec.LookPath("envoy")
		if err == nil {
			log.Printf("Using envoy from PATH: %s", envoyPath)
			return NewLinuxInstanceManager(bootstrapTemplate, envoyPath, "")
		}
	}

	switch runtime.GOOS {
	case "darwin":
		log.Printf("Using docker to Run envoy")

		image := fmt.Sprintf("quay.io/solo-io/envoy-gloo-wrapper:%s", mustGetEnvoyWrapperTag())
		return NewDockerInstanceManager(bootstrapTemplate, image)

	case "linux":
		var tmpDir string

		// try to grab one form docker...
		tmpDir, err = os.MkdirTemp(os.Getenv("HELPER_TMP"), "envoy")
		Expect(err).NotTo(HaveOccurred())

		envoyImageTag := mustGetEnvoyGlooTag()

		log.Printf("Using envoy docker image tag: %s", envoyImageTag)

		bash := fmt.Sprintf(`
set -ex
CID=$(docker Run -d  quay.io/solo-io/envoy-gloo:%s /bin/bash -c exit)

# just print the image sha for repoducibility
echo "Using Envoy Image:"
docker inspect quay.io/solo-io/envoy-gloo:%s -f "{{.RepoDigests}}"

docker cp $CID:/usr/local/bin/envoy .
docker rm $CID
    `, envoyImageTag, envoyImageTag)
		scriptfile := filepath.Join(tmpDir, "getenvoy.sh")

		os.WriteFile(scriptfile, []byte(bash), 0755)

		cmd := exec.Command("bash", scriptfile)
		cmd.Dir = tmpDir
		cmd.Stdout = ginkgo.GinkgoWriter
		cmd.Stderr = ginkgo.GinkgoWriter
		err = cmd.Run()
		Expect(err).NotTo(HaveOccurred())

		return NewLinuxInstanceManager(bootstrapTemplate, filepath.Join(tmpDir, "envoy"), tmpDir)

	default:
		ginkgo.Fail("Unsupported OS: " + runtime.GOOS)
	}
	return nil
}

// mustGetEnvoyGlooTag returns the tag of the envoy-gloo image which will be executed
// The tag is chosen using the following process:
//  1. If ENVOY_IMAGE_TAG is defined, use that tag
//  2. If not defined, use the ENVOY_GLOO_IMAGE tag defined in the Makefile
func mustGetEnvoyGlooTag() string {
	eit := os.Getenv(testutils.EnvoyImageTag)
	if eit != "" {
		return eit
	}

	makefile := filepath.Join(util.GetModuleRoot(), "Makefile")
	inFile, err := os.Open(makefile)
	Expect(err).NotTo(HaveOccurred())

	defer inFile.Close()

	const prefix = "ENVOY_GLOO_IMAGE ?= quay.io/solo-io/envoy-gloo:"

	scanner := bufio.NewScanner(inFile)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, prefix) {
			return strings.TrimSpace(strings.TrimPrefix(line, prefix))
		}
	}

	ginkgo.Fail("Could not determine envoy-gloo tag. Find valid tag names here https://quay.io/repository/solo-io/envoy-gloo?tab=tags")
	return ""
}

// mustGetEnvoyWrapperTag returns the tag of the envoy-gloo-wrapper image which will be executed
// The tag is chosen using the following process:
//  1. If ENVOY_IMAGE_TAG is defined, use that tag
//  2. If not defined, use the latest released tag of that image
func mustGetEnvoyWrapperTag() string {
	eit := os.Getenv(testutils.EnvoyImageTag)
	if eit != "" {
		return eit
	}

	latestPatchVersion, err := version.GetLastReleaseOfCurrentBranch()
	if err != nil {
		ginkgo.Fail(errors.Wrap(err, "Failed to extract the latest release of current minor").Error())
	}

	return strings.TrimPrefix(latestPatchVersion.String(), "v")
}
