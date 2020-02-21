package utils

import (
	"github.com/solo-io/gloo/projects/gateway/pkg/defaults"

	v1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
)

func GatewaysByProxyName(gateways v1.GatewayList) map[string]v1.GatewayList {
	result := make(map[string]v1.GatewayList)
	for _, gw := range gateways {
		proxyNames := GetProxyNamesForGateway(gw)
		for _, name := range proxyNames {
			result[name] = append(result[name], gw)
		}
	}
	return result
}

func GetProxyNamesForGateway(gw *v1.Gateway) []string {
	proxyNames := gw.ProxyNames
	if len(proxyNames) == 0 {
		proxyNames = []string{defaults.GatewayProxyName}
	}
	return proxyNames
}
