package e2e

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/pkg/log"

	"os"
	"testing"
)

func TestKubernetes(t *testing.T) {
	if os.Getenv("RUN_KUBE_TESTS") != "1" {
		log.Printf("This test creates kubernetes resources and is disabled by default. To enable, set RUN_KUBE_TESTS=1 in your env.")
		return
	}
	RegisterFailHandler(Fail)
	log.DefaultOut = GinkgoWriter
	RunSpecs(t, "Kubernetes Suite")
}
