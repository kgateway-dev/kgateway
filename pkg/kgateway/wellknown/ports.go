package wellknown

// DefaultXdsService is the default name of the Kubernetes Service that serves xDS config.
// This value should stay in sync with:
// - the default value of `XdsServiceName` in pkg/kgateway/extensions2/settings/settings.go
// - the default Service name in install/helm/kgateway/templates/service.yaml
const DefaultXdsService = "kgateway"

// DefaultXdsPort is the default xDS port. This value should stay in sync with:
// - the default value of `XdsServicePort` in pkg/settings/settings.go
// - the `controller.service.ports.grpc` value in install/helm/kgateway/values.yaml
var DefaultXdsPort uint32 = 9977

// EnvoyAdminPort is the default envoy admin port. This value should stay in sync with
// pkg/kgateway/helm/envoy/templates/configmap.yaml and deployment.yaml.
var EnvoyAdminPort uint32 = 19000

// EnvoyReadinessPort is the default port of the envoy readiness listener. This value
// should stay in sync with pkg/kgateway/helm/envoy/templates/configmap.yaml.
var EnvoyReadinessPort uint32 = 8082

// EnvoyMetricsPort is the default port of the envoy Prometheus listener. This value
// should stay in sync with pkg/kgateway/helm/envoy/templates/configmap.yaml and
// deployment.yaml.
var EnvoyMetricsPort uint32 = 9091

// KgatewayAdminPort is the kgateway admin server port
var KgatewayAdminPort uint32 = 9095
