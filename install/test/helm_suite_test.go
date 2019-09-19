package test

import (
	"io/ioutil"
	"os"
	"os/exec"
	"sync"
	"testing"

	"github.com/solo-io/go-utils/testutils"
	v1 "k8s.io/api/core/v1"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	. "github.com/solo-io/go-utils/manifesttestutils"
)

func TestHelm(t *testing.T) {

	version = os.Getenv("TAGGED_VERSION")
	if version == "" {
		version = "dev"
		pullPolicy = v1.PullAlways
	} else {
		version = version[1:]
		pullPolicy = v1.PullIfNotPresent
	}

	RegisterFailHandler(Fail)
	testutils.RegisterCommonFailHandlers()
	RunSpecs(t, "Helm Suite")
}

const (
	namespace = "gloo-system"
)

var (
	version string
	// use a mutex to prevent these tests from running in parallel
	makefileSerializer sync.Mutex
	pullPolicy         v1.PullPolicy
	manifests          = map[string]TestManifest{}
)

func MustMake(dir string, args ...string) {
	makeCmd := exec.Command("make", args...)
	makeCmd.Dir = dir

	makeCmd.Stdout = GinkgoWriter
	makeCmd.Stderr = GinkgoWriter
	err := makeCmd.Run()

	ExpectWithOffset(1, err).NotTo(HaveOccurred())
}

func renderManifest(helmFlags string) TestManifest {
	makefileSerializer.Lock()
	defer makefileSerializer.Unlock()

	if tm, ok := manifests[helmFlags]; ok {
		return tm
	}

	f, err := ioutil.TempFile("", "*.yaml")
	ExpectWithOffset(2, err).NotTo(HaveOccurred())
	_ = f.Close()
	manifestYaml := f.Name()
	defer os.Remove(manifestYaml)

	MustMake(".", "-C", "../..", "install/gloo-gateway.yaml", "HELMFLAGS="+helmFlags, "OUTPUT_YAML="+manifestYaml)
	tm := NewTestManifest(manifestYaml)
	manifests[helmFlags] = tm
	return tm
}
