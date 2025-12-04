package waypointquery

import gwv1 "sigs.k8s.io/gateway-api/apis/v1"

// IstioWaypointForLabel is the Istio API applied on Gateway resources to
// declare the types of traffic it supports.
// See https://istio.io/latest/docs/ambient/usage/waypoint/#waypoint-traffic-types.
const IstioWaypointForLabel = "istio.io/waypoint-for"

// WaypointFor represents the types of traffic supported by a given waypoint.
type WaypointFor string

const (
	WaypointForAll      = "all"
	WaypointForService  = "service"
	WaypointForWorkload = "workload"

	defaultWaypointFor = WaypointForService
)

func (w WaypointFor) ForService() bool {
	return w == WaypointForAll || w == WaypointForService
}

func (w WaypointFor) ForWorkload() bool {
	return w == WaypointForAll || w == WaypointForWorkload
}

func GetWaypointFor(gw *gwv1.Gateway) WaypointFor {
	if value, ok := gw.Labels[IstioWaypointForLabel]; ok {
		// invalid values are effectively WaypointForNone
		// since ForService() and ForWorkload() will return false
		return WaypointFor(value)
	}
	return defaultWaypointFor
}
