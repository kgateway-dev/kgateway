package utils

import (
	envoycore "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoygrpccredential "github.com/envoyproxy/go-control-plane/envoy/config/grpc_credential/v3"
	envoyauth "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
	"github.com/golang/protobuf/ptypes"
	"github.com/golang/protobuf/ptypes/wrappers"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/projects/gloo/constants"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/ssl"
	gloohelpers "github.com/solo-io/gloo/test/helpers"
	. "github.com/solo-io/go-utils/testutils"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	test_matchers "github.com/solo-io/solo-kit/test/matchers"
)

var _ = Describe("Ssl", func() {

	var (
		upstreamCfg            *ssl.UpstreamSslConfig
		downstreamCfg          *ssl.SslConfig
		tlsSecret              *v1.TlsSecret
		secret                 *v1.Secret
		secrets                v1.SecretList
		configTranslator       *sslConfigTranslator
		resolveCommonSslConfig func(cs CertSource, secrets v1.SecretList) (*envoyauth.CommonTlsContext, error)
	)

	resolveCommonSslConfig = func(cs CertSource, secrets v1.SecretList) (*envoyauth.CommonTlsContext, error) {
		return configTranslator.ResolveCommonSslConfig(cs, secrets, false)
	}

	Context("files", func() {
		BeforeEach(func() {
			upstreamCfg = &ssl.UpstreamSslConfig{
				Sni: "test.com",
				SslSecrets: &ssl.UpstreamSslConfig_SslFiles{
					SslFiles: &ssl.SSLFiles{
						TlsCert: gloohelpers.Certificate(),
						TlsKey:  gloohelpers.PrivateKey(),
						RootCa:  gloohelpers.Certificate(),
					},
				},
			}
			downstreamCfg = &ssl.SslConfig{
				SniDomains: []string{"test.com", "test1.com"},
				SslSecrets: &ssl.SslConfig_SslFiles{
					SslFiles: &ssl.SSLFiles{
						TlsCert: gloohelpers.Certificate(),
						TlsKey:  gloohelpers.PrivateKey(),
						RootCa:  gloohelpers.Certificate(),
					},
				},
			}
			configTranslator = NewSslConfigTranslator()

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
				upstreamCfg.SslSecrets.(*ssl.UpstreamSslConfig_SslFiles).SslFiles.RootCa = ""
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

			It("should _not_ error with only a rootca", func() {
				// rootca, tlscert, and tlskey are set in a beforeEach.  Explicitly UNsetting cert+key:
				upstreamCfg.SslSecrets.(*ssl.UpstreamSslConfig_SslFiles).SslFiles.TlsCert = ""
				upstreamCfg.SslSecrets.(*ssl.UpstreamSslConfig_SslFiles).SslFiles.TlsKey = ""
				_, err := resolveCommonSslConfig(upstreamCfg, nil)
				Expect(err).NotTo(HaveOccurred())
			})
		})
	})
	Context("secret", func() {
		BeforeEach(func() {
			tlsSecret = &v1.TlsSecret{
				CertChain:  gloohelpers.Certificate(),
				PrivateKey: gloohelpers.PrivateKey(),
				RootCa:     gloohelpers.Certificate(),
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
			upstreamCfg = &ssl.UpstreamSslConfig{
				Sni: "test.com",
				SslSecrets: &ssl.UpstreamSslConfig_SecretRef{
					SecretRef: ref,
				},
			}
			downstreamCfg = &ssl.SslConfig{
				SniDomains: []string{"test.com", "test1.com"},
				SslSecrets: &ssl.SslConfig_SecretRef{
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
		DescribeTable("should fail if invalid private key is provided",
			func(c func() CertSource) {
				tlsSecret.PrivateKey = "bad_private_key"
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

		It("should disable tls session if disableStatelessTlsSessionResumption is true", func() {
			downstreamCfg.DisableTlsSessionResumption = &wrappers.BoolValue{Value: true}
			cfg, err := configTranslator.ResolveDownstreamSslConfig(secrets, downstreamCfg)
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.GetDisableStatelessSessionResumption()).To(BeTrue())
		})

		It("should set require client cert for downstream config", func() {
			cfg, err := configTranslator.ResolveDownstreamSslConfig(secrets, downstreamCfg)
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.RequireClientCertificate.GetValue()).To(BeTrue())
		})

		It("should set require client cert to false if oneWayTls enabled for downstream config", func() {
			downstreamCfg.OneWayTls = &wrappers.BoolValue{Value: true}
			cfg, err := configTranslator.ResolveDownstreamSslConfig(secrets, downstreamCfg)
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.RequireClientCertificate.GetValue()).To(BeFalse())
		})

		It("should set alpn default for downstream config", func() {
			cfg, err := configTranslator.ResolveDownstreamSslConfig(secrets, downstreamCfg)
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.CommonTlsContext.AlpnProtocols).To(Equal([]string{"h2", "http/1.1"}))
		})
		It("should set alpn empty for downstream config", func() {
			downstreamCfg.AlpnProtocols = []string{constants.AllowEmpty}
			cfg, err := configTranslator.ResolveDownstreamSslConfig(secrets, downstreamCfg)
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.CommonTlsContext.AlpnProtocols).To(Equal([]string{}))
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

		It("should not set allow negotiation by default for upstream config", func() {
			cfg, err := configTranslator.ResolveUpstreamSslConfig(secrets, upstreamCfg)
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.AllowRenegotiation).To(BeFalse())
		})

		It("should set allow negotiation if configured for upstream config", func() {
			upstreamCfg.AllowRenegotiation = &wrappers.BoolValue{Value: true}
			cfg, err := configTranslator.ResolveUpstreamSslConfig(secrets, upstreamCfg)
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.AllowRenegotiation).To(BeTrue())
		})

		Context("ocsp", func() {
			Context("staple policy", func() {
				It("should default to LENIENT_STAPLING for ocsp staple policy", func() {
					cfg, err := configTranslator.ResolveDownstreamSslConfig(secrets, downstreamCfg)
					Expect(err).NotTo(HaveOccurred())
					Expect(cfg.OcspStaplePolicy).To(Equal(envoyauth.DownstreamTlsContext_LENIENT_STAPLING))
				})

				It("should default to LENIENT_STAPLING if staple policy does not exist", func() {
					downstreamCfg.OcspStaplePolicy = ssl.SslConfig_OcspStaplePolicy(ssl.SslConfig_OcspStaplePolicy_value["INVALID"])
					cfg, err := configTranslator.ResolveDownstreamSslConfig(secrets, downstreamCfg)
					Expect(err).NotTo(HaveOccurred())
					Expect(cfg.OcspStaplePolicy).To(Equal(envoyauth.DownstreamTlsContext_LENIENT_STAPLING))
				})

				DescribeTable("set ocsp staple policy when provided", func(policy ssl.SslConfig_OcspStaplePolicy, expected envoyauth.DownstreamTlsContext_OcspStaplePolicy) {
					downstreamCfg.OcspStaplePolicy = policy
					cfg, err := configTranslator.ResolveDownstreamSslConfig(secrets, downstreamCfg)
					Expect(err).NotTo(HaveOccurred())
					Expect(cfg.OcspStaplePolicy).To(Equal(expected))
				},
					Entry("MUST_STAPLE", ssl.SslConfig_MUST_STAPLE, envoyauth.DownstreamTlsContext_MUST_STAPLE),
					Entry("LENIENT_STAPLING", ssl.SslConfig_LENIENT_STAPLING, envoyauth.DownstreamTlsContext_LENIENT_STAPLING),
					Entry("STRICT_STAPLING", ssl.SslConfig_STRICT_STAPLING, envoyauth.DownstreamTlsContext_STRICT_STAPLING),
				)
			})

			// Not testing that the staple is set, as it is a generated `der` file from an OCSP server, which should be tested in an E2E test.
			Context("staple response", func() {
				It("should not set ocsp staple if none is provided", func() {
					cfg, err := configTranslator.ResolveCommonSslConfig(downstreamCfg, secrets, true)
					Expect(err).NotTo(HaveOccurred())
					Expect(len(cfg.TlsCertificates)).To(Equal(1))
					Expect(cfg.TlsCertificates[0].OcspStaple).To(BeNil())
				})
			})
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
				upstreamCfg.Parameters = &ssl.SslParameters{
					MinimumProtocolVersion: ssl.SslParameters_TLSv1_1,
					MaximumProtocolVersion: ssl.SslParameters_TLSv1_2,
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
			sdsConfig *ssl.SDSConfig
		)
		BeforeEach(func() {
			sdsConfig = &ssl.SDSConfig{
				CertificatesSecretName: "CertificatesSecretName",
				ValidationContextName:  "ValidationContextName",
			}
			upstreamCfg = &ssl.UpstreamSslConfig{
				Sni: "test.com",
				SslSecrets: &ssl.UpstreamSslConfig_Sds{
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
			// If they are not equivalent, it means that any serialization is different.
			// see here: https://github.com/envoyproxy/go-control-plane/pull/158
			// and here: https://github.com/envoyproxy/envoy/pull/6241
			// this may lead to envoy updates being too frequent
			Expect(vctx.SdsConfig).To(BeEquivalentTo(cert.SdsConfig))

			envoyGrpc := vctx.SdsConfig.ConfigSourceSpecifier.(*envoycore.ConfigSource_ApiConfigSource).ApiConfigSource.GrpcServices[0].TargetSpecifier.(*envoycore.GrpcService_EnvoyGrpc_).EnvoyGrpc
			Expect(envoyGrpc.ClusterName).To(Equal("gateway_proxy_sds"))

		})

		It("should have a sds setup with a custom cluster name", func() {
			cfgCustomCluster := &ssl.UpstreamSslConfig{
				Sni: "test.com",
				SslSecrets: &ssl.UpstreamSslConfig_Sds{
					Sds: &ssl.SDSConfig{
						CertificatesSecretName: "CertificatesSecretName",
						ValidationContextName:  "ValidationContextName",
						SdsBuilder: &ssl.SDSConfig_ClusterName{
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
			// If they are not equivalent, it means that any serialization is different.
			// see here: https://github.com/envoyproxy/go-control-plane/pull/158
			// and here: https://github.com/envoyproxy/envoy/pull/6241
			// this may lead to envoy updates being too frequent
			Expect(vctx.SdsConfig).To(BeEquivalentTo(cert.SdsConfig))

			envoyGrpc := vctx.SdsConfig.ConfigSourceSpecifier.(*envoycore.ConfigSource_ApiConfigSource).ApiConfigSource.GrpcServices[0].TargetSpecifier.(*envoycore.GrpcService_EnvoyGrpc_).EnvoyGrpc
			Expect(envoyGrpc.ClusterName).To(Equal("custom-cluster"))

		})

		Context("TargetUri is specified", func() {
			BeforeEach(func() {
				sdsConfig.TargetUri = "targetUri"
			})

			When("only TargetUri is specified", func() {

				It("should have a sds setup with a GoogleGrpc TargetSpecifier with the expected TargetUri", func() {
					c, err := resolveCommonSslConfig(upstreamCfg, nil)
					Expect(err).NotTo(HaveOccurred())
					Expect(c.TlsCertificateSdsSecretConfigs).To(HaveLen(1))
					Expect(c.ValidationContextType).ToNot(BeNil())

					vctx := c.ValidationContextType.(*envoyauth.CommonTlsContext_ValidationContextSdsSecretConfig).ValidationContextSdsSecretConfig
					cert := c.TlsCertificateSdsSecretConfigs[0]
					Expect(vctx.Name).To(Equal("ValidationContextName"))
					Expect(cert.Name).To(Equal("CertificatesSecretName"))

					vctxGoogleGrpc := vctx.SdsConfig.ConfigSourceSpecifier.(*envoycore.ConfigSource_ApiConfigSource).ApiConfigSource.GrpcServices[0].TargetSpecifier.(*envoycore.GrpcService_GoogleGrpc_).GoogleGrpc
					Expect(vctxGoogleGrpc.TargetUri).To(Equal("targetUri"))
					Expect(vctxGoogleGrpc.StatPrefix).To(Equal("ValidationContextName"))

					// vctx and cert are expected to have different StatPrefixes on their GoogleGrpc TargetSpecifiers
					// Modify vctxGoogleGrpc.StatPrefix, which has already been verified, to match that which we expect
					// for cert
					vctxGoogleGrpc.StatPrefix = "CertificatesSecretName"
					// If they are not equivalent, it means that any serialization is different.
					// see here: https://github.com/envoyproxy/go-control-plane/pull/158
					// and here: https://github.com/envoyproxy/envoy/pull/6241
					// this may lead to envoy updates being too frequent
					Expect(vctx.SdsConfig).To(BeEquivalentTo(cert.SdsConfig))
				})
			})

			When("TargetUri and ClusterName are specified", func() {
				BeforeEach(func() {
					sdsConfig.SdsBuilder = &ssl.SDSConfig_ClusterName{
						ClusterName: "custom-cluster",
					}
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
					// If they are not equivalent, it means that any serialization is different.
					// see here: https://github.com/envoyproxy/go-control-plane/pull/158
					// and here: https://github.com/envoyproxy/envoy/pull/6241
					// this may lead to envoy updates being too frequent
					Expect(vctx.SdsConfig).To(BeEquivalentTo(cert.SdsConfig))

					envoyGrpc := vctx.SdsConfig.ConfigSourceSpecifier.(*envoycore.ConfigSource_ApiConfigSource).ApiConfigSource.GrpcServices[0].TargetSpecifier.(*envoycore.GrpcService_EnvoyGrpc_).EnvoyGrpc
					Expect(envoyGrpc.ClusterName).To(Equal("custom-cluster"))
				})
			})
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
			sdsConfig *ssl.SDSConfig
		)
		BeforeEach(func() {
			sdsConfig = &ssl.SDSConfig{
				CertificatesSecretName: "CertificatesSecretName",
				ValidationContextName:  "ValidationContextName",
				SdsBuilder: &ssl.SDSConfig_CallCredentials{
					CallCredentials: &ssl.CallCredentials{
						FileCredentialSource: &ssl.CallCredentials_FileCredentialSource{
							TokenFileName: "TokenFileName",
							Header:        "Header",
						},
					},
				},
			}
			upstreamCfg = &ssl.UpstreamSslConfig{
				Sni: "test.com",
				SslSecrets: &ssl.UpstreamSslConfig_Sds{
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
			// If they are not equivalent, it means that any serialization is different.
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
			var sslParameters *ssl.SslParameters
			tlsParams, err := configTranslator.ResolveSslParamsConfig(sslParameters)

			Expect(err).To(BeNil())
			Expect(tlsParams).To(BeNil())
		})

		It("should return TlsParameters for valid SslParameters", func() {
			sslParameters := &ssl.SslParameters{
				MinimumProtocolVersion: ssl.SslParameters_TLSv1_1,
				MaximumProtocolVersion: ssl.SslParameters_TLSv1_2,
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
			var invalidProtocolVersion ssl.SslParameters_ProtocolVersion = 5 // INVALID

			sslParameters := &ssl.SslParameters{
				MinimumProtocolVersion: invalidProtocolVersion,
				MaximumProtocolVersion: ssl.SslParameters_TLSv1_2,
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
	ExpectWithOffset(1, validationCtx.TrustedCa.GetFilename()).To(Equal(gloohelpers.Certificate()))

	ExpectWithOffset(1, tlsCfg.GetTlsCertificates()[0].GetCertificateChain().GetFilename()).To(Equal(gloohelpers.Certificate()))
	ExpectWithOffset(1, tlsCfg.GetTlsCertificates()[0].GetPrivateKey().GetFilename()).To(Equal(gloohelpers.PrivateKey()))

}

func ValidateCommonContextInline(tlsCfg *envoyauth.CommonTlsContext, err error) {

	ExpectWithOffset(1, err).NotTo(HaveOccurred())
	validationCtx := tlsCfg.GetValidationContext()
	ExpectWithOffset(1, validationCtx).ToNot(BeNil())
	ExpectWithOffset(1, validationCtx.TrustedCa.GetInlineString()).To(Equal(gloohelpers.Certificate()))

	ExpectWithOffset(1, tlsCfg.GetTlsCertificates()[0].GetCertificateChain().GetInlineString()).To(Equal(gloohelpers.Certificate()))
	ExpectWithOffset(1, tlsCfg.GetTlsCertificates()[0].GetPrivateKey().GetInlineString()).To(Equal(gloohelpers.PrivateKey()))

}
