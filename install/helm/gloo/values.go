package gloo

// Common
type Image struct {
	Name       string `json:"name"`
	Tag        string `json:"tag"`
	Repository string `json:"repository"`
	PullPolicy string `json:"pull_policy"`
	PullSecret string `json:"pull_secret"`
}

type Envoy struct {
	Static EnvoyStatic `json:"static"`
}

type EnvoyStatic struct {
	Listeners []string `json:"listeners"`
	Clusters  []string `json:"clusters"`
}

type DeploymentSpec struct {
	Replicas int `json:"replicas"`
}

// Gloo
type Settings struct {
	WatchNamespaces []string `json:"watch_namespaces"`
	WriteNamespace  string   `json:"write_namespace"`
}

type Gloo struct {
	Deployment GlooDeployment `json:"deployment"`
}

type GlooDeployment struct {
	Image   Image  `json:"image"`
	XdsPort string `json:"xds_port"`
	*DeploymentSpec
}

type Discovery struct {
	Deployment DiscoveryDeployment `json:"deployment"`
}

type DiscoveryDeployment struct {
	Image Image `json:"image"`
	*DeploymentSpec
}

type Gateway struct {
	Deployment GatewayDeployment `json:"deployment"`
}

type GatewayDeployment struct {
	Image Image `json:"image"`
	*DeploymentSpec
}

type GatewayProxy struct {
	Deployment GatewayProxyDeployment `json:"deployment"`
	ConfigMap  GatewayProxyConfigMap  `json:"config_map"`
}

type GatewayProxyDeployment struct {
	Image       Image             `json:"image"`
	HttpPort    string            `json:"http_port"`
	ExtraPorts       map[string]string `json:"extra_ports"`
	ExtraAnnotations map[string]string `json:"extra_annotations"`
}

type GatewayProxyConfigMap struct {
	Envoy Envoy `json:"envoy"`
}

type Ingress struct {
	Deployment IngressDeployment `json:"deployment"`
}

type IngressDeployment struct {
	Image Image `json:"image"`
	*DeploymentSpec
}

type IngressProxy struct {
	Deployment IngressProxyDeployment `json:"deployment"`
	ConfigMap  IngressProxyConfigMap  `json:"config_map"`
}

type IngressProxyDeployment struct {
	Image            Image             `json:"image"`
	HttpPort         string            `json:"http_port"`
	HttpsPort        string            `json:"https_port"`
	ExtraPorts       map[string]string `json:"extra_ports"`
	ExtraAnnotations map[string]string `json:"extra_annotations"`
	*DeploymentSpec
}

type IngressProxyConfigMap struct {
	Envoy Envoy `json:"envoy"`
}
