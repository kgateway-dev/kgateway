package xds_test

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/pkg/log"
	"github.com/solo-io/gloo/test/helpers/local"
)

func TestXds(t *testing.T) {
	RegisterFailHandler(Fail)
	log.DefaultOut = GinkgoWriter
	RunSpecs(t, "Xds Suite")
}

var (
	envoyFactory  *localhelpers.EnvoyFactory
	envoyInstance *localhelpers.EnvoyInstance
)

var _ = BeforeSuite(func() {
	var err error
	envoyFactory, err = localhelpers.NewEnvoyFactory()
	Expect(err).NotTo(HaveOccurred())
})

var _ = AfterSuite(func() {
	envoyFactory.Clean()
})
