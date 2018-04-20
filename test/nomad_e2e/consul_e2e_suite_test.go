package nomad_e2e

import (
	"os"
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/pkg/log"
	"github.com/solo-io/gloo/test/helpers"
	"github.com/solo-io/gloo/test/helpers/local"
)

func TestConsul(t *testing.T) {
	if os.Getenv("RUN_NOMAD_TESTS") != "1" {
		log.Printf("This test downloads and runs nomad consul and vault. It is disabled by default. " +
			"To enable, set RUN_NOMAD_TESTS=1 in your env.")
		return
	}
	RegisterFailHandler(Fail)
	log.DefaultOut = GinkgoWriter
	RunSpecs(t, "Nomad Suite")
}

var (
	vaultFactory  *localhelpers.VaultFactory
	vaultInstance *localhelpers.VaultInstance

	consulFactory  *localhelpers.ConsulFactory
	consulInstance *localhelpers.ConsulInstance

	nomadFactory  *localhelpers.NomadFactory
	nomadInstance *localhelpers.NomadInstance

	err error
)

var _ = BeforeSuite(func() {
	vaultFactory, err = localhelpers.NewVaultFactory()
	helpers.Must(err)
	vaultInstance, err = vaultFactory.NewVaultInstance()
	helpers.Must(err)
	err = vaultInstance.Run()
	helpers.Must(err)

	consulFactory, err = localhelpers.NewConsulFactory()
	helpers.Must(err)
	consulInstance, err = consulFactory.NewConsulInstance()
	helpers.Must(err)
	err = consulInstance.Run()
	helpers.Must(err)

	nomadFactory, err = localhelpers.NewNomadFactory()
	helpers.Must(err)
	nomadInstance, err = nomadFactory.NewNomadInstance()
	helpers.Must(err)
	err = nomadInstance.Run()
	helpers.Must(err)
})

var _ = AfterSuite(func() {
	vaultInstance.Clean()
	vaultFactory.Clean()

	consulInstance.Clean()
	consulFactory.Clean()

	nomadInstance.Clean()
	nomadFactory.Clean()
})
