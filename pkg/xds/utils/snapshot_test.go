package utils_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/projects/gateway2/xds/utils"
	"google.golang.org/protobuf/types/known/structpb"
	corev1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	apiv1 "sigs.k8s.io/gateway-api/apis/v1"
)

var _ = Describe("Snapshot", func() {

	It("should resolve gw key", func() {
		gw := &apiv1.Gateway{
			ObjectMeta: corev1.ObjectMeta{
				Name:      "testname",
				Namespace: "test",
			},
		}

		key := utils.SnapshotCacheKey(gw)

		var nh utils.NodeNameNsHasher

		node := &corev3.Node{
			Metadata: &structpb.Struct{
				Fields: map[string]*structpb.Value{
					"gateway": {
						Kind: &structpb.Value_StructValue{
							StructValue: &structpb.Struct{
								Fields: map[string]*structpb.Value{
									"name": {
										Kind: &structpb.Value_StringValue{
											StringValue: "testname",
										},
									},
									"namespace": {
										Kind: &structpb.Value_StringValue{
											StringValue: "test",
										},
									},
								},
							},
						},
					},
				},
			},
		}

		Expect(nh.ID(node)).To(Equal(key))

	})

})
