package translator_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gateway/pkg/translator"
	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
)

var _ = Describe("Predicate", func() {

	Context("GetPredicate", func() {

		It("returns AllNamespacesPredicate when ReadGatewaysFromAllNamespaces is true", func() {
			p := translator.GetPredicate(defaults.GlooSystem, true)
			_, success := p.(*translator.AllNamespacesPredicate)
			Expect(success).To(BeTrue())
		})

		It("returns SingleNamespacePredicate when ReadGatewaysFromAllNamespaces is false", func() {
			p := translator.GetPredicate(defaults.GlooSystem, false)
			_, success := p.(*translator.SingleNamespacePredicate)
			Expect(success).To(BeTrue())
		})
	})

	Context("SingleNamespacePredicate", func() {

		var predicate translator.Predicate

		BeforeEach(func() {
			predicate = translator.GetPredicate(defaults.GlooSystem, false)
		})

		It("should read Gateway within namespace", func() {
			shouldRead := predicate.ReadGateway(&v1.Gateway{
				Metadata: &core.Metadata{
					Name:      "gw",
					Namespace: defaults.GlooSystem,
				},
			})
			Expect(shouldRead).To(BeTrue())
		})

		It("should not read Gateway outside namespace", func() {
			shouldRead := predicate.ReadGateway(&v1.Gateway{
				Metadata: &core.Metadata{
					Name:      "gw",
					Namespace: "other-namespace",
				},
			})
			Expect(shouldRead).To(BeFalse())
		})

	})

	Context("AllNamespacesPredicate", func() {

		var predicate translator.Predicate

		BeforeEach(func() {
			predicate = translator.GetPredicate(defaults.GlooSystem, true)
		})

		It("should read Gateway within namespace", func() {
			shouldRead := predicate.ReadGateway(&v1.Gateway{
				Metadata: &core.Metadata{
					Name:      "gw",
					Namespace: defaults.GlooSystem,
				},
			})
			Expect(shouldRead).To(BeTrue())
		})

		It("should read Gateway outside namespace", func() {
			shouldRead := predicate.ReadGateway(&v1.Gateway{
				Metadata: &core.Metadata{
					Name:      "gw",
					Namespace: "other-namespace",
				},
			})
			Expect(shouldRead).To(BeTrue())
		})

	})
})
