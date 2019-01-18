package gloo

// Common
type Image struct {
	Name       string `json:"name"`
	Tag        string `json:"tag"`
	Repository string `json:"repository"`
	PullPolicy string `json:"pull_policy"`
}

type Envoy struct {
	Static EnvoyStatic `json:"static"`
}

type EnvoyStatic struct {
	Listeners []string `json:"listeners"`
	Clusters  []string `json:"clusters"`
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
}

type Discovery struct {
	Deployment DiscoveryDeployment `json:"deployment"`
}

type DiscoveryDeployment struct {
	Image Image `json:"image"`
}

type Gateway struct {
	Deployment GatewayDeployment `json:"deployment"`
}

type GatewayDeployment struct {
	Image Image `json:"image"`
}

type GatewayProxy struct {
	Deployment GatewayProxyDeployment `json:"deployment"`
	ConfigMap  GatewayProxyConfigMap  `json:"config_map"`
}

type GatewayProxyDeployment struct {
	Image    Image  `json:"image"`
	HttpPort string `json:"http_port"`
}

type GatewayProxyConfigMap struct {
	Envoy Envoy `json:"envoy"`
}

type Ingress struct {
	Deployment IngressDeployment `json:"deployment"`
}

type IngressDeployment struct {
	Image Image `json:"image"`
}

type IngressProxy struct {
	Deployment IngressProxyDeployment `json:"deployment"`
	ConfigMap  IngressProxyConfigMap  `json:"config_map"`
}

type IngressProxyDeployment struct {
	Image     Image  `json:"image"`
	HttpPort  string `json:"http_port"`
	HttpsPort string `json:"https_port"`
}

type IngressProxyConfigMap struct {
	Envoy Envoy `json:"envoy"`
}
