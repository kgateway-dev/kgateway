package secret_test

import (
	"testing"

	"github.com/hashicorp/vault/api"
	"github.com/solo-io/gloo/test/services"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/solo-io/gloo/test/gomega"
)

func TestSecret(t *testing.T) {
	SetAsyncAssertionDefaults(AsyncAssertionDefaults{})
	RegisterFailHandler(Fail)

	RunSpecs(t, "Secret Suite")
}

var (
	vaultFactory  *services.VaultFactory
	vaultInstance *services.VaultInstance
	client        *api.Client
)

var _ = BeforeSuite(func() {
	var err error
	vaultFactory, err = services.NewVaultFactory()
	Expect(err).NotTo(HaveOccurred())
	client, err = api.NewClient(api.DefaultConfig())
	Expect(err).NotTo(HaveOccurred())
})

var _ = AfterSuite(func() {
	_ = vaultFactory.Clean()
})
