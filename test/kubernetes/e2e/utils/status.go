package utils

import (
	"strings"

	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

func HTTPRouteStatusContainsMsg(route *gwv1.HTTPRoute, msg string) bool {
	for _, parent := range route.Status.RouteStatus.Parents {
		for _, condition := range parent.Conditions {
			if strings.Contains(condition.Message, msg) {
				return true
			}
		}
	}
	return false
}
