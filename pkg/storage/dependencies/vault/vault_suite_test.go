package vault

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/pkg/log"
	"github.com/solo-io/gloo/test/helpers"
	"github.com/solo-io/gloo/test/helpers/local"
)

func TestVault(t *testing.T) {
	RegisterFailHandler(Fail)
	log.DefaultOut = GinkgoWriter
	RunSpecs(t, "Vault Suite")
}

var (
	vaultFactory  *localhelpers.VaultFactory
	vaultInstance *localhelpers.VaultInstance
	err           error
)

var _ = BeforeSuite(func() {
	vaultFactory, err = localhelpers.NewVaultFactory()
	helpers.Must(err)
	vaultInstance, err = vaultFactory.NewVaultInstance()
	helpers.Must(err)
	err = vaultInstance.Run()
	helpers.Must(err)
})

var _ = AfterSuite(func() {
	vaultInstance.Clean()
	vaultFactory.Clean()
})
