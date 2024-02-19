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

	DescribeTable("ClassicEdgeNodeHash",
		func(nodeMetadata *structpb.Struct, expectedHash types.GomegaMatcher) {
			nodeHash := xds.NewClassicEdgeNodeHash()

			node := &envoy_config_core_v3.Node{
				Metadata: nodeMetadata,
			}
			Expect(nodeHash.ID(node)).To(expectedHash,
				"ClassicEdgeNodeHash should produce the expected string identifier for the Envoy node.")
		},
		Entry(&structpb.Struct{}, Equal(xds.FallbackNodeCacheKey)),
		Entry(&structpb.Struct{
			Fields: map[string]*structpb.Value{
				"non-role-field": structpb.NewStringValue("non-role-value"),
			},
		}, Equal(xds.FallbackNodeCacheKey)),
		Entry(&structpb.Struct{
			Fields: map[string]*structpb.Value{
				"role": structpb.NewStringValue("role-value"),
			},
		}, Equal("role-value")),
	)

})
