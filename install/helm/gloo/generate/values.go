package generate

type Config struct {
	Namespace    Namespace `json:"namespace"`
	Rbac         Rbac `json:"rbac"`
	Settings     Settings `json:"settings"`
	Gloo         Gloo `json:"gloo"`
	Discovery    Discovery `json:"discovery"`
	Gateway      Gateway `json:"gateway"`
	GatewayProxy GatewayProxy `json:"gatewayProxy"`
	Ingress      Ingress `json:"ingress"`
	IngressProxy IngressProxy `json:"ingressProxy"`
}

type Namespace struct {
	Create bool `json:"create"`
}

type Rbac struct {
	Create bool `json:"create"`
}

// Common
type Image struct {
	Tag        string `json:"tag"`
	Repository string `json:"repository"`
	PullPolicy string `json:"pullPolicy"`
	PullSecret string `json:"pullSecret,omitempty"`
}

type DeploymentSpec struct {
	Replicas int `json:"replicas"`
}

type Integrations struct {
	Knative Knative `json:"knative"`
}
type Knative struct {
	Enabled bool `json:"enabled"`
	Proxy   KnativeProxy `json:"proxy,omitempty"`
}

type KnativeProxy struct {
	Image            Image             `json:"image"`
	HttpPort         string            `json:"httpPort"`
	HttpsPort        string            `json:"httpsPort"`
	*DeploymentSpec
}

type Settings struct {
	WatchNamespaces []string `json:"watchNamespaces"`
	WriteNamespace  string   `json:"writeNamespace"`
	Integrations    Integrations `json:"integrations"`
}

type Gloo struct {
	Deployment GlooDeployment `json:"deployment"`
}

type GlooDeployment struct {
	Image   Image  `json:"image"`
	XdsPort string `json:"xdsPort"`
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
	ConfigMap  GatewayProxyConfigMap  `json:"configMap"`
}

type GatewayProxyDeployment struct {
	Image            Image             `json:"image"`
	HttpPort         string            `json:"httpPort"`
	ExtraPorts       map[string]string `json:"extraPorts,omitempty"`
	ExtraAnnotations map[string]string `json:"extraAnnotations,omitempty"`
}

type GatewayProxyConfigMap struct {
	Data string `json:"data"`
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
	ConfigMap  IngressProxyConfigMap  `json:"configMap"`
}

type IngressProxyDeployment struct {
	Image            Image             `json:"image"`
	HttpPort         string            `json:"httpPort"`
	HttpsPort        string            `json:"httpsPort"`
	ExtraPorts       map[string]string `json:"extraPorts,omitempty"`
	ExtraAnnotations map[string]string `json:"extraAnnotations,omitempty"`
	*DeploymentSpec
}

type IngressProxyConfigMap struct {
	Data string `json:"data"`
}
