package xds_test

import (
	"fmt"

	envoy_config_core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/xds"
	"google.golang.org/protobuf/types/known/structpb"
)

var _ = Describe("Cache", func() {

	It("NodeRoleHasher generates the correct ID", func() {
		nodeRoleHasher := xds.NewNodeRoleHasher()
		node := &envoy_config_core_v3.Node{}
		Expect(nodeRoleHasher.ID(node)).To(Equal(xds.FallbackNodeCacheKey),
			fmt.Sprintf("Should return %s if the role field in the node metadata is not present", xds.FallbackNodeCacheKey))

		role := "role"
		node.Metadata = &structpb.Struct{
			Fields: map[string]*structpb.Value{
				role: structpb.NewStringValue(role),
			},
		}
		Expect(nodeRoleHasher.ID(node)).To(Equal(role), "Should return the role field in the node metadata")
	})

	It("SnapshotCacheKeys returns the keys formatted correctly", func() {
		namespace1, namespace2, name1, name2 := "namespace1", "namespace2", "name1", "name2"
		proxies := []*v1.Proxy{
			v1.NewProxy(namespace1, name1),
			v1.NewProxy(namespace2, name2),
		}
		expectedKeys := []string{fmt.Sprintf("%v~%v", namespace1, name1), fmt.Sprintf("%v~%v", namespace2, name2)}
		actualKeys := xds.SnapshotCacheKeys(proxies)
		Expect(actualKeys).To(BeEquivalentTo(expectedKeys))
	})
})
