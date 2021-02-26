package utils

import (
	envoycore "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoygrpccredential "github.com/envoyproxy/go-control-plane/envoy/config/grpc_credential/v3"
	envoyauth "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
	"github.com/golang/protobuf/ptypes"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/ginkgo/extensions/table"
	. "github.com/onsi/gomega"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	. "github.com/solo-io/go-utils/testutils"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	test_matchers "github.com/solo-io/solo-kit/test/matchers"
)

var _ = Describe("Ssl", func() {

	var (
		upstreamCfg            *v1.UpstreamSslConfig
		downstreamCfg          *v1.SslConfig
		tlsSecret              *v1.TlsSecret
		secret                 *v1.Secret
		secrets                v1.SecretList
		configTranslator       *sslConfigTranslator
		resolveCommonSslConfig func(cs CertSource, secrets v1.SecretList) (*envoyauth.CommonTlsContext, error)
	)

	Context("files", func() {
		BeforeEach(func() {
			upstreamCfg = &v1.UpstreamSslConfig{
				Sni: "test.com",
				SslSecrets: &v1.UpstreamSslConfig_SslFiles{
					SslFiles: &v1.SSLFiles{
						RootCa:  "rootca",
						TlsCert: "tlscert",
						TlsKey:  "tlskey",
					},
				},
			}
			downstreamCfg = &v1.SslConfig{
				SniDomains: []string{"test.com", "test1.com"},
				SslSecrets: &v1.SslConfig_SslFiles{
					SslFiles: &v1.SSLFiles{
						RootCa:  "rootca",
						TlsCert: "tlscert",
						TlsKey:  "tlskey",
					},
				},
			}
			configTranslator = NewSslConfigTranslator()
			resolveCommonSslConfig = func(cs CertSource, secrets v1.SecretList) (*envoyauth.CommonTlsContext, error) {
				return configTranslator.ResolveCommonSslConfig(cs, secrets, false)
			}

		})

		DescribeTable("should resolve from files",
			func(c func() CertSource) {
				ValidateCommonContextFiles(resolveCommonSslConfig(c(), nil))
			},
			Entry("upstreamCfg", func() CertSource { return upstreamCfg }),
			Entry("downstreamCfg", func() CertSource { return downstreamCfg }),
		)

		Context("san", func() {
			It("should error with san and not rootca", func() {
				upstreamCfg.SslSecrets.(*v1.UpstreamSslConfig_SslFiles).SslFiles.RootCa = ""
				upstreamCfg.VerifySubjectAltName = []string{"test"}
				_, err := resolveCommonSslConfig(upstreamCfg, nil)
				Expect(err).To(Equal(RootCaMustBeProvidedError))
			})

			It("should add SAN verification when provided", func() {
				upstreamCfg.VerifySubjectAltName = []string{"test"}
				c, err := resolveCommonSslConfig(upstreamCfg, nil)
				Expect(err).NotTo(HaveOccurred())
				vctx := c.ValidationContextType.(*envoyauth.CommonTlsContext_ValidationContext).ValidationContext
				Expect(vctx.MatchSubjectAltNames).To(Equal(verifySanListToMatchSanList(upstreamCfg.VerifySubjectAltName)))
			})
		})
	})
	Context("secret", func() {
		BeforeEach(func() {
			tlsSecret = &v1.TlsSecret{
				CertChain:  "tlscert",
				PrivateKey: "tlskey",
				RootCa:     "rootca",
			}
			secret = &v1.Secret{
				Kind: &v1.Secret_Tls{
					Tls: tlsSecret,
				},
				Metadata: &core.Metadata{
					Name:      "secret",
					Namespace: "secret",
				},
			}
			ref := secret.Metadata.Ref()
			secrets = v1.SecretList{secret}
			upstreamCfg = &v1.UpstreamSslConfig{
				Sni: "test.com",
				SslSecrets: &v1.UpstreamSslConfig_SecretRef{
					SecretRef: ref,
				},
			}
			downstreamCfg = &v1.SslConfig{
				SniDomains: []string{"test.com", "test1.com"},
				SslSecrets: &v1.SslConfig_SecretRef{
					SecretRef: ref,
				},
			}
			configTranslator = NewSslConfigTranslator()
		})

		It("should error with no secret", func() {
			configTranslator = NewSslConfigTranslator()
			_, err := resolveCommonSslConfig(upstreamCfg, nil)
			Expect(err).To(HaveOccurred())
		})

		It("should error with wrong secret", func() {
			secret.Kind = &v1.Secret_Aws{}
			_, err := resolveCommonSslConfig(upstreamCfg, secrets)
			Expect(err).To(HaveOccurred())
			Expect(err).To(HaveInErrorChain(NotTlsSecretError(secret.GetMetadata().Ref())))
		})

		DescribeTable("should resolve from secret refs",
			func(c func() CertSource) {
				ValidateCommonContextInline(resolveCommonSslConfig(c(), secrets))
			},
			Entry("upstreamCfg", func() CertSource { return upstreamCfg }),
			Entry("downstreamCfg", func() CertSource { return downstreamCfg }),
		)
		DescribeTable("should fail if only cert is not provided",
			func(c func() CertSource) {
				tlsSecret.CertChain = ""
				_, err := resolveCommonSslConfig(c(), secrets)
				Expect(err).To(HaveOccurred())

			},
			Entry("upstreamCfg", func() CertSource { return upstreamCfg }),
			Entry("downstreamCfg", func() CertSource { return downstreamCfg }),
		)
		DescribeTable("should fail if only private key is not provided",
			func(c func() CertSource) {
				tlsSecret.PrivateKey = ""
				_, err := resolveCommonSslConfig(c(), secrets)
				Expect(err).To(HaveOccurred())

			},
			Entry("upstreamCfg", func() CertSource { return upstreamCfg }),
			Entry("downstreamCfg", func() CertSource { return downstreamCfg }),
		)
		DescribeTable("should not have validation context if no rootca",
			func(c func() CertSource) {
				tlsSecret.RootCa = ""
				cfg, err := resolveCommonSslConfig(c(), secrets)
				Expect(err).NotTo(HaveOccurred())
				Expect(cfg.ValidationContextType).To(BeNil())

			},
			Entry("upstreamCfg", func() CertSource { return upstreamCfg }),
			Entry("downstreamCfg", func() CertSource { return downstreamCfg }),
		)

		It("should set require client cert for downstream config", func() {
			cfg, err := configTranslator.ResolveDownstreamSslConfig(secrets, downstreamCfg)
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.RequireClientCertificate.GetValue()).To(BeTrue())
		})

		It("should set alpn default for downstream config", func() {
			cfg, err := configTranslator.ResolveDownstreamSslConfig(secrets, downstreamCfg)
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.CommonTlsContext.AlpnProtocols).To(Equal([]string{"h2", "http/1.1"}))
		})

		It("should set alpn for downstream config", func() {
			downstreamCfg.AlpnProtocols = []string{"test"}
			cfg, err := configTranslator.ResolveDownstreamSslConfig(secrets, downstreamCfg)
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.CommonTlsContext.AlpnProtocols).To(Equal([]string{"test"}))
		})

		It("should NOT set default alpn for upstream config", func() {
			cfg, err := configTranslator.ResolveUpstreamSslConfig(secrets, upstreamCfg)
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.CommonTlsContext.AlpnProtocols).To(BeEmpty())
		})

		It("should set alpn for upstream config", func() {
			upstreamCfg.AlpnProtocols = []string{"test"}
			cfg, err := configTranslator.ResolveUpstreamSslConfig(secrets, upstreamCfg)
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.CommonTlsContext.AlpnProtocols).To(Equal([]string{"test"}))
		})

		It("should not set require client cert for downstream config with no rootca", func() {
			tlsSecret.RootCa = ""
			cfg, err := configTranslator.ResolveDownstreamSslConfig(secrets, downstreamCfg)
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.RequireClientCertificate.GetValue()).To(BeFalse())
		})

		It("should require cert and key for downstream config", func() {
			downstreamCfg.SslSecrets = nil
			cfg, err := configTranslator.ResolveDownstreamSslConfig(secrets, downstreamCfg)
			Expect(err).To(HaveOccurred())
			Expect(cfg).To(BeNil())
		})

		It("should not require certs or rootca for upstream config", func() {
			upstreamCfg.SslSecrets = nil
			cfg, err := configTranslator.ResolveUpstreamSslConfig(secrets, upstreamCfg)
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg).NotTo(BeNil())
		})

		It("should set sni for upstream config", func() {
			cfg, err := configTranslator.ResolveUpstreamSslConfig(secrets, upstreamCfg)
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.Sni).To(Equal("test.com"))
		})

		Context("san", func() {
			It("should error with san and not rootca", func() {
				tlsSecret.RootCa = ""
				upstreamCfg.VerifySubjectAltName = []string{"test"}
				_, err := configTranslator.ResolveCommonSslConfig(upstreamCfg, secrets, true)
				Expect(err).To(Equal(RootCaMustBeProvidedError))
				_, err = configTranslator.ResolveCommonSslConfig(upstreamCfg, secrets, false)
				Expect(err).To(Equal(RootCaMustBeProvidedError))
			})

			It("should add SAN verification when provided", func() {
				upstreamCfg.VerifySubjectAltName = []string{"test"}
				c, err := configTranslator.ResolveCommonSslConfig(upstreamCfg, secrets, false)
				Expect(err).NotTo(HaveOccurred())
				vctx := c.ValidationContextType.(*envoyauth.CommonTlsContext_ValidationContext).ValidationContext
				Expect(vctx.MatchSubjectAltNames).To(Equal(verifySanListToMatchSanList(upstreamCfg.VerifySubjectAltName)))

				c, err = configTranslator.ResolveCommonSslConfig(upstreamCfg, secrets, true)
				Expect(err).NotTo(HaveOccurred())
				vctx = c.ValidationContextType.(*envoyauth.CommonTlsContext_ValidationContext).ValidationContext
				Expect(vctx.MatchSubjectAltNames).To(Equal(verifySanListToMatchSanList(upstreamCfg.VerifySubjectAltName)))
			})
		})

		// This logic is the same in files and sds so it only needs to be tested once.
		Context("tls params", func() {

			It("should add TLS Params when provided", func() {
				upstreamCfg.Parameters = &v1.SslParameters{
					MinimumProtocolVersion: v1.SslParameters_TLSv1_1,
					MaximumProtocolVersion: v1.SslParameters_TLSv1_2,
					CipherSuites:           []string{"cipher-test"},
					EcdhCurves:             []string{"ec-dh-test"},
				}
				c, err := resolveCommonSslConfig(upstreamCfg, secrets)
				Expect(err).NotTo(HaveOccurred())
				expectParams := &envoyauth.TlsParameters{
					TlsMinimumProtocolVersion: envoyauth.TlsParameters_TLSv1_1,
					TlsMaximumProtocolVersion: envoyauth.TlsParameters_TLSv1_2,
					CipherSuites:              []string{"cipher-test"},
					EcdhCurves:                []string{"ec-dh-test"},
				}
				Expect(c.TlsParams).To(Equal(expectParams))
			})
		})

	})

	Context("sds", func() {
		var (
			sdsConfig *v1.SDSConfig
		)
		BeforeEach(func() {
			sdsConfig = &v1.SDSConfig{
				TargetUri:              "TargetUri",
				CertificatesSecretName: "CertificatesSecretName",
				ValidationContextName:  "ValidationContextName",
			}
			upstreamCfg = &v1.UpstreamSslConfig{
				Sni: "test.com",
				SslSecrets: &v1.UpstreamSslConfig_Sds{
					Sds: sdsConfig,
				},
			}
			configTranslator = NewSslConfigTranslator()
		})

		It("should have a sds setup with a default cluster name", func() {
			c, err := resolveCommonSslConfig(upstreamCfg, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(c.TlsCertificateSdsSecretConfigs).To(HaveLen(1))
			Expect(c.ValidationContextType).ToNot(BeNil())

			vctx := c.ValidationContextType.(*envoyauth.CommonTlsContext_ValidationContextSdsSecretConfig).ValidationContextSdsSecretConfig
			cert := c.TlsCertificateSdsSecretConfigs[0]
			Expect(vctx.Name).To(Equal("ValidationContextName"))
			Expect(cert.Name).To(Equal("CertificatesSecretName"))
			// If they are no equivalent, it means that any serialization is different.
			// see here: https://github.com/envoyproxy/go-control-plane/pull/158
			// and here: https://github.com/envoyproxy/envoy/pull/6241
			// this may lead to envoy updates being too frequent
			Expect(vctx.SdsConfig).To(BeEquivalentTo(cert.SdsConfig))

			envoyGrpc := vctx.SdsConfig.ConfigSourceSpecifier.(*envoycore.ConfigSource_ApiConfigSource).ApiConfigSource.GrpcServices[0].TargetSpecifier.(*envoycore.GrpcService_EnvoyGrpc_).EnvoyGrpc
			Expect(envoyGrpc.ClusterName).To(Equal("gateway_proxy_sds"))

		})

		It("should have a sds setup with a custom cluster name", func() {
			cfgCustomCluster := &v1.UpstreamSslConfig{
				Sni: "test.com",
				SslSecrets: &v1.UpstreamSslConfig_Sds{
					Sds: &v1.SDSConfig{
						TargetUri:              "TargetUri",
						CertificatesSecretName: "CertificatesSecretName",
						ValidationContextName:  "ValidationContextName",
						SdsBuilder: &v1.SDSConfig_ClusterName{
							ClusterName: "custom-cluster",
						},
					},
				},
			}
			c, err := resolveCommonSslConfig(cfgCustomCluster, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(c.TlsCertificateSdsSecretConfigs).To(HaveLen(1))
			Expect(c.ValidationContextType).ToNot(BeNil())

			vctx := c.ValidationContextType.(*envoyauth.CommonTlsContext_ValidationContextSdsSecretConfig).ValidationContextSdsSecretConfig
			cert := c.TlsCertificateSdsSecretConfigs[0]
			Expect(vctx.Name).To(Equal("ValidationContextName"))
			Expect(cert.Name).To(Equal("CertificatesSecretName"))
			// If they are no equivalent, it means that any serialization is different.
			// see here: https://github.com/envoyproxy/go-control-plane/pull/158
			// and here: https://github.com/envoyproxy/envoy/pull/6241
			// this may lead to envoy updates being too frequent
			Expect(vctx.SdsConfig).To(BeEquivalentTo(cert.SdsConfig))

			envoyGrpc := vctx.SdsConfig.ConfigSourceSpecifier.(*envoycore.ConfigSource_ApiConfigSource).ApiConfigSource.GrpcServices[0].TargetSpecifier.(*envoycore.GrpcService_EnvoyGrpc_).EnvoyGrpc
			Expect(envoyGrpc.ClusterName).To(Equal("custom-cluster"))

		})

		Context("san", func() {
			It("should error with san and not validationContext", func() {
				sdsConfig.ValidationContextName = ""
				upstreamCfg.VerifySubjectAltName = []string{"test"}
				_, err := resolveCommonSslConfig(upstreamCfg, nil)
				Expect(err).To(Equal(MissingValidationContextError))
			})

			It("should add SAN verification when provided", func() {
				upstreamCfg.VerifySubjectAltName = []string{"test"}
				c, err := resolveCommonSslConfig(upstreamCfg, nil)
				Expect(err).NotTo(HaveOccurred())
				vctx := c.ValidationContextType.(*envoyauth.CommonTlsContext_CombinedValidationContext).CombinedValidationContext
				Expect(vctx.DefaultValidationContext.MatchSubjectAltNames).To(Equal(verifySanListToMatchSanList(upstreamCfg.VerifySubjectAltName)))
			})
		})
	})

	Context("sds with tokenFile", func() {
		var (
			sdsConfig *v1.SDSConfig
		)
		BeforeEach(func() {
			sdsConfig = &v1.SDSConfig{
				TargetUri:              "TargetUri",
				CertificatesSecretName: "CertificatesSecretName",
				ValidationContextName:  "ValidationContextName",
				SdsBuilder: &v1.SDSConfig_CallCredentials{
					CallCredentials: &v1.CallCredentials{
						FileCredentialSource: &v1.CallCredentials_FileCredentialSource{
							TokenFileName: "TokenFileName",
							Header:        "Header",
						},
					},
				},
			}
			upstreamCfg = &v1.UpstreamSslConfig{
				Sni: "test.com",
				SslSecrets: &v1.UpstreamSslConfig_Sds{
					Sds: sdsConfig,
				},
			}
			configTranslator = NewSslConfigTranslator()
		})

		It("should have a sds setup with a file-based metadata", func() {
			c, err := resolveCommonSslConfig(upstreamCfg, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(c.TlsCertificateSdsSecretConfigs).To(HaveLen(1))
			Expect(c.ValidationContextType).ToNot(BeNil())

			vctx := c.ValidationContextType.(*envoyauth.CommonTlsContext_ValidationContextSdsSecretConfig).ValidationContextSdsSecretConfig
			cert := c.TlsCertificateSdsSecretConfigs[0]
			Expect(vctx.Name).To(Equal("ValidationContextName"))
			Expect(cert.Name).To(Equal("CertificatesSecretName"))
			// If they are no equivalent, it means that any serialization is different.
			// see here: https://github.com/envoyproxy/go-control-plane/pull/158
			// and here: https://github.com/envoyproxy/envoy/pull/6241
			// this may lead to envoy updates being too frequent
			Expect(vctx.SdsConfig).To(BeEquivalentTo(cert.SdsConfig))

			getGrpcConfig := func(s *envoyauth.SdsSecretConfig) *envoycore.GrpcService_GoogleGrpc {
				return s.SdsConfig.ConfigSourceSpecifier.(*envoycore.ConfigSource_ApiConfigSource).ApiConfigSource.GrpcServices[0].TargetSpecifier.(*envoycore.GrpcService_GoogleGrpc_).GoogleGrpc
			}

			Expect(getGrpcConfig(vctx).ChannelCredentials).To(BeEquivalentTo(&envoycore.GrpcService_GoogleGrpc_ChannelCredentials{
				CredentialSpecifier: &envoycore.GrpcService_GoogleGrpc_ChannelCredentials_LocalCredentials{
					LocalCredentials: &envoycore.GrpcService_GoogleGrpc_GoogleLocalCredentials{},
				},
			}))
			Expect(getGrpcConfig(vctx).CredentialsFactoryName).To(Equal(MetadataPluginName))

			credPlugin := getGrpcConfig(vctx).CallCredentials[0].CredentialSpecifier.(*envoycore.GrpcService_GoogleGrpc_CallCredentials_FromPlugin).FromPlugin
			Expect(credPlugin.Name).To(Equal(MetadataPluginName))
			var credConfig envoygrpccredential.FileBasedMetadataConfig
			ptypes.UnmarshalAny(credPlugin.GetTypedConfig(), &credConfig)

			Expect(&credConfig).To(test_matchers.MatchProto(&envoygrpccredential.FileBasedMetadataConfig{
				SecretData: &envoycore.DataSource{
					Specifier: &envoycore.DataSource_Filename{
						Filename: "TokenFileName",
					},
				},
				HeaderKey: "Header",
			}))

		})

		Context("san", func() {
			It("should error with san and not validationContext", func() {
				sdsConfig.ValidationContextName = ""
				upstreamCfg.VerifySubjectAltName = []string{"test"}
				_, err := resolveCommonSslConfig(upstreamCfg, nil)
				Expect(err).To(Equal(MissingValidationContextError))
			})

			It("should add SAN verification when provided", func() {
				upstreamCfg.VerifySubjectAltName = []string{"test"}
				c, err := resolveCommonSslConfig(upstreamCfg, nil)
				Expect(err).NotTo(HaveOccurred())
				vctx := c.ValidationContextType.(*envoyauth.CommonTlsContext_CombinedValidationContext).CombinedValidationContext
				Expect(vctx.DefaultValidationContext.MatchSubjectAltNames).To(Equal(verifySanListToMatchSanList(upstreamCfg.VerifySubjectAltName)))
			})
		})
	})

	Context("ssl parameters", func() {

		BeforeEach(func() {
			configTranslator = NewSslConfigTranslator()
		})

		It("should return nil for nil SslParameters", func() {
			var sslParameters *v1.SslParameters
			tlsParams, err := configTranslator.ResolveSslParamsConfig(sslParameters)

			Expect(err).To(BeNil())
			Expect(tlsParams).To(BeNil())
		})

		It("should return TlsParameters for valid SslParameters", func() {
			sslParameters := &v1.SslParameters{
				MinimumProtocolVersion: v1.SslParameters_TLSv1_1,
				MaximumProtocolVersion: v1.SslParameters_TLSv1_2,
				CipherSuites:           []string{"cipher-test"},
				EcdhCurves:             []string{"ec-dh-test"},
			}
			tlsParams, err := configTranslator.ResolveSslParamsConfig(sslParameters)

			Expect(err).To(BeNil())
			Expect(tlsParams.GetCipherSuites()).To(Equal([]string{"cipher-test"}))
			Expect(tlsParams.GetEcdhCurves()).To(Equal([]string{"ec-dh-test"}))
			Expect(tlsParams.GetTlsMinimumProtocolVersion()).To(Equal(envoyauth.TlsParameters_TLSv1_1))
			Expect(tlsParams.GetTlsMaximumProtocolVersion()).To(Equal(envoyauth.TlsParameters_TLSv1_2))
		})

		It("should error for invalid SslParameters", func() {
			var invalidProtocolVersion v1.SslParameters_ProtocolVersion = 5 // INVALID

			sslParameters := &v1.SslParameters{
				MinimumProtocolVersion: invalidProtocolVersion,
				MaximumProtocolVersion: v1.SslParameters_TLSv1_2,
				CipherSuites:           []string{"cipher-test"},
				EcdhCurves:             []string{"ec-dh-test"},
			}
			tlsParams, err := configTranslator.ResolveSslParamsConfig(sslParameters)

			Expect(err).NotTo(BeNil())
			Expect(tlsParams).To(BeNil())
		})

	})

})

func ValidateCommonContextFiles(tlsCfg *envoyauth.CommonTlsContext, err error) {

	ExpectWithOffset(1, err).NotTo(HaveOccurred())
	validationCtx := tlsCfg.GetValidationContext()
	ExpectWithOffset(1, validationCtx).ToNot(BeNil())
	ExpectWithOffset(1, validationCtx.TrustedCa.GetFilename()).To(Equal("rootca"))

	ExpectWithOffset(1, tlsCfg.GetTlsCertificates()[0].GetCertificateChain().GetFilename()).To(Equal("tlscert"))
	ExpectWithOffset(1, tlsCfg.GetTlsCertificates()[0].GetPrivateKey().GetFilename()).To(Equal("tlskey"))

}

func ValidateCommonContextInline(tlsCfg *envoyauth.CommonTlsContext, err error) {

	ExpectWithOffset(1, err).NotTo(HaveOccurred())
	validationCtx := tlsCfg.GetValidationContext()
	ExpectWithOffset(1, validationCtx).ToNot(BeNil())
	ExpectWithOffset(1, validationCtx.TrustedCa.GetInlineString()).To(Equal("rootca"))

	ExpectWithOffset(1, tlsCfg.GetTlsCertificates()[0].GetCertificateChain().GetInlineString()).To(Equal("tlscert"))
	ExpectWithOffset(1, tlsCfg.GetTlsCertificates()[0].GetPrivateKey().GetInlineString()).To(Equal("tlskey"))

}
