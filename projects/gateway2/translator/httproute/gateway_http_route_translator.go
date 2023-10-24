package httproute

import (
	"github.com/solo-io/gloo/projects/gateway2/reports"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	apiv1 "sigs.k8s.io/gateway-api/apis/v1"
)

// GatewayRouteTable is shorthand for K8s Gateway v1beta1.HTTPRoute
type GatewayHTTPRouteTable = apiv1.HTTPRoute

// TranslateHTTPRoutes translates the set of gloo VirtualHosts required to produce the routes needed by a Gloo HTTP Filter Chain (Envoy HCM)
// the Routes passed in are assumed to be the entire set of HTTP routes intended to be exposed on a single HTTP Filter Chain.
func TranslateGatewayHTTPRoutes(parentName string, parentHost *apiv1.Hostname, hrts []GatewayHTTPRouteTable, reporter reports.Reporter) map[string]*v1.VirtualHost {
	return nil
}
