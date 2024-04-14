package xds_test

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"
	"github.com/solo-io/gloo/projects/gloo/pkg/utils"
	"github.com/solo-io/gloo/projects/gloo/pkg/xds"
)

var _ = Describe("Cache", func() {

	It("SnapshotCacheKeys returns the keys formatted correctly", func() {
		owner, namespace1, namespace2, name1, name2 := "owner", "namespace1", "namespace2", "name1", "name2"
		p1 := v1.NewProxy(namespace1, name1)
		p1.Metadata.Labels = map[string]string{utils.ProxyTypeKey: owner}
		p2 := v1.NewProxy(namespace2, name2)
		p2.Metadata.Labels = map[string]string{utils.ProxyTypeKey: owner}
		proxies := []*v1.Proxy{p1, p2}
		// legacy proxies are formatted as namespace~name
		expectedKeys := []string{fmt.Sprintf("%v~%v", namespace1, name1), fmt.Sprintf("%v~%v", namespace2, name2)}
		actualKeys := xds.SnapshotCacheKeys(proxies)
		Expect(actualKeys).To(BeEquivalentTo(expectedKeys))
	})

	It("SnapshotCacheKeys returns the keys formatted correctly", func() {
		namespace1, namespace2, name1, name2 := "namespace1", "namespace2", "name1", "name2"
		p1 := v1.NewProxy(namespace1, name1)
		p2 := v1.NewProxy(namespace2, name2)
		proxies := []*v1.Proxy{p1, p2}
		// missing owner is correctly formatted with legacy format: namespace~name
		expectedKeys := []string{fmt.Sprintf("%v~%v", namespace1, name1), fmt.Sprintf("%v~%v", namespace2, name2)}
		actualKeys := xds.SnapshotCacheKeys(proxies)
		Expect(actualKeys).To(BeEquivalentTo(expectedKeys))
	})

	It("Gloo Gateway SnapshotCacheKeys uses owner label", func() {
		owner, namespace1, namespace2, name1, name2 := utils.GatewayApiProxyValue, "namespace1", "namespace2", "name1", "name2"
		// default gloo-system namespace is used for namespace
		p1 := v1.NewProxy(namespace1, name1)
		p1.Metadata.Labels = map[string]string{
			utils.ProxyTypeKey: owner,
		}
		p2 := v1.NewProxy(namespace2, name2)
		p2.Metadata.Labels = map[string]string{
			utils.ProxyTypeKey: owner,
		}
		proxies := []*v1.Proxy{p1, p2}
		expectedKeys := []string{fmt.Sprintf("%v~%v~%v", owner, namespace1, name1), fmt.Sprintf("%v~%v~%v", owner, namespace2, name2)}
		actualKeys := xds.SnapshotCacheKeys(proxies)
		Expect(actualKeys).To(BeEquivalentTo(expectedKeys))
	})

	It("Gloo Gateway SnapshotCacheKeys use namespace label", func() {
		namespace1, namespace2, name1, name2 := "namespace1", "namespace2", "name1", "name2"
		p1 := v1.NewProxy(defaults.GlooSystem, name1)
		// proxy metadata is different
		p1.Metadata.Labels = map[string]string{
			utils.ProxyTypeKey:        utils.GatewayApiProxyValue,
			utils.GatewayNamespaceKey: namespace1,
		}
		p2 := v1.NewProxy(defaults.GlooSystem, name2)
		p2.Metadata.Labels = map[string]string{
			utils.ProxyTypeKey:        utils.GatewayApiProxyValue,
			utils.GatewayNamespaceKey: namespace2,
		}
		proxies := []*v1.Proxy{p1, p2}
		expectedKeys := []string{fmt.Sprintf("%v~%v~%v", utils.GatewayApiProxyValue, namespace1, name1), fmt.Sprintf("%v~%v~%v", utils.GatewayApiProxyValue, namespace2, name2)}
		actualKeys := xds.SnapshotCacheKeys(proxies)
		Expect(actualKeys).To(BeEquivalentTo(expectedKeys))
	})
})
