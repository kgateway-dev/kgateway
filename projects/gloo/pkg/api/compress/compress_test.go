package compress_test

import (
	"encoding/json"

	gloostatusutils "github.com/solo-io/gloo/pkg/utils/statusutils"

	"github.com/solo-io/solo-kit/pkg/utils/protoutils"
	"github.com/solo-io/solo-kit/pkg/utils/statusutils"

	. "github.com/onsi/ginkgo/v2"
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
		statusUnmarshaler = statusutils.NewNamespacedStatusesUnmarshaler(protoutils.UnmarshalMapToProto)
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

		It("should ignore unknown fields on unmarshal spec", func() {
			p := &v1.Proxy{
				Metadata: &core.Metadata{
					Name: "foo",
				},
				Listeners: []*v1.Listener{{BindAddress: "1234"}},
			}

			// take a valid spec
			spec, err := MarshalSpec(p)
			Expect(err).NotTo(HaveOccurred())

			// add an unknown field
			spec["unknownField"] = "unknownFieldValue"

			p2 := &v1.Proxy{}
			err = UnmarshalSpec(p2, spec)
			Expect(err).NotTo(HaveOccurred())

			Expect(p2.Listeners).To(BeEquivalentTo(p.Listeners))
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
			UnmarshalStatus(p2, status, statusUnmarshaler)
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
		It("should truncate the status reason when annotated with max length", func() {
			p := &v1.Proxy{
				Metadata: &core.Metadata{
					Name: "foo",
				},
			}
			SetMaxStatusSizeBytes(p, "4")
			statusClient.SetStatus(p, &core.Status{State: core.Status_Accepted, Reason: "very long message"})
			status, err := MarshalStatus(p)
			Expect(err).NotTo(HaveOccurred())
			unmarshalledProxy := &v1.Proxy{}
			UnmarshalStatus(unmarshalledProxy, status, statusUnmarshaler)
			finalStatus := statusClient.GetStatus(unmarshalledProxy)
			//Truncate the status and append the warning
			Expect(finalStatus.GetReason()).To(Equal("very" + MaxLengthWarningMessage))
		})
		It("should not truncate the status reason when annotated with invalid max length", func() {
			p := &v1.Proxy{
				Metadata: &core.Metadata{
					Name: "foo",
				},
			}
			err := SetMaxStatusSizeBytes(p, "Not an int")
			Expect(err).To(HaveOccurred())
			originalStatus := &core.Status{State: core.Status_Accepted, Reason: "very long message"}
			statusClient.SetStatus(p, originalStatus)
			status, err := MarshalStatus(p)
			Expect(err).NotTo(HaveOccurred())
			unmarshalledProxy := &v1.Proxy{}
			UnmarshalStatus(unmarshalledProxy, status, statusUnmarshaler)
			finalStatus := statusClient.GetStatus(unmarshalledProxy)
			Expect(finalStatus).To(BeEquivalentTo(originalStatus))
		})
		It("should not modify the status reason when message is shorter than the limit", func() {
			p := &v1.Proxy{
				Metadata: &core.Metadata{
					Name: "foo",
				},
			}
			SetMaxStatusSizeBytes(p, "5")
			originalStatus := &core.Status{State: core.Status_Accepted, Reason: "hi"}
			statusClient.SetStatus(p, originalStatus)
			status, err := MarshalStatus(p)
			Expect(err).NotTo(HaveOccurred())
			unmarshalledProxy := &v1.Proxy{}
			UnmarshalStatus(unmarshalledProxy, status, statusUnmarshaler)
			finalStatus := statusClient.GetStatus(unmarshalledProxy)
			Expect(finalStatus).To(BeEquivalentTo(finalStatus))
		})
		It("should truncate status reasons from multiple namespaces", func() {
			p := &v1.Proxy{
				Metadata: &core.Metadata{
					Name: "foo",
				},
			}
			SetMaxStatusSizeBytes(p, "4")
			originalStatus := &core.Status{State: core.Status_Accepted, Reason: "very long message"}
			statusClient.SetStatus(p, originalStatus)
			otherNamespaceStatusClient := gloostatusutils.GetStatusClientForNamespace("ns2")
			otherNamespaceStatusClient.SetStatus(p, originalStatus)

			status, err := MarshalStatus(p)
			Expect(err).NotTo(HaveOccurred())
			unmarshalledProxy := &v1.Proxy{}
			UnmarshalStatus(unmarshalledProxy, status, statusUnmarshaler)
			finalStatus := statusClient.GetStatus(unmarshalledProxy)
			//Truncate the status and append the warning
			Expect(finalStatus.GetReason()).To(Equal("very" + MaxLengthWarningMessage))
			otherNamespaceFinalStatus := otherNamespaceStatusClient.GetStatus(unmarshalledProxy)
			Expect(otherNamespaceFinalStatus.GetReason()).To(Equal("very" + MaxLengthWarningMessage))
		})
	})

})

func size(s interface{}) int {
	r, err := json.Marshal(s)
	Expect(err).NotTo(HaveOccurred())
	return len(r)
}
