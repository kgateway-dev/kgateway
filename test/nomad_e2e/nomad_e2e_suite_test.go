package nomad_e2e

import (
	"os"
	"testing"

	"time"

	"github.com/hashicorp/consul/api"
	vaultapi "github.com/hashicorp/vault/api"
	. "github.com/onsi/ginkgo"
	"github.com/solo-io/gloo/pkg/log"
	"github.com/solo-io/gloo/pkg/storage"
	"github.com/solo-io/gloo/pkg/storage/consul"
	"github.com/solo-io/gloo/pkg/storage/dependencies"
	"github.com/solo-io/gloo/pkg/storage/dependencies/vault"
	"github.com/solo-io/gloo/test/helpers"
	"github.com/solo-io/gloo/test/helpers/local"
	"io/ioutil"
)

func TestConsul(t *testing.T) {
	if os.Getenv("RUN_NOMAD_TESTS") != "1" {
		log.Printf("This test downloads and runs nomad consul and vault. It is disabled by default. " +
			"To enable, set RUN_NOMAD_TESTS=1 in your env.")
		return
	}

	helpers.RegisterPreFailHandler(func() {
		var logs string
		for _, task := range []string{"control-plane", "ingress"} {
			if nomadInstance != nil {
				l, err := nomadInstance.Logs("gloo", task)
				logs += l + "\n"
				if err != nil {
					logs += "error getting logs for " + task + ": " + err.Error()
				}
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
	})

	helpers.RegisterCommonFailHandlers()

	log.DefaultOut = GinkgoWriter
	RunSpecs(t, "Nomad Suite")
}

var (
	envoyFactory *localhelpers.EnvoyFactory

	vaultFactory  *localhelpers.VaultFactory
	vaultInstance *localhelpers.VaultInstance

	consulFactory  *localhelpers.ConsulFactory
	consulInstance *localhelpers.ConsulInstance

	nomadFactory  *localhelpers.NomadFactory
	nomadInstance *localhelpers.NomadInstance

	gloo    storage.Interface
	secrets dependencies.SecretStorage

	outputDirectory string
	err             error
)

var _ = BeforeSuite(func() {
	envoyFactory, err = localhelpers.NewEnvoyFactory()
	helpers.Must(err)

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
	nomadInstance, err = nomadFactory.NewNomadInstance(vaultInstance)
	helpers.Must(err)
	nomadInstance.Silence()
	err = nomadInstance.Run()
	helpers.Must(err)

	gloo, err = consul.NewStorage(api.DefaultConfig(), "gloo", time.Second)
	helpers.Must(err)

	vaultCfg := vaultapi.DefaultConfig()
	vaultCfg.Address = "http://127.0.0.1:8200"
	cli, err := vaultapi.NewClient(vaultCfg)
	helpers.Must(err)
	cli.SetToken(vaultInstance.Token())

	secrets = vault.NewSecretStorage(cli, "gloo", time.Second)

	// TODO: set outputDirectory from something more variable
	outputDirectory, err = ioutil.TempDir(os.Getenv("HELPER_TMP"), "gloo-nomad-e2e")
	helpers.Must(err)
	err = nomadInstance.SetupNomadForE2eTest(envoyFactory.EnvoyPath(), outputDirectory, true)
	helpers.Must(err)
})

var _ = AfterSuite(func() {
	if err := nomadInstance.TeardownNomadE2e(); err != nil {
		log.Warnf("FAILED TEARING DOWN: %v", err)
	}
	envoyFactory.Clean()

	vaultInstance.Clean()
	vaultFactory.Clean()

	consulInstance.Clean()
	consulFactory.Clean()

	nomadInstance.Clean()
	nomadFactory.Clean()

	os.RemoveAll(outputDirectory)
})
