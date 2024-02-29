package utils_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gateway/pkg/defaults"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"

	. "github.com/solo-io/gloo/projects/gateway/pkg/utils"
)

var _ = Describe("gateway util unit tests", func() {
	Describe("GatewaysByProxyName", func() {
		It("assigns each gateway once to each proxy by their proxyNames", func() {

			gws := v1.GatewayList{
				{Metadata: &core.Metadata{Name: "gw1"}, ProxyNames: nil /*default proxy*/},
				{Metadata: &core.Metadata{Name: "gw2"}, ProxyNames: []string{"proxy1", "proxy2"}},
				{Metadata: &core.Metadata{Name: "gw3"}, ProxyNames: []string{"proxy1", defaults.GatewayProxyName}},
			}

			gw1, gw2, gw3 := gws[0], gws[1], gws[2]

			byProxy := GatewaysByProxyName(gws)
			Expect(byProxy).To(Equal(map[string]v1.GatewayList{
				defaults.GatewayProxyName: {gw1, gw3},
				"proxy1":                  {gw2, gw3},
				"proxy2":                  {gw2},
			}))
		})
	})

	Describe("SortedGatewaysByProxyName", func() {
		// Must pass repeatedly so we don't accidentally get the right order by chance
		It("assigns each gateway once to each proxy by their proxyNames", MustPassRepeatedly(5), func() {

			gws := v1.GatewayList{
				{Metadata: &core.Metadata{Name: "gw5"}, ProxyNames: []string{"proxy3", "proxy2"}},
				{Metadata: &core.Metadata{Name: "gw4"}, ProxyNames: []string{"proxy1", "proxy4"}},
				{Metadata: &core.Metadata{Name: "gw3"}, ProxyNames: []string{"proxy1", defaults.GatewayProxyName}},
				{Metadata: &core.Metadata{Name: "gw2"}, ProxyNames: []string{"proxy1", "proxy2"}},
				{Metadata: &core.Metadata{Name: "gw1"}, ProxyNames: nil /*default proxy*/},
			}

			gw5, gw4, gw3, gw2, gw1 := gws[0], gws[1], gws[2], gws[3], gws[4]

			byProxy := SortedGatewaysByProxyName(gws)
			Expect(byProxy).To(Equal([]GatewaysAndProxyName{
				{Gateways: v1.GatewayList{gw3, gw1}, Name: defaults.GatewayProxyName},
				{Gateways: v1.GatewayList{gw4, gw3, gw2}, Name: "proxy1"},
				{Gateways: v1.GatewayList{gw5, gw2}, Name: "proxy2"},
				{Gateways: v1.GatewayList{gw5}, Name: "proxy3"},
				{Gateways: v1.GatewayList{gw4}, Name: "proxy4"},
			}))
		})
	})
})
