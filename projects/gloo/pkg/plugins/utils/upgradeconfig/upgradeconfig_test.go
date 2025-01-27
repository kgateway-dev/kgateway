package upgradeconfig_test

import (
	envoy_config_route_v3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	envoyhttp "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	. "github.com/solo-io/gloo/projects/gloo/pkg/plugins/utils/upgradeconfig"
)

var _ = Describe("Upgradeconfig", func() {
	Context("HCM", func() {
		It("should not error on empty list", func() {
			err := ValidateHCMUpgradeConfigs(nil)
			Expect(err).NotTo(HaveOccurred())
		})
		It("should allow websocket upgrade", func() {
			configs := []*envoyhttp.HttpConnectionManager_UpgradeConfig{{
				UpgradeType: WebSocketUpgradeType,
			}}
			err := ValidateHCMUpgradeConfigs(configs)
			Expect(err).NotTo(HaveOccurred())
		})
		It("should not allow double websocket upgrade", func() {
			configs := []*envoyhttp.HttpConnectionManager_UpgradeConfig{{
				UpgradeType: WebSocketUpgradeType,
			}, {
				UpgradeType: WebSocketUpgradeType,
			}}
			err := ValidateHCMUpgradeConfigs(configs)
			Expect(err).To(HaveOccurred())
		})
		It("should allow websocket and connect upgrade", func() {
			configs := []*envoyhttp.HttpConnectionManager_UpgradeConfig{{
				UpgradeType: WebSocketUpgradeType,
			}, {
				UpgradeType: ConnectUpgradeType,
			}}
			err := ValidateHCMUpgradeConfigs(configs)
			Expect(err).NotTo(HaveOccurred())
		})
	})
	Context("Route", func() {
		It("should not error on empty list", func() {
			err := ValidateRouteUpgradeConfigs(nil)
			Expect(err).NotTo(HaveOccurred())
		})
		It("should allow websocket upgrade", func() {
			configs := []*envoy_config_route_v3.RouteAction_UpgradeConfig{{
				UpgradeType: WebSocketUpgradeType,
			}}
			err := ValidateRouteUpgradeConfigs(configs)
			Expect(err).NotTo(HaveOccurred())
		})
		It("should not allow double websocket upgrade", func() {
			configs := []*envoy_config_route_v3.RouteAction_UpgradeConfig{{
				UpgradeType: WebSocketUpgradeType,
			}, {
				UpgradeType: WebSocketUpgradeType,
			}}
			err := ValidateRouteUpgradeConfigs(configs)
			Expect(err).To(HaveOccurred())
		})
	})

})
