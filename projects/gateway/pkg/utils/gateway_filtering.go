package utils

import (
	"github.com/solo-io/gloo/projects/gateway/pkg/defaults"

	v1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
)

func GatewaysByProxyName(gateways v1.GatewayList) (map[string]v1.GatewayList, v1.GatewayList) {
	gatewaysByProxy := make(map[string]v1.GatewayList)
	var orphanedGateways v1.GatewayList

	for _, gw := range gateways {
		proxyNames := gw.GetProxyNames()
		if len(proxyNames) == 0 {
			// append to orphanedGateways is there are no associated proxies
			orphanedGateways = append(orphanedGateways, gw)
		} else {
			// append to gatewaysByProxy if there is at least 1 associated proxy
			for _, name := range proxyNames {
				gatewaysByProxy[name] = append(gatewaysByProxy[name], gw)
			}
		}
	}

	return gatewaysByProxy, orphanedGateways
}

func GetProxyNamesForGateway(gw *v1.Gateway) []string {
	proxyNames := gw.GetProxyNames()
	if len(proxyNames) == 0 {
		proxyNames = []string{defaults.GatewayProxyName}
	}
	return proxyNames
}
