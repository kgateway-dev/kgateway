package utils_test

import (
	envoyauth "github.com/envoyproxy/go-control-plane/envoy/api/v2/auth"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	core "github.com/solo-io/solo-kit/pkg/api/v1/resources/core"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/ginkgo/extensions/table"
	. "github.com/onsi/gomega"

	. "github.com/solo-io/gloo/projects/gloo/pkg/utils"
)

var _ = Describe("Ssl", func() {

	var (
		upstreamCfg      *v1.UpstreamSslConfig
		downstreamCfg    *v1.SslConfig
		tlsSecret        *v1.TlsSecret
		secret           *v1.Secret
		secrets          v1.SecretList
		configTranslator *SslConfigTranslator
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
			configTranslator = NewSslConfigTranslator(nil)
		})

		DescribeTable("should resolve from files",
			func(c func() CertSource) {
				ValidateCommonContextFiles(configTranslator.ResolveCommonSslConfig(c()))
			},
			Entry("upstreamCfg", func() CertSource { return upstreamCfg }),
			Entry("downstreamCfg", func() CertSource { return downstreamCfg }),
		)

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
				Metadata: core.Metadata{
					Name:      "secret",
					Namespace: "secret",
				},
			}
			ref := secret.Metadata.Ref()
			secrets = v1.SecretList{secret}
			upstreamCfg = &v1.UpstreamSslConfig{
				Sni: "test.com",
				SslSecrets: &v1.UpstreamSslConfig_SecretRef{
					SecretRef: &ref,
				},
			}
			downstreamCfg = &v1.SslConfig{
				SniDomains: []string{"test.com", "test1.com"},
				SslSecrets: &v1.SslConfig_SecretRef{
					SecretRef: &ref,
				},
			}
			configTranslator = NewSslConfigTranslator(secrets)
		})

		It("should error with no secret", func() {
			configTranslator = NewSslConfigTranslator(nil)
			_, err := configTranslator.ResolveCommonSslConfig(upstreamCfg)
			Expect(err).To(HaveOccurred())

		})

		It("should error with wrong secret", func() {
			secret.Kind = &v1.Secret_Aws{}
			_, err := configTranslator.ResolveCommonSslConfig(upstreamCfg)
			Expect(err).To(HaveOccurred())

		})

		DescribeTable("should resolve from secret refs",
			func(c func() CertSource) {
				ValidateCommonContextInline(configTranslator.ResolveCommonSslConfig(c()))
			},
			Entry("upstreamCfg", func() CertSource { return upstreamCfg }),
			Entry("downstreamCfg", func() CertSource { return downstreamCfg }),
		)
		DescribeTable("should fail if only cert is not provided",
			func(c func() CertSource) {
				tlsSecret.CertChain = ""
				_, err := configTranslator.ResolveCommonSslConfig(c())
				Expect(err).To(HaveOccurred())

			},
			Entry("upstreamCfg", func() CertSource { return upstreamCfg }),
			Entry("downstreamCfg", func() CertSource { return downstreamCfg }),
		)
		DescribeTable("should fail if only private key is not provided",
			func(c func() CertSource) {
				tlsSecret.PrivateKey = ""
				_, err := configTranslator.ResolveCommonSslConfig(c())
				Expect(err).To(HaveOccurred())

			},
			Entry("upstreamCfg", func() CertSource { return upstreamCfg }),
			Entry("downstreamCfg", func() CertSource { return downstreamCfg }),
		)
		DescribeTable("should not have validation context if no rootca",
			func(c func() CertSource) {
				tlsSecret.RootCa = ""
				cfg, err := configTranslator.ResolveCommonSslConfig(c())
				Expect(err).NotTo(HaveOccurred())
				Expect(cfg.ValidationContextType).To(BeNil())

			},
			Entry("upstreamCfg", func() CertSource { return upstreamCfg }),
			Entry("downstreamCfg", func() CertSource { return downstreamCfg }),
		)

		It("should set require client cert for downstream config", func() {
			cfg, err := configTranslator.ResolveDownstreamSslConfig(downstreamCfg)
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.RequireClientCertificate.GetValue()).To(BeTrue())
		})

		It("should not set require client cert for downstream config with no rootca", func() {
			tlsSecret.RootCa = ""
			cfg, err := configTranslator.ResolveDownstreamSslConfig(downstreamCfg)
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.RequireClientCertificate.GetValue()).To(BeFalse())
		})

		It("should set sni for upstream config", func() {
			cfg, err := configTranslator.ResolveUpstreamSslConfig(upstreamCfg)
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.Sni).To(Equal("test.com"))
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
