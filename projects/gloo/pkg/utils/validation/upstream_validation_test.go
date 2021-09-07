package validation_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	. "github.com/solo-io/gloo/projects/gloo/pkg/api/grpc/validation"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/utils/validation"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
)

var _ = Describe("Upstream validation utils", func() {

	Context("SSL config", func() {

		It("Should detect missing secret ref", func() {
			snap := &v1.ApiSnapshot{}
			us := &v1.Upstream{
				SslConfig: &v1.UpstreamSslConfig{
					SslSecrets: &v1.UpstreamSslConfig_SecretRef{
						SecretRef: &core.ResourceRef{
							Namespace: "non-existent",
							Name:      "also-non-existent",
						},
					},
				},
			}
			usReport := validation.ValidateUpstream(snap, us)

			Expect(usReport.GetErrors()).NotTo(BeNil())
			Expect(usReport.GetErrors()).To(HaveLen(1))
			Expect(usReport.GetErrors()[0].GetType()).To(Equal(UpstreamReport_Error_SSL_CONFIG_ERROR))
			Expect(usReport.GetErrors()[0].GetReason()).To(Equal("Secret does not exist"))
		})

	})

})
