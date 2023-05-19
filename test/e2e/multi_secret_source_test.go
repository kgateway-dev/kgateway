package e2e_test

import (
	"fmt"
	"os"

	"github.com/solo-io/gloo/test/ginkgo/labels"
	"google.golang.org/protobuf/types/known/wrapperspb"

	"github.com/solo-io/gloo/test/testutils"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	bootstrap_clients "github.com/solo-io/gloo/projects/gloo/pkg/bootstrap/clients"
	"github.com/solo-io/gloo/test/e2e"
)

var _ = Describe("Multiple Secret Clients", Label(labels.Nightly), func() {

	var (
		testContext *e2e.TestContextWithVault
		settings    *gloov1.Settings
	)

	BeforeEach(func() {
		// For an individual test, we can define the environmental requirements necessary for it to succeed.
		// Ideally our tests are environment agnostic. However, if there are certain conditions that must
		// be met, you can define those here. By explicitly defining these requirements, we can error loudly
		// when they are not met. See `testutils.ValidateRequirementsAndNotifyGinkgo` for a more detailed
		// overview of this feature
		var testRequirements = []testutils.Requirement{
			testutils.Vault(),
		}

		testContext = testContextFactory.NewTestContextWithVault(testRequirements...)
		testContext.BeforeEach()

		settings = &gloov1.Settings{}
	})

	AfterEach(func() {
		testContext.AfterEach()
	})

	JustBeforeEach(func() {
		testContext.SetRunSettings(settings)
		testContext.JustBeforeEach()
	})

	JustAfterEach(func() {
		testContext.JustAfterEach()
	})

	// This appears to lend itself to table-driven tests, but because we need
	// to call `testContext.SetRunSettings()` in the BeforeEach, a table will
	// not work as intended (https://github.com/onsi/ginkgo/issues/378)
	universalTests := func() {
		It("creates secret client", func() {
			Expect(testContext.TestClients().SecretClient).NotTo(BeNil())
			_, ok := testContext.TestClients().SecretClient.BaseClient().(*bootstrap_clients.MultiResourceClient)
			Expect(ok).To(BeTrue())
			// DO_NOT_SUBMIT: fill out test
		})
		It("lists secrets", func() {
			// DO_NOT_SUBMIT: fill out test
		})
		It("watches secrets", func() {
			// DO_NOT_SUBMIT: fill out test
		})
	}
	hasNSubClients := func(numClients int) {
		It(fmt.Sprintf("has %d sub-client(s)", numClients), func() {
			Expect(testContext.TestClients().SecretClient.BaseClient().(*bootstrap_clients.MultiResourceClient).Clients()).To(HaveLen(numClients))
		})
	}

	Context("SecretSource API", func() {
		When("using single kube client", func() {
			BeforeEach(func() {
				settings.SecretSource = &gloov1.Settings_KubernetesSecretSource{}
			})
			universalTests()
			hasNSubClients(1)

		})
		When("using single directory client", func() {
			dir, err := os.MkdirTemp("", "secrets_client")
			Expect(err).NotTo(HaveOccurred())
			BeforeEach(func() {
				settings.SecretSource = &gloov1.Settings_DirectorySecretSource{
					DirectorySecretSource: &gloov1.Settings_Directory{
						Directory: dir,
					},
				}
			})
			AfterEach(func() {
				err := os.RemoveAll(dir)
				Expect(err).NotTo(HaveOccurred())
			})
			universalTests()
			hasNSubClients(1)
		})
		When("using single vault client", func() {
			BeforeEach(func() {
				settings.SecretSource = &gloov1.Settings_VaultSecretSource{
					VaultSecretSource: &gloov1.Settings_VaultSecrets{
						Address: testContext.VaultInstance().Address(),
						TlsConfig: &gloov1.Settings_VaultTlsConfig{
							Insecure: &wrapperspb.BoolValue{Value: true},
						},
						AuthMethod: &gloov1.Settings_VaultSecrets_AccessToken{
							AccessToken: testContext.VaultInstance().Token(),
						},
					},
				}
			})
			universalTests()
			hasNSubClients(1)
		})
	})
	Context("SecretOptions API", func() {
		getVaultSourceOptions := func() *gloov1.Settings_SecretOptions_Source_Vault {
			return &gloov1.Settings_SecretOptions_Source_Vault{
				Vault: &gloov1.Settings_VaultSecrets{
					Address: testContext.VaultInstance().Address(),
					TlsConfig: &gloov1.Settings_VaultTlsConfig{
						Insecure: &wrapperspb.BoolValue{Value: true},
					},
					AuthMethod: &gloov1.Settings_VaultSecrets_AccessToken{
						AccessToken: testContext.VaultInstance().Token(),
					},
				},
			}
		}
		When("using single kube client", func() {
			BeforeEach(func() {
				settings.SecretOptions = &gloov1.Settings_SecretOptions{
					SecretSources: []*gloov1.Settings_SecretOptions_Source{
						{
							Source: &gloov1.Settings_SecretOptions_Source_Kubernetes{},
						},
					},
				}
			})
			universalTests()
			hasNSubClients(1)

		})
		When("using single directory client", func() {
			dir, err := os.MkdirTemp("", "secrets_client")
			Expect(err).NotTo(HaveOccurred())
			BeforeEach(func() {
				settings.SecretOptions = &gloov1.Settings_SecretOptions{
					SecretSources: []*gloov1.Settings_SecretOptions_Source{
						{
							Source: &gloov1.Settings_SecretOptions_Source_Directory{
								Directory: &gloov1.Settings_Directory{
									Directory: dir,
								},
							},
						},
					},
				}
			})
			AfterEach(func() {
				err := os.RemoveAll(dir)
				Expect(err).NotTo(HaveOccurred())
			})
			universalTests()
			hasNSubClients(1)
		})
		When("using single vault client", func() {
			BeforeEach(func() {
				settings.SecretOptions = &gloov1.Settings_SecretOptions{
					SecretSources: []*gloov1.Settings_SecretOptions_Source{
						{
							Source: getVaultSourceOptions(),
						},
					},
				}
			})
			universalTests()
			hasNSubClients(1)
		})
	})
})
