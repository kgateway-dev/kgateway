package utils_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/projects/gateway2/translator/utils"
	envoycache "github.com/solo-io/solo-kit/pkg/api/v1/control-plane/cache"
	"github.com/solo-io/solo-kit/pkg/api/v1/control-plane/resource"

	clusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
)

var _ = Describe("Utils", func() {

	It("should give the same hash for the same resources", func() {
		cluster1 := &clusterv3.Cluster{Name: "cluster1"}
		cluster2 := &clusterv3.Cluster{Name: "cluster2"}
		resources1 := []envoycache.Resource{resource.NewEnvoyResource(cluster2), resource.NewEnvoyResource(cluster1)}
		resources2 := []envoycache.Resource{resource.NewEnvoyResource(cluster1), resource.NewEnvoyResource(cluster2)}
		h1, err1 := utils.EnvoyCacheResourcesListSetToFnvHash(resources1)
		h2, err2 := utils.EnvoyCacheResourcesListSetToFnvHash(resources2)
		Expect(err1).NotTo(HaveOccurred())
		Expect(err2).NotTo(HaveOccurred())
		Expect(h1).ToNot(Equal(0))
		Expect(h1).To(Equal(h2))
	})
})
