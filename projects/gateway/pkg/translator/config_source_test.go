package translator

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
)

var _ = Describe("ConfigSource", func() {

	It("appends sources to the gateway metadata", func() {
		for _, obj := range []ObjectWithMetadata{
			&gloov1.Route{},
			&gloov1.Listener{},
			&gloov1.VirtualHost{},
		} {
			err := appendSource(obj, &v1.VirtualService{
				Metadata: &core.Metadata{Name: "taco", Namespace: "pizza", Generation: 5},
			})
			Expect(err).NotTo(HaveOccurred())
			meta, err := GetSourceMeta(obj)
			Expect(err).NotTo(HaveOccurred())
			Expect(meta.Sources).To(Equal([]SourceRef{{
				ResourceKind:       "*v1.VirtualService",
				ResourceRef:        &core.ResourceRef{Namespace: "pizza", Name: "taco"},
				ObservedGeneration: 5,
			}}))
		}
	})
})
