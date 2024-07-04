package printers

import (
	"bytes"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/grpc_json"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/kubernetes"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
)

var _ = Describe("UpstreamTable", func() {
	It("handles malformed upstream (nil spec)", func() {
		Expect(func() {
			us := &v1.Upstream{}
			UpstreamTable(nil, []*v1.Upstream{us}, GinkgoWriter)
		}).NotTo(Panic())
	})

	It("prints grpc upstream function names", func() {
		us := &v1.Upstream{
			Metadata: &core.Metadata{
				Name: "test-us",
			},
			UpstreamType: &v1.Upstream_Kube{
				Kube: &kubernetes.UpstreamSpec{
					ServiceName:      "test",
					ServiceNamespace: "gloo-system",
					ServiceSpec: &options.ServiceSpec{
						PluginType: &options.ServiceSpec_GrpcJsonTranscoder{
							GrpcJsonTranscoder: &grpc_json.GrpcJsonTranscoder{
								DescriptorSet: &grpc_json.GrpcJsonTranscoder_ProtoDescriptorBin{
									ProtoDescriptorBin: []byte{10, 230, 1, 10, 16, 104, 101, 108, 108, 111, 119, 111, 114, 108, 100, 46, 112, 114, 111, 116, 111, 18, 10, 104, 101, 108, 108, 111, 119, 111, 114, 108, 100, 34, 28, 10, 12, 72, 101, 108, 108, 111, 82, 101, 113, 117, 101, 115, 116, 18, 12, 10, 4, 110, 97, 109, 101, 24, 1, 32, 1, 40, 9, 34, 29, 10, 10, 72, 101, 108, 108, 111, 82, 101, 112, 108, 121, 18, 15, 10, 7, 109, 101, 115, 115, 97, 103, 101, 24, 1, 32, 1, 40, 9, 50, 73, 10, 7, 71, 114, 101, 101, 116, 101, 114, 18, 62, 10, 8, 83, 97, 121, 72, 101, 108, 108, 111, 18, 24, 46, 104, 101, 108, 108, 111, 119, 111, 114, 108, 100, 46, 72, 101, 108, 108, 111, 82, 101, 113, 117, 101, 115, 116, 26, 22, 46, 104, 101, 108, 108, 111, 119, 111, 114, 108, 100, 46, 72, 101, 108, 108, 111, 82, 101, 112, 108, 121, 34, 0, 66, 54, 10, 27, 105, 111, 46, 103, 114, 112, 99, 46, 101, 120, 97, 109, 112, 108, 101, 115, 46, 104, 101, 108, 108, 111, 119, 111, 114, 108, 100, 66, 15, 72, 101, 108, 108, 111, 87, 111, 114, 108, 100, 80, 114, 111, 116, 111, 80, 1, 162, 2, 3, 72, 76, 87, 98, 6, 112, 114, 111, 116, 111, 51},
								},
								Services: []string{"helloworld.Greeter"},
							},
						},
					},
				},
			},
		}

		var out bytes.Buffer
		UpstreamTable(nil, []*v1.Upstream{us}, &out)
		// The `SayHello` method exists in the ProtoDescriptorBin. This should be printed when listing upstreams.
		// Since there is only one service, it is safe to assume that this method belongs to it
		Expect(out.String()).To(ContainSubstring("- SayHello"))
	})
})
