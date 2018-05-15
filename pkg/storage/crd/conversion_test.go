package crd_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/solo-io/gloo/pkg/api/types/v1"
	"github.com/solo-io/gloo/pkg/protoutil"
	. "github.com/solo-io/gloo/pkg/storage/crd"
	crdv1 "github.com/solo-io/gloo/pkg/storage/crd/solo.io/v1"
	"github.com/solo-io/gloo/test/helpers"
)

var _ = Describe("Conversion", func() {
	Describe("UpstreamToCrd", func() {
		It("Converts a gloo upstream to crd", func() {
			us := helpers.NewTestUpstream1()
			annotations := map[string]string{"foo": "bar"}
			us.Metadata = &v1.Metadata{
				Annotations: annotations,
			}
			upCrd, err := ConfigObjectToCrd("foo", us)
			Expect(err).NotTo(HaveOccurred())
			Expect(upCrd.GetName()).To(Equal(us.Name))
			Expect(upCrd.GetNamespace()).To(Equal("foo"))
			Expect(upCrd.GetAnnotations()).To(Equal(annotations))
			spec := *upCrd.(*crdv1.Upstream).Spec
			// removed parts
			Expect(spec["name"]).To(BeNil())
			Expect(spec["metadata"]).To(BeNil())
			Expect(spec["annotations"]).To(BeNil())

			// shifted parts
			Expect(spec["type"]).To(Equal(us.Type))
			m, err := protoutil.MarshalMap(us.Spec)
			Expect(err).To(BeNil())
			Expect(spec["spec"]).To(Equal(m))
			var fnsInSpec []interface{}
			for _, fn := range us.Functions {
				fnSpec, err := protoutil.MarshalMap(fn.Spec)
				Expect(err).To(BeNil())
				fnsInSpec = append(fnsInSpec, map[string]interface{}{
					"name": fn.Name,
					"spec": fnSpec,
				})
			}
			Expect(spec["functions"]).To(Equal(fnsInSpec))
		})
	})
	Describe("VirtualServiceToCrd", func() {
		It("Converts a gloo virtualservice to crd", func() {
			vService := helpers.NewTestVirtualService("foo", helpers.NewTestRoute1())
			annotations := map[string]string{"foo": "bar"}
			vService.Metadata = &v1.Metadata{
				Annotations: annotations,
			}
			vsCrd, err := ConfigObjectToCrd("foo", vService)
			Expect(err).NotTo(HaveOccurred())
			Expect(vsCrd.GetName()).To(Equal(vService.Name))
			Expect(vsCrd.GetNamespace()).To(Equal("foo"))
			Expect(vsCrd.GetAnnotations()).To(Equal(annotations))
			spec := *vsCrd.(*crdv1.VirtualService).Spec
			// removed parts
			Expect(spec["name"]).To(BeNil())
			Expect(spec["metadata"]).To(BeNil())
			Expect(spec["annotations"]).To(BeNil())

			//// shifted parts
			vServiceMap, err := protoutil.MarshalMap(vService)
			Expect(err).To(BeNil())
			Expect(spec["routes"]).To(Equal(vServiceMap["routes"]))
		})
	})
})
