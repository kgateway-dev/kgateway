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
	RegisterFailHandler(func(message string, callerSkip ...int) {
		var logs string
		for _, task := range []string{"control-plane", "ingress"} {
			l, err := utils.Logs(nomadInstance, "gloo", task)
			logs += l + "\n"
			if err != nil {
				logs += "error getting logs for " + task + ": " + err.Error()
			}
		}
		addr, err := helpers.ConsulServiceAddress("ingress", "admin")
		if err == nil {
			configDump, err := helpers.Curl(addr, helpers.CurlOpts{Path: "/config_dump"})
			if err == nil {
				logs += "\n\n\n" + configDump + "\n\n\n"
			}
		}

		log.Printf("\n****************************************" +
			"\nLOGS FROM THE BOYS: \n\n" + logs + "\n************************************")
		Fail(message, callerSkip...)
	})
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
	consulInstance.Silence()
	err = consulInstance.Run()
	helpers.Must(err)

	nomadFactory, err = localhelpers.NewNomadFactory()
	helpers.Must(err)
	nomadInstance, err = nomadFactory.NewNomadInstance()
	helpers.Must(err)
	nomadInstance.Silence()
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

	nomadInstance.Clean()
	nomadFactory.Clean()

})
