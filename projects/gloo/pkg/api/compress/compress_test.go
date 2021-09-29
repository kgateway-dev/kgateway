package compress_test

import (
	"encoding/json"

	gloostatusutils "github.com/solo-io/gloo/pkg/utils/statusutils"

	"github.com/solo-io/solo-kit/pkg/utils/protoutils"
	"github.com/solo-io/solo-kit/pkg/utils/statusutils"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	. "github.com/solo-io/gloo/projects/gloo/pkg/api/compress"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
)

var _ = Describe("Compress", func() {

	var (
		statusClient      resources.StatusClient
		statusUnmarshaler *statusutils.NamespacedStatusesUnmarshaler
	)

	BeforeEach(func() {
		ns := gloostatusutils.GetStatusReporterNamespaceOrDefault("default")
		statusUnmarshaler = statusutils.NewNamespacedStatusesUnmarshaler(ns, protoutils.UnmarshalMapToProto)
		statusClient = gloostatusutils.GetStatusClientForNamespace(ns)
	})

	Context("spec", func() {
		It("should  not compress spec when not annotated", func() {
			p := &v1.Proxy{
				Metadata: &core.Metadata{
					Name: "foo",
				},
				Listeners: []*v1.Listener{{BindAddress: "1234"}},
			}

			spec, err := MarshalSpec(p)
			Expect(err).NotTo(HaveOccurred())
			Expect(spec).NotTo(HaveKey("compressedSpec"))
		})
		It("should compress spec when annotated", func() {
			p := &v1.Proxy{
				Metadata: &core.Metadata{
					Name:        "foo",
					Annotations: map[string]string{"gloo.solo.io/compress": "true"},
				},
				Listeners: []*v1.Listener{{BindAddress: "1234"}},
			}

			spec, err := MarshalSpec(p)
			Expect(err).NotTo(HaveOccurred())
			Expect(spec).To(HaveKey("compressedSpec"))
		})

		It("should uncompress to the same thing", func() {
			p := &v1.Proxy{
				Metadata: &core.Metadata{
					Name:        "foo",
					Annotations: map[string]string{"gloo.solo.io/compress": "true"},
				},
				Listeners: []*v1.Listener{{BindAddress: "1234"}},
			}

			spec, err := MarshalSpec(p)
			Expect(err).NotTo(HaveOccurred())

			p2 := &v1.Proxy{}
			err = UnmarshalSpec(p2, spec)
			Expect(err).NotTo(HaveOccurred())

			Expect(p2.Listeners).To(BeEquivalentTo(p.Listeners))
		})
		It("should compress to a smaller size", func() {
			var l []*v1.Listener
			for i := 0; i < 100; i++ {
				l = append(l, &v1.Listener{BindAddress: "1234"})
			}
			p := &v1.Proxy{
				Metadata: &core.Metadata{
					Name: "foo",
				},
				Listeners: l,
			}
			uncompressedSpec, err := MarshalSpec(p)
			Expect(err).NotTo(HaveOccurred())
			p.Metadata.Annotations = map[string]string{"gloo.solo.io/compress": "true"}
			compressedSpec, err := MarshalSpec(p)
			Expect(err).NotTo(HaveOccurred())
			Expect(uncompressedSpec).NotTo(HaveKey("spec"))
			Expect(compressedSpec).To(HaveKey("compressedSpec"))

			// make sure it gets compressed by 90%
			Expect(size(compressedSpec)).To(BeNumerically("<", size(uncompressedSpec)/10))
		})
	})

	Context("status", func() {

		It("should not unmarshall to the same thing status even when annotated", func() {
			p := &v1.Proxy{
				Metadata: &core.Metadata{
					Name:        "foo",
					Annotations: map[string]string{"gloo.solo.io/compress": "true"},
				},
			}
			statusClient.SetStatus(p, &core.Status{State: core.Status_Accepted})

			status, err := MarshalStatus(p)
			Expect(err).NotTo(HaveOccurred())

			p2 := &v1.Proxy{}
			err = UnmarshalStatus(p2, status, statusUnmarshaler)
			Expect(err).NotTo(HaveOccurred())
			Expect(p.GetNamespacedStatuses()).To(BeEquivalentTo(p2.GetNamespacedStatuses()))
		})

		It("should not compress status even when annotated", func() {
			p := &v1.Proxy{
				Metadata: &core.Metadata{
					Name: "foo",
				},
			}
			statusClient.SetStatus(p, &core.Status{State: core.Status_Accepted})

			status1, err := MarshalStatus(p)
			Expect(err).NotTo(HaveOccurred())
			p.Metadata.Annotations = map[string]string{"gloo.solo.io/compress": "true"}

			status2, err := MarshalStatus(p)
			Expect(err).NotTo(HaveOccurred())

			Expect(status1).To(BeEquivalentTo(status2))
		})
	})

})

func size(s interface{}) int {
	r, err := json.Marshal(s)
	Expect(err).NotTo(HaveOccurred())
	return len(r)
}
