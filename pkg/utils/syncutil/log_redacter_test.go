package syncutil_test

import (
	"github.com/gogo/protobuf/types"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/ginkgo/extensions/table"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/pkg/utils/syncutil"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	xdsproto "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/enterprise/options/extauth/v1"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
)

var _ = Describe("Log Redacter", func() {
	var (
		secretName      = "my-test-secret"
		secretNamespace = "my-secret-namespace"
		privateKey      = "RSA PRIVATE KEY CONTENT"

		noSecretsSnapshot = &v1.SetupSnapshot{
			Settings: []*v1.Settings{{
				Metadata: core.Metadata{
					Name:      "settings",
					Namespace: "ns",
				},
			}},
		}
		snapshotWithSecrets = &v1.ApiSnapshot{
			Endpoints: []*v1.Endpoint{{
				Metadata: core.Metadata{
					Name:      "endpoint",
					Namespace: "ns",
				},
			}},
			Secrets: []*v1.Secret{{
				Kind: &v1.Secret_Tls{Tls: &v1.TlsSecret{
					PrivateKey: privateKey,
				}},
				Metadata: core.Metadata{
					Name:      secretName,
					Namespace: secretNamespace,
				},
			}},
		}
	)

	It("does not redact anything when no secrets", func() {
		Expect(syncutil.StringifySnapshot(noSecretsSnapshot)).NotTo(ContainSubstring(syncutil.Redacted))
	})

	It("contains redacted content when secrets are present", func() {
		s := syncutil.StringifySnapshot(snapshotWithSecrets)

		Expect(s).To(ContainSubstring(syncutil.Redacted))
		Expect(s).NotTo(ContainSubstring(privateKey))
	})

	DescribeTable("AuthConfig logging", func(secretPhrase string, config *xdsproto.ExtAuthConfig) {
		redactor := syncutil.NewProtoRedactor(syncutil.LogRedactorTag, syncutil.LogRedactorTagValue)

		jsonString, err := redactor.BuildRedactedJsonString(config)
		Expect(err).NotTo(HaveOccurred())
		Expect(jsonString).NotTo(ContainSubstring(secretPhrase))
	}, Entry("can hide OAuth secret data", "client-secret-data-should-be-redacted", &xdsproto.ExtAuthConfig{
		AuthConfigRefName: "ref-name",
		Configs: []*xdsproto.ExtAuthConfig_Config{{
			AuthConfig: &xdsproto.ExtAuthConfig_Config_Oauth{
				Oauth: &xdsproto.ExtAuthConfig_OAuthConfig{
					ClientId:                "client-id",
					ClientSecret:            "client-secret-data-should-be-redacted",
					IssuerUrl:               "issuer",
					AuthEndpointQueryParams: nil,
					AppUrl:                  "app-url.com",
					CallbackPath:            "/callback",
				},
			},
		}},
	}), Entry("doesn't hide anything for BasicAuth configs", "irrelevant-here", &xdsproto.ExtAuthConfig{
		AuthConfigRefName: "ref-name",
		Configs: []*xdsproto.ExtAuthConfig_Config{{
			AuthConfig: &xdsproto.ExtAuthConfig_Config_BasicAuth{
				BasicAuth: &xdsproto.BasicAuth{
					Realm: "realm.com",
					Apr: &xdsproto.BasicAuth_Apr{
						Users: map[string]*xdsproto.BasicAuth_Apr_SaltedHashedPassword{
							"user1": {HashedPassword: "hash", Salt: "salt"},
						},
					},
				},
			},
		}}}), Entry("hides API keys from logs", "my-secret-api-key", &xdsproto.ExtAuthConfig{
		AuthConfigRefName: "ref-name",
		Configs: []*xdsproto.ExtAuthConfig_Config{{
			AuthConfig: &xdsproto.ExtAuthConfig_Config_ApiKeyAuth{
				ApiKeyAuth: &xdsproto.ExtAuthConfig_ApiKeyAuthConfig{
					ValidApiKeys: map[string]*xdsproto.ExtAuthConfig_ApiKeyAuthConfig_KeyMetadata{
						"my-secret-api-key": {
							Username: "user-name",
						},
					},
				},
			},
		}},
	}), Entry("doesn't hide anything for plugin auth", "irrelevant", &xdsproto.ExtAuthConfig{
		Configs: []*xdsproto.ExtAuthConfig_Config{{
			AuthConfig: &xdsproto.ExtAuthConfig_Config_PluginAuth{
				PluginAuth: &xdsproto.AuthPlugin{
					Name: "plugin-name",
					Config: &types.Struct{
						Fields: map[string]*types.Value{
							"RequiredHeader": {
								Kind: &types.Value_StringValue{
									StringValue: "test-header",
								},
							},
							"AllowedValues": {
								Kind: &types.Value_ListValue{
									ListValue: &types.ListValue{
										Values: []*types.Value{
											{
												Kind: &types.Value_StringValue{
													StringValue: "allowed-header-1",
												},
											},
											{
												Kind: &types.Value_StringValue{
													StringValue: "allowed-header-2",
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		}},
	}), Entry("doesn't hide anything for Opa Auth", "irrelevant", &xdsproto.ExtAuthConfig{
		Configs: []*xdsproto.ExtAuthConfig_Config{{
			AuthConfig: &xdsproto.ExtAuthConfig_Config_OpaAuth{
				OpaAuth: &xdsproto.ExtAuthConfig_OpaAuthConfig{
					Modules: map[string]string{"module1": "path", "module2": "path"},
					Query:   "test-query",
				},
			},
		}},
	}), Entry("doesn't hide anything for LDAP auth", "irrelevant", &xdsproto.ExtAuthConfig{
		Configs: []*xdsproto.ExtAuthConfig_Config{{
			AuthConfig: &xdsproto.ExtAuthConfig_Config_Ldap{
				Ldap: &xdsproto.Ldap{
					AllowedGroups: []string{"test1", "test2"},
				},
			},
		}},
	}),
	)
})
