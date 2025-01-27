package helpers_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/test/helpers"
)

var _ = Describe("UpstreamBuilder", func() {
	When("using the base builder", func() {
		It("generates an upstream without SslConfig", func() {
			up := helpers.NewUpstreamBuilder().Build(1)
			Expect(up.SslConfig).To(BeNil())
		})
	})

	When("with consistent SNI", func() {
		It("generates an upstream with the same SNI for any i", func() {
			up1 := helpers.NewUpstreamBuilder().WithConsistentSni().Build(1)
			Expect(up1.SslConfig).NotTo(BeNil())
			up2 := helpers.NewUpstreamBuilder().WithConsistentSni().Build(2)
			Expect(up2.SslConfig).NotTo(BeNil())

			Expect(up1.SslConfig.Sni).To(Equal(up2.SslConfig.Sni))
		})
	})

	When("with unique SNI", func() {
		It("generates an upstream with unique SNI for a given i", func() {
			up1 := helpers.NewUpstreamBuilder().WithUniqueSni().Build(1)
			Expect(up1.SslConfig).NotTo(BeNil())
			up2 := helpers.NewUpstreamBuilder().WithUniqueSni().Build(2)
			Expect(up2.SslConfig).NotTo(BeNil())

			Expect(up1.SslConfig.Sni).NotTo(Equal(up2.SslConfig.Sni))
		})
	})

})
