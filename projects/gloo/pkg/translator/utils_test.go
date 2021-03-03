package translator_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/projects/gloo/pkg/translator"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
)

var _ = Describe("Plugin", func() {

	It("empty namespace: should convert upstream to cluster name and back properly", func() {
		ref := &core.ResourceRef{Name: "name", Namespace: ""}
		clusterName := translator.UpstreamToClusterName(ref)
		convertedBack, err := translator.ClusterToUpstreamRef(clusterName)
		Expect(err).ToNot(HaveOccurred())
		Expect(convertedBack).To(Equal(ref))
	})

	It("populated namespace: should convert upstream to cluster name and back properly", func() {
		ref := &core.ResourceRef{Name: "name", Namespace: "namespace"}
		clusterName := translator.UpstreamToClusterName(ref)
		convertedBack, err := translator.ClusterToUpstreamRef(clusterName)
		Expect(err).ToNot(HaveOccurred())
		Expect(convertedBack).To(Equal(ref))
	})

})
