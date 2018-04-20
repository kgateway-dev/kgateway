package nomad_e2e

import (
	"os"
	"testing"

	"time"

	"github.com/hashicorp/consul/api"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/pkg/log"
	"github.com/solo-io/gloo/pkg/storage"
	"github.com/solo-io/gloo/pkg/storage/consul"
	"github.com/solo-io/gloo/test/helpers"
	"github.com/solo-io/gloo/test/helpers/local"
	"github.com/solo-io/gloo/test/nomad_e2e/utils"
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

	gloo storage.Interface

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

	gloo, err = consul.NewStorage(api.DefaultConfig(), "gloo", time.Second)
	helpers.Must(err)

	err = utils.SetupNomadForE2eTest(nomadInstance, true)
	helpers.Must(err)
})

var _ = AfterSuite(func() {
	if err := utils.TeardownNomadE2e(); err != nil {
		log.Warnf("FAILED TEARING DOWN: %v", err)
	}

	vaultInstance.Clean()
	vaultFactory.Clean()

	consulInstance.Clean()
	consulFactory.Clean()

	if err := nomadInstance.Clean(); err != nil {
		log.Warnf("FAILED CLEANING UP: %v", err)
	}
	nomadFactory.Clean()

})
