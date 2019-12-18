package upgradeconfig_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	. "github.com/solo-io/gloo/projects/gloo/pkg/plugins/utils/upgradeconfig"

	envoyroute "github.com/envoyproxy/go-control-plane/envoy/api/v2/route"
	envoyhttp "github.com/envoyproxy/go-control-plane/envoy/config/filter/network/http_connection_manager/v2"
)

var _ = Describe("Upgradeconfig", func() {

	It("should not error on empty list", func() {
		err := ValidateHCMUpgradeConfigs(nil)
		Expect(err).NotTo(HaveOccurred())
	})
	It("should not error on empty list", func() {
		err := ValidateRouteUpgradeConfigs(nil)
		Expect(err).NotTo(HaveOccurred())
	})
	It("should allow websocket upgrade", func() {
		configs := []*envoyroute.RouteAction_UpgradeConfig{{
			UpgradeType: WebSocketUpgradeType,
		}}
		err := ValidateRouteUpgradeConfigs(configs)
		Expect(err).NotTo(HaveOccurred())
	})
	It("should allow websocket upgrade", func() {
		configs := []*envoyhttp.HttpConnectionManager_UpgradeConfig{{
			UpgradeType: WebSocketUpgradeType,
		}}
		err := ValidateHCMUpgradeConfigs(configs)
		Expect(err).NotTo(HaveOccurred())
	})
	It("should not allow route double websocket upgrade", func() {
		configs := []*envoyroute.RouteAction_UpgradeConfig{{
			UpgradeType: WebSocketUpgradeType,
		}, {
			UpgradeType: WebSocketUpgradeType,
		}}
		err := ValidateRouteUpgradeConfigs(configs)
		Expect(err).To(HaveOccurred())
	})
	It("should not allow route double websocket upgrade", func() {
		configs := []*envoyhttp.HttpConnectionManager_UpgradeConfig{{
			UpgradeType: WebSocketUpgradeType,
		}, {
			UpgradeType: WebSocketUpgradeType,
		}}
		err := ValidateHCMUpgradeConfigs(configs)
		Expect(err).To(HaveOccurred())
	})

})
