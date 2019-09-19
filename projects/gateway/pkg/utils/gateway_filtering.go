package utils

import (
	"github.com/solo-io/gloo/projects/gateway/pkg/defaults"

	v2 "github.com/solo-io/gloo/projects/gateway/pkg/api/v2"
)

func GatewaysByProxyName(gateways v2.GatewayList) map[string]v2.GatewayList {
	result := make(map[string]v2.GatewayList)
	for _, gw := range gateways {
		proxyNames := GetProxyNamesForGateway(gw)
		for _, name := range proxyNames {
			result[name] = append(result[name], gw)
		}
	}
	return result
}

func GetProxyNamesForGateway(gw *v2.Gateway) []string {
	proxyNames := gw.ProxyNames
	if len(proxyNames) == 0 {
		if gw.GatewayProxyName != "" {
			proxyNames = []string{gw.GatewayProxyName}
		} else {
			proxyNames = []string{defaults.GatewayProxyName}
		}
	}
	return proxyNames
}
