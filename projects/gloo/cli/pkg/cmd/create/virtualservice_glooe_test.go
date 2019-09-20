package create_test

import (
	"fmt"

	"github.com/solo-io/gloo/projects/gloo/cli/pkg/constants"

	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/create"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/ginkgo/extensions/table"
	. "github.com/onsi/gomega"

	"github.com/solo-io/gloo/pkg/cliutil/testutil"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/helpers"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/testutils"
	extauthpb "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/enterprise/plugins/extauth/v1"
	pluginutils "github.com/solo-io/gloo/projects/gloo/pkg/plugins/utils"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
)

var _ = Describe("Virtualservice", func() {
	BeforeEach(func() {
		helpers.UseMemoryClients()
	})

	getExtension := func() extauthpb.VhostExtension {

		vs, err := helpers.MustVirtualServiceClient().Read("gloo-system", "vs1", clients.ReadOpts{})
		Expect(err).NotTo(HaveOccurred())
		Expect(vs.Metadata.Name).To(Equal("vs1"))

		var extension extauthpb.VhostExtension
		err = pluginutils.UnmarshalExtension(vs.GetVirtualHost().GetVirtualHostPlugins(), constants.ExtAuthExtensionName, &extension)
		Expect(err).NotTo(HaveOccurred())
		return extension
	}

	getOIDCConfig := func() *extauthpb.OAuth {
		return getExtension().Configs[0].GetOauth()
	}

	getApiKeyConfig := func() *extauthpb.ApiKeyAuth {
		return getExtension().Configs[0].GetApiKeyAuth()
	}
	getOpaConfig := func() *extauthpb.OpaAuth {
		return getExtension().Configs[0].GetOpaAuth()
	}

	get2ndOpaConfig := func() *extauthpb.OpaAuth {
		return getExtension().Configs[1].GetOpaAuth()
	}

	DescribeTable("should create oidc vhost",
		func(cmd string, expected extauthpb.OAuth) {
			err := testutils.Glooctl(cmd)
			Expect(err).NotTo(HaveOccurred())
			oidc := getOIDCConfig()
			Expect(*oidc).To(Equal(expected))
		},
		Entry("with oid config", "create vs --name vs1 --enable-oidc-auth --oidc-auth-client-id "+
			"1 --oidc-auth-app-url http://app.example.com --oidc-auth-client-secret-name fake "+
			"--oidc-auth-client-secret-namespace fakens --oidc-auth-issuer-url http://issuer.example.com "+
			"--oidc-auth-callback-path /cb",
			extauthpb.OAuth{
				ClientId: "1",
				ClientSecretRef: &core.ResourceRef{
					Name:      "fake",
					Namespace: "fakens",
				},
				CallbackPath: "/cb",
				IssuerUrl:    "http://issuer.example.com",
				AppUrl:       "http://app.example.com",
			}),
		Entry("with default callback", "create vs --name vs1 --enable-oidc-auth --oidc-auth-client-id "+
			"1 --oidc-auth-app-url http://app.example.com --oidc-auth-client-secret-name fake "+
			"--oidc-auth-client-secret-namespace fakens --oidc-auth-issuer-url http://issuer.example.com",
			extauthpb.OAuth{
				ClientId: "1",
				ClientSecretRef: &core.ResourceRef{
					Name:      "fake",
					Namespace: "fakens",
				},
				CallbackPath: "/oidc-gloo-callback",
				IssuerUrl:    "http://issuer.example.com",
				AppUrl:       "http://app.example.com",
			}),
		Entry("with default scopes", "create vs --name vs1 --enable-oidc-auth --oidc-auth-client-id "+
			"1 --oidc-auth-app-url http://app.example.com --oidc-auth-client-secret-name fake "+
			"--oidc-auth-client-secret-namespace fakens --oidc-auth-issuer-url http://issuer.example.com "+
			"--oidc-scope=scope1 --oidc-scope=scope2",
			extauthpb.OAuth{
				ClientId: "1",
				ClientSecretRef: &core.ResourceRef{
					Name:      "fake",
					Namespace: "fakens",
				},
				CallbackPath: "/oidc-gloo-callback",
				IssuerUrl:    "http://issuer.example.com",
				AppUrl:       "http://app.example.com",
				Scopes:       []string{"scope1", "scope2"},
			}),
	)

	DescribeTable("should create apikey vhost",
		func(cmd string, expected extauthpb.ApiKeyAuth) {
			err := testutils.Glooctl(cmd)
			Expect(err).NotTo(HaveOccurred())
			apiKey := getApiKeyConfig()
			Expect(*apiKey).To(Equal(expected))
		},
		Entry("with apikey config -- label selector", "create vs --name vs1 --enable-apikey-auth "+
			"--apikey-label-selector k1=v1",
			extauthpb.ApiKeyAuth{
				LabelSelector:    map[string]string{"k1": "v1"},
				ApiKeySecretRefs: nil,
			}),

		Entry("with apikey config -- secret refs", "create vs --name vs1 --enable-apikey-auth "+
			"--apikey-secret-namespace ns1 --apikey-secret-name s1 ",
			extauthpb.ApiKeyAuth{
				LabelSelector: nil,
				ApiKeySecretRefs: []*core.ResourceRef{
					{
						Namespace: "ns1",
						Name:      "s1",
					},
				},
			}),
		Entry("with apikey config -- both groups & secret refs", "create vs --name vs1 --enable-apikey-auth "+
			"--apikey-label-selector k1=v1 --apikey-secret-namespace ns1 --apikey-secret-name s1 ",
			extauthpb.ApiKeyAuth{
				LabelSelector: map[string]string{"k1": "v1"},
				ApiKeySecretRefs: []*core.ResourceRef{
					{
						Namespace: "ns1",
						Name:      "s1",
					},
				},
			}),
	)

	Context("ApiKey virtual service errors", func() {
		It("throws error if namespace provided and name omitted ", func() {
			_, err := testutils.GlooctlOut("create vs --name vs1 --enable-apikey-auth " +
				"--apikey-secret-namespace ns1")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(Equal(create.ProvideNamespaceAndNameError("ns1", "").Error()))
		})
		It("throws error if name provided and namespace omitted ", func() {
			_, err := testutils.GlooctlOut("create vs --name vs1 --enable-apikey-auth " +
				"--apikey-secret-name s1")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(Equal(create.ProvideNamespaceAndNameError("", "s1").Error()))
		})
	})

	DescribeTable("should create opa vhost",
		func(cmd string, expected extauthpb.OpaAuth) {
			err := testutils.Glooctl(cmd)
			Expect(err).NotTo(HaveOccurred())
			opa := getOpaConfig()
			Expect(*opa).To(Equal(expected))
		},
		Entry("with opa query and no modules", "create vs --name vs1 --enable-opa-auth "+
			"--opa-query test",
			extauthpb.OpaAuth{
				Query: "test",
			}),
		Entry("with opa query and some modules", "create vs --name vs1 --enable-opa-auth "+
			"--opa-query test --opa-module-ref ns1.name1 --opa-module-ref ns2.name2",
			extauthpb.OpaAuth{
				Query:   "test",
				Modules: []*core.ResourceRef{{Namespace: "ns1", Name: "name1"}, {Namespace: "ns2", Name: "name2"}},
			}),
	)

	It("should create opa after oidc", func() {
		err := testutils.Glooctl("create vs --name vs1 " +
			"--enable-oidc-auth --oidc-auth-client-id 1 " +
			"--oidc-auth-app-url http://app.example.com --oidc-auth-client-secret-name fake " +
			"--oidc-auth-client-secret-namespace fakens --oidc-auth-issuer-url http://issuer.example.com " +
			"--oidc-auth-callback-path /cb " +
			"--enable-opa-auth --opa-query test --opa-module-ref ns1.name1 --opa-module-ref ns2.name2")
		Expect(err).NotTo(HaveOccurred())

		expected := extauthpb.OpaAuth{
			Query:   "test",
			Modules: []*core.ResourceRef{{Namespace: "ns1", Name: "name1"}, {Namespace: "ns2", Name: "name2"}},
		}

		oidc := getOIDCConfig()
		Expect(oidc).NotTo(BeNil())

		opa := get2ndOpaConfig()
		Expect(opa).NotTo(BeNil())
		Expect(*opa).To(Equal(expected))

	})

	Context("OPA virtual service errors", func() {
		It("throws error if no query provided ", func() {
			_, err := testutils.GlooctlOut("create vs --name vs1 --enable-opa-auth")
			Expect(err).To(MatchError(create.EmptyQueryError))
		})
	})

	Context("Interactive tests", func() {

		It("should create vs with no rate limits and auth", func() {
			testutil.ExpectInteractive(func(c *testutil.Console) {
				c.ExpectString("Add a domain for this virtual service (empty defaults to all domains)?")
				c.SendLine("")
				c.ExpectString("do you wish to add rate limiting to the virtual service")
				c.SendLine("n")
				c.ExpectString("do you wish to add oidc auth to the virtual service")
				c.SendLine("n")
				c.ExpectString("do you wish to add apikey auth to the virtual service")
				c.SendLine("n")
				c.ExpectString("do you wish to add OPA auth to the virtual service")
				c.SendLine("n")

				c.ExpectString("Use default namespace (gloo-system)?")
				c.SendLine("")
				c.ExpectString("name of the resource:")
				c.SendLine("default")

				c.ExpectEOF()
			}, func() {
				err := testutils.Glooctl("create vs -i")
				Expect(err).NotTo(HaveOccurred())
				_, err = helpers.MustVirtualServiceClient().Read("gloo-system", "default", clients.ReadOpts{})
				Expect(err).NotTo(HaveOccurred())
			})
		})

		It("should create vs with oidc auth", func() {
			testutil.ExpectInteractive(func(c *testutil.Console) {
				c.ExpectString("Add a domain for this virtual service (empty defaults to all domains)?")
				c.SendLine("")
				c.ExpectString("do you wish to add rate limiting to the virtual service")
				c.SendLine("n")
				c.ExpectString("do you wish to add oidc auth to the virtual service")
				c.SendLine("y")
				c.ExpectString("What is your app url?")
				c.SendLine("http://app.example.com")
				c.ExpectString("What is your issuer url?")
				c.SendLine("https://accounts.google.com")
				c.ExpectString("What path (relative to your app url) should we use as a callback from the issuer?")
				c.SendLine("/auth-callback")
				c.ExpectString("What is your client id?")
				c.SendLine("me")
				c.ExpectString("What is your client secret name?")
				c.SendLine("secret-name")
				c.ExpectString("What is your client secret namespace?")
				c.SendLine("gloo-system")
				c.ExpectString("provide additional scopes to request (empty to finish)")
				c.SendLine("scope1")
				c.ExpectString("provide additional scopes to request (empty to finish)")
				c.SendLine("")

				c.ExpectString("do you wish to add apikey auth to the virtual service")
				c.SendLine("n")

				c.ExpectString("do you wish to add OPA auth to the virtual service")
				c.SendLine("n")

				c.ExpectString("Use default namespace (gloo-system)?")
				c.SendLine("")
				c.ExpectString("name of the resource:")
				c.SendLine("vs1")

				c.ExpectEOF()
			}, func() {
				err := testutils.Glooctl("create vs -i")
				Expect(err).NotTo(HaveOccurred())

				oidc := getOIDCConfig()
				expected := extauthpb.OAuth{
					ClientId: "me",
					ClientSecretRef: &core.ResourceRef{
						Name:      "secret-name",
						Namespace: "gloo-system",
					},
					CallbackPath: "/auth-callback",
					IssuerUrl:    "https://accounts.google.com",
					AppUrl:       "http://app.example.com",
					Scopes:       []string{"scope1"},
				}
				Expect(*oidc).To(Equal(expected))

			})
		})

		It("should create vs with apikey auth", func() {
			testutil.ExpectInteractive(func(c *testutil.Console) {
				c.ExpectString("Add a domain for this virtual service (empty defaults to all domains)")
				c.SendLine("")
				c.ExpectString("do you wish to add rate limiting to the virtual service")
				c.SendLine("n")
				c.ExpectString("do you wish to add oidc auth to the virtual service")
				c.SendLine("n")
				c.ExpectString("do you wish to add apikey auth to the virtual service")
				c.SendLine("y")

				c.ExpectString("provide a label (key=value) to be a part of your label selector (empty to finish)")
				c.SendLine("k1=v1")
				c.ExpectString("provide a label (key=value) to be a part of your label selector (empty to finish)")
				c.SendLine("")
				c.ExpectString("apikey secret name to attach to this virtual service? (empty to skip)")
				c.SendLine("s1")
				c.ExpectString("provide a namespace to search for the secret in (empty to finish)")
				c.SendLine("ns1")

				c.ExpectString("do you wish to add OPA auth to the virtual service")
				c.SendLine("n")

				c.ExpectString("Use default namespace (gloo-system)?")
				c.SendLine("")
				c.ExpectString("name of the resource:")
				c.SendLine("vs1")

				c.ExpectEOF()
			}, func() {
				err := testutils.Glooctl("create vs -i")
				Expect(err).NotTo(HaveOccurred())

				apiKey := getApiKeyConfig()
				expected := extauthpb.ApiKeyAuth{
					LabelSelector: map[string]string{"k1": "v1"},
					ApiKeySecretRefs: []*core.ResourceRef{
						{
							Namespace: "ns1",
							Name:      "s1",
						},
					},
				}
				Expect(*apiKey).To(Equal(expected))

			})
		})

		It("should create vs with OPA auth", func() {
			testutil.ExpectInteractive(func(c *testutil.Console) {
				c.ExpectString("Add a domain for this virtual service (empty defaults to all domains)")
				c.SendLine("")
				c.ExpectString("do you wish to add rate limiting to the virtual service")
				c.SendLine("n")
				c.ExpectString("do you wish to add oidc auth to the virtual service")
				c.SendLine("n")
				c.ExpectString("do you wish to add apikey auth to the virtual service")
				c.SendLine("n")

				c.ExpectString("do you wish to add OPA auth to the virtual service")
				c.SendLine("y")

				c.ExpectString("OPA query to attach to this virtual service?")
				c.SendLine("test")
				c.ExpectString("provide references to config maps used as OPA modules in resolving above query (empty to finish)")
				c.SendLine("ns1.name1")
				c.ExpectString("provide references to config maps used as OPA modules in resolving above query (empty to finish)")
				c.SendLine("")

				c.ExpectString("Use default namespace (gloo-system)?")
				c.SendLine("")
				c.ExpectString("name of the resource:")
				c.SendLine("vs1")

				c.ExpectEOF()
			}, func() {
				err := testutils.Glooctl("create vs -i")
				Expect(err).NotTo(HaveOccurred())

				opa := getOpaConfig()
				expected := extauthpb.OpaAuth{
					Query: "test",
					Modules: []*core.ResourceRef{
						{
							Namespace: "ns1",
							Name:      "name1",
						},
					},
				}
				Expect(*opa).To(Equal(expected))

			})
		})

	})

	var _ = Describe("dry-run", func() {
		It("can print as kube yaml in dry run", func() {
			out, err := testutils.GlooctlOut("create virtualservice kube --dry-run --name vs --domains foo.bar,baz.qux")
			Expect(err).NotTo(HaveOccurred())
			fmt.Print(out)
			Expect(out).To(Equal(`apiVersion: gateway.solo.io/v1
kind: VirtualService
metadata:
  creationTimestamp: null
  name: vs
  namespace: gloo-system
spec:
  displayName: vs
  virtualHost:
    domains:
    - foo.bar
    - baz.qux
status: {}
`))
		})

		It("can print as solo-kit yaml in dry run", func() {
			out, err := testutils.GlooctlOut("create virtualservice kube --dry-run -oyaml --name vs --domains foo.bar,baz.qux")
			Expect(err).NotTo(HaveOccurred())
			fmt.Print(out)
			Expect(out).To(Equal(`---
displayName: vs
metadata:
  name: vs
  namespace: gloo-system
status: {}
virtualHost:
  domains:
  - foo.bar
  - baz.qux
`))
		})
	})

})
