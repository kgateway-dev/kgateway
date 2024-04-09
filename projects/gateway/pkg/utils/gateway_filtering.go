package utils

import (
	"sort"

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

// GatewaysAndProxyName is a struct that holds a list of gateways and a proxy name
type GatewaysAndProxyName struct {
	Gateways v1.GatewayList
	Name     string
}

// SortedGatewaysByProxyName returns a slice of GatewaysAndProxyName structs, sorted by proxy name
func SortedGatewaysByProxyName(gateways v1.GatewayList) []GatewaysAndProxyName {
	// Create the mapping
	gatewaysByProxyName := make(map[string]v1.GatewayList)
	for _, gw := range gateways {
		proxyNames := GetProxyNamesForGateway(gw)
		for _, name := range proxyNames {
			gatewaysByProxyName[name] = append(gatewaysByProxyName[name], gw)
		}
	}

	// Create and populate the slice
	var gatewayAndProxyNames []GatewaysAndProxyName

	for name, gateways := range gatewaysByProxyName {
		gatewayAndProxyNames = append(gatewayAndProxyNames, GatewaysAndProxyName{
			Gateways: gateways,
			Name:     name,
		})
	}

	// sort the slice by the struct's Name field
	sort.Slice(gatewayAndProxyNames, func(i, j int) bool {
		return gatewayAndProxyNames[i].Name < gatewayAndProxyNames[j].Name
	})

	return gatewayAndProxyNames
}

func GetProxyNamesForGateway(gw *v1.Gateway) []string {
	proxyNames := gw.GetProxyNames()
	if len(proxyNames) == 0 {
		proxyNames = []string{defaults.GatewayProxyName}
	}
	return proxyNames
}
