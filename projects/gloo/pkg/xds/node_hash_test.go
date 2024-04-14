package xds_test

import (
	"github.com/onsi/gomega/types"

	envoy_config_core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/projects/gloo/pkg/xds"
	"google.golang.org/protobuf/types/known/structpb"
)

var _ = Describe("NodeHash", func() {

	DescribeTable("NodeRoleHasher",
		func(nodeMetadata *structpb.Struct, expectedHash types.GomegaMatcher) {
			nodeHash := xds.NewNodeRoleHasher()

			node := &envoy_config_core_v3.Node{
				Metadata: nodeMetadata,
			}
			Expect(nodeHash.ID(node)).To(expectedHash,
				"NodeRoleHasher should produce the expected string identifier for the Envoy node.")
		},
		Entry("empty metadata", &structpb.Struct{}, Equal(xds.FallbackNodeCacheKey)),
		Entry("metadata without role", &structpb.Struct{
			Fields: map[string]*structpb.Value{
				"non-role-field": structpb.NewStringValue("non-role-value"),
			},
		}, Equal(xds.FallbackNodeCacheKey)),
		Entry("metadata with proxy workload", &structpb.Struct{
			Fields: map[string]*structpb.Value{
				"role": structpb.NewStringValue("proxy-namespace~proxy-name"),
			},
		}, Equal("proxy-namespace~proxy-name")),
		Entry("metadata with owner and proxy workload", &structpb.Struct{
			Fields: map[string]*structpb.Value{
				"role": structpb.NewStringValue("gloo-gateway~proxy-namespace~proxy-name"),
			},
		}, Equal("gloo-gateway~proxy-namespace~proxy-name")),
		Entry("metadata with non-proxy workload", &structpb.Struct{
			Fields: map[string]*structpb.Value{
				"role": structpb.NewStringValue("no-tilde-in-role"),
			},
		}, Equal("no-tilde-in-role")),
	)

})
