package syncutil_test

import (
	structpb "github.com/golang/protobuf/ptypes/struct"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/pkg/utils/syncutil"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	xdsproto "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/enterprise/options/extauth/v1"
	v1snap "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/gloosnapshot"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
)

var _ = Describe("Log Redactor", func() {
	var (
		privateKey = "RSA PRIVATE KEY CONTENT"
		caCrt      = "CA CERT CONTENT"

		noSecretsSnapshot = &v1.SetupSnapshot{
			Settings: []*v1.Settings{{
				Metadata: &core.Metadata{
					Name:      "settings",
					Namespace: "ns",
				},
			}},
		}
		snapshotWithSecrets = &v1snap.ApiSnapshot{
			Endpoints: []*v1.Endpoint{{
				Metadata: &core.Metadata{
					Name:      "endpoint",
					Namespace: "ns",
				},
			}},
			Secrets: []*v1.Secret{{
				Kind: &v1.Secret_Tls{Tls: &v1.TlsSecret{
					PrivateKey: privateKey,
				}},
				Metadata: &core.Metadata{
					Name:      "secret-name",
					Namespace: "ns",
				},
			}},
		}

		snapshotWithArtifacts = &v1snap.ApiSnapshot{
			Artifacts: []*v1.Artifact{{
				Data: map[string]string{
					"ca.crt": caCrt,
				},
				Metadata: &core.Metadata{
					Name:      "artifact-name",
					Namespace: "ns",
				},
			}},
		}
	)

	It("does not redact anything when no secrets", func() {
		Expect(syncutil.StringifySnapshot(noSecretsSnapshot)).NotTo(ContainSubstring(syncutil.Redacted))
	})

	It("replaces secret content with [REDACTED] placeholder", func() {
		s := syncutil.StringifySnapshot(snapshotWithSecrets)

		Expect(s).To(ContainSubstring(syncutil.Redacted))
		Expect(s).NotTo(ContainSubstring(privateKey))
	})

	It("replaces artifact content with [REDACTED] placeholder", func() {
		s := syncutil.StringifySnapshot(snapshotWithArtifacts)

		Expect(s).To(ContainSubstring(syncutil.Redacted))
		Expect(s).NotTo(ContainSubstring(caCrt))
	})

	DescribeTable("AuthConfig logging", func(secretPhrase string, config *xdsproto.ExtAuthConfig) {
		redactor := syncutil.NewProtoRedactor()

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
		}},
	}), Entry("hides API keys from logs", "my-secret-api-key", &xdsproto.ExtAuthConfig{
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
					Config: &structpb.Struct{
						Fields: map[string]*structpb.Value{
							"RequiredHeader": {
								Kind: &structpb.Value_StringValue{
									StringValue: "test-header",
								},
							},
							"AllowedValues": {
								Kind: &structpb.Value_ListValue{
									ListValue: &structpb.ListValue{
										Values: []*structpb.Value{
											{
												Kind: &structpb.Value_StringValue{
													StringValue: "allowed-header-1",
												},
											},
											{
												Kind: &structpb.Value_StringValue{
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
	}), Entry("hides ldap credentials from logs", "serviceusersecret", &xdsproto.ExtAuthConfig{
		AuthConfigRefName: "ref-name",
		Configs: []*xdsproto.ExtAuthConfig_Config{{
			AuthConfig: &xdsproto.ExtAuthConfig_Config_LdapInternal{
				LdapInternal: &xdsproto.ExtAuthConfig_LdapConfig{
					GroupLookupSettings: &xdsproto.ExtAuthConfig_LdapServiceAccountConfig{
						CheckGroupsWithServiceAccount: true,
						Username:                      "serviceusersecretuser",
						Password:                      "serviceusersecretpassword",
					},
				},
			},
		},
		},
	}), Entry("hides the hmac secrets from logs", "hmacsecret", &xdsproto.ExtAuthConfig{
		Configs: []*xdsproto.ExtAuthConfig_Config{{
			AuthConfig: &xdsproto.ExtAuthConfig_Config_HmacAuth{
				HmacAuth: &xdsproto.ExtAuthConfig_HmacAuthConfig{
					SecretStorage:      &xdsproto.ExtAuthConfig_HmacAuthConfig_SecretList{SecretList: &xdsproto.ExtAuthConfig_InMemorySecretList{SecretList: map[string]string{"hmacsecretuser": "hmacsecretpass"}}},
					ImplementationType: &xdsproto.ExtAuthConfig_HmacAuthConfig_ParametersInHeaders{ParametersInHeaders: &xdsproto.HmacParametersInHeaders{}},
				},
			},
		}},
	}), Entry("hides the encryption key value from logs", "encryptionKey", &xdsproto.ExtAuthConfig{
		Configs: []*xdsproto.ExtAuthConfig_Config{{
			AuthConfig: &xdsproto.ExtAuthConfig_Config_Oauth2{
				Oauth2: &xdsproto.ExtAuthConfig_OAuth2Config{
					OauthType: &xdsproto.ExtAuthConfig_OAuth2Config_OidcAuthorizationCode{
						OidcAuthorizationCode: &xdsproto.ExtAuthConfig_OidcAuthorizationCodeConfig{
							UserSession: &xdsproto.ExtAuthConfig_UserSessionConfig{
								CipherConfig: &xdsproto.ExtAuthConfig_UserSessionConfig_CipherConfig{
									Key: "encryptionKey",
								},
							},
						},
					},
				},
			},
		}},
	}),
	)
})
