package validation

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	v1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	"github.com/solo-io/gloo/test/samples"
)

var _ = Describe("virtualServicesForRouteTable", func() {
	It("retrieves all virtual services containing the given route table", func() {
		vs, routeTables := samples.LinkedRouteTablesWithVirtualService("vs1", "a")
		vs2, routeTables2 := samples.LinkedRouteTablesWithVirtualService("v2", "b")
		vss := v1.VirtualServiceList{vs, vs2}
		rtts := append(routeTables, routeTables2...)

		containingVs := virtualServicesForRouteTable(routeTables[0], vss, rtts)
		Expect(containingVs).To(Equal(v1.VirtualServiceList{vs}))

		containingVs = virtualServicesForRouteTable(routeTables2[0], vss, rtts)
		Expect(containingVs).To(Equal(v1.VirtualServiceList{vs2}))
	})
})
