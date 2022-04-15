package secret_test

import (
	"fmt"
	"log"
	"os"

	"github.com/hashicorp/vault/api"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/helpers"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/testutils"
	"github.com/solo-io/gloo/test/services"
)

var _ = Describe("Create", func() {
	if os.Getenv("RUN_VAULT_TESTS") != "1" {
		log.Print("This test downloads and runs vault and is disabled by default. To enable, set RUN_VAULT_TESTS=1 in your env.")
		return
	}

	var (
		vaultFactory  *services.VaultFactory
		vaultInstance *services.VaultInstance
		client        *api.Client
	)

	BeforeSuite(func() {
		var err error
		vaultFactory, err = services.NewVaultFactory(&services.VaultFactoryConfig{PathPrefix: services.TestPathPrefix})
		Expect(err).NotTo(HaveOccurred())
		client, err = api.NewClient(api.DefaultConfig())
		Expect(err).NotTo(HaveOccurred())
		client.SetToken("root")
		_ = client.SetAddress("http://localhost:8200")

	})

	AfterSuite(func() {
		_ = vaultFactory.Clean()
	})

	BeforeEach(func() {
		helpers.UseDefaultClients()
		var err error
		// Start Vault
		vaultInstance, err = vaultFactory.NewVaultInstance()
		Expect(err).NotTo(HaveOccurred())
		err = vaultInstance.Run()
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		if vaultInstance != nil {
			err := vaultInstance.Clean()
			Expect(err).NotTo(HaveOccurred())
		}
		helpers.UseDefaultClients()
	})

	Context("vault storage backend", func() {
		It("does secrets", func() {
			err := testutils.Glooctl("create secret aws --name test --access-key foo --secret-key bar --use-vault --vault-address=http://localhost:8200 --vault-token=root")
			Expect(err).NotTo(HaveOccurred())
			secret, err := client.Logical().Read("secret/data/gloo/gloo.solo.io/v1/Secret/gloo-system/test")
			Expect(err).NotTo(HaveOccurred())
			Expect(secret).NotTo(BeNil())
		})

		It("works with custom secrets engine path secrets", func() {
			err := testutils.Glooctl(fmt.Sprintf("create secret aws --name test --access-key foo --secret-key bar --use-vault --vault-address=http://localhost:8200 --vault-token=root --vault-path-prefix=%s", services.TestPathPrefix))
			Expect(err).NotTo(HaveOccurred())
			secret, err := client.Logical().Read(fmt.Sprintf("%s/data/gloo/gloo.solo.io/v1/Secret/gloo-system/test", services.TestPathPrefix))
			Expect(err).NotTo(HaveOccurred())
			Expect(secret).NotTo(BeNil())
		})
	})
})
