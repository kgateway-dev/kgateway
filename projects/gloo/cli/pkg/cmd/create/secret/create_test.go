package secret_test

import (
	"context"
	"fmt"
	"log"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/helpers"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/testutils"
	glootestutils "github.com/solo-io/gloo/test/testutils"
)

var _ = Describe("Create", func() {

	if !glootestutils.IsEnvTruthy(glootestutils.RunVaultTests) {
		log.Print("This test downloads and runs vault and is disabled by default. To enable, set RUN_VAULT_TESTS=1 in your env.")
		return
	}

	var (
		ctx    context.Context
		cancel context.CancelFunc
	)

	BeforeEach(func() {
		ctx, cancel = context.WithCancel(context.Background())

		helpers.UseDefaultClients()
		var err error
		// Start Vault
		vaultInstance, err = vaultFactory.NewVaultInstance()
		Expect(err).NotTo(HaveOccurred())
		err = vaultInstance.Run(ctx)
		Expect(err).NotTo(HaveOccurred())

		// Connect the client to the vaultInstance
		client.SetToken(vaultInstance.Token())
		_ = client.SetAddress(vaultInstance.Address())
	})

	AfterEach(func() {
		helpers.UseDefaultClients()

		cancel()
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
			customSecretEngine := "customSecretEngine"
			err := vaultInstance.EnableSecretEngine(customSecretEngine)
			Expect(err).NotTo(HaveOccurred())

			err = testutils.Glooctl(fmt.Sprintf("create secret aws --name test --access-key foo --secret-key bar --use-vault --vault-address=http://localhost:8200 --vault-token=root --vault-path-prefix=%s", customSecretEngine))
			Expect(err).NotTo(HaveOccurred())
			secret, err := client.Logical().Read(fmt.Sprintf("%s/data/gloo/gloo.solo.io/v1/Secret/gloo-system/test", customSecretEngine))
			Expect(err).NotTo(HaveOccurred())
			Expect(secret).NotTo(BeNil())
		})
	})
})
