package defaults

import (
	"time"
)

const GlooRestXdsName = "rest_xds_cluster"

var HttpPort uint32 = 8080
var HttpsPort uint32 = 8443
var EnvoyAdminPort uint32 = 19000
var GlooAdminPort uint32 = 9091
var GlooProxyDebugPort = 9966
var GlooRestXdsPort = 9976
var GlooXdsPort = 9977
var GlooValidationPort = 9988
var GlooMtlsModeRestXdsPort = 9998
var GlooMtlsModeXdsPort = 9999
var DefaultRefreshRate = time.Minute
var TapServerHttpPort uint16 = 9001
var TapServerGrpcPort uint16 = 9002

// https://github.com/solo-io/gloo/blob/1de5515c1fd655c462dce8f0d1f0342fe5400e4e/install/helm/gloo/templates/9-gateway-proxy-configmap.yaml#L58
var PrometheusListenerPort = 8081

// https://github.com/solo-io/gloo/blob/1de5515c1fd655c462dce8f0d1f0342fe5400e4e/install/helm/gloo/templates/9-gateway-proxy-configmap.yaml#L96
var ReadConfigListenerPort = 8082

// Used for testing
var TcpPort uint32 = 8000
var HybridPort uint32 = 8087
