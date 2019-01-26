package generate

type Config struct {
	Namespace    *Namespace    `json:"namespace"`
	Rbac         *Rbac         `json:"rbac"`
	Settings     *Settings     `json:"settings"`
	Gloo         *Gloo         `json:"gloo"`
	Discovery    *Discovery    `json:"discovery,omitempty"`
	Gateway      *Gateway      `json:"gateway,omitempty"`
	GatewayProxy *GatewayProxy `json:"gatewayProxy,omitempty"`
	Ingress      *Ingress      `json:"ingress,omitempty"`
	IngressProxy *IngressProxy `json:"ingressProxy,omitempty"`
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
	Knative *Knative `json:"knative"`
}
type Knative struct {
	Enabled *bool          `json:"enabled"`
	Proxy   *KnativeProxy `json:"proxy,omitempty"`
}

type KnativeProxy struct {
	Image     *Image `json:"image,omitempty"`
	HttpPort  string `json:"httpPort,omitempty"`
	HttpsPort string `json:"httpsPort,omitempty"`
	*DeploymentSpec
}

type Settings struct {
	WatchNamespaces []string      `json:"watchNamespaces"`
	WriteNamespace  string        `json:"writeNamespace"`
	Integrations    *Integrations `json:"integrations,omitempty"`
}

type Gloo struct {
	Deployment *GlooDeployment `json:"deployment,omitempty"`
}

type GlooDeployment struct {
	Image   *Image `json:"image,omitempty"`
	XdsPort string `json:"xdsPort,omitempty"`
	*DeploymentSpec
}

type Discovery struct {
	Deployment *DiscoveryDeployment `json:"deployment,omitempty"`
}

type DiscoveryDeployment struct {
	Image *Image `json:"image,omitempty"`
	*DeploymentSpec
}

type Gateway struct {
	Enabled    *bool               `json:"enabled"`
	Deployment *GatewayDeployment `json:"deployment,omitempty"`
}

type GatewayDeployment struct {
	Image *Image `json:"image,omitempty"`
	*DeploymentSpec
}

type GatewayProxy struct {
	Deployment *GatewayProxyDeployment `json:"deployment,omitempty"`
	ConfigMap  *GatewayProxyConfigMap  `json:"configMap,omitempty"`
}

type GatewayProxyDeployment struct {
	Image            *Image            `json:"image,omitempty"`
	HttpPort         string            `json:"httpPort,omitempty"`
	ExtraPorts       []interface{}     `json:"extraPorts,omitempty"`
	ExtraAnnotations map[string]string `json:"extraAnnotations,omitempty"`
}

type GatewayProxyConfigMap struct {
	Data map[string]string `json:"data"`
}

type Ingress struct {
	Enabled    *bool               `json:"enabled"`
	Deployment *IngressDeployment `json:"deployment,omitempty"`
}

type IngressDeployment struct {
	Image *Image `json:"image,omitempty"`
	*DeploymentSpec
}

type IngressProxy struct {
	Deployment *IngressProxyDeployment `json:"deployment,omitempty"`
	ConfigMap  *IngressProxyConfigMap  `json:"configMap,omitempty"`
}

type IngressProxyDeployment struct {
	Image            *Image            `json:"image,omitempty"`
	HttpPort         string            `json:"httpPort,omitempty"`
	HttpsPort        string            `json:"httpsPort,omitempty"`
	ExtraPorts       []interface{}     `json:"extraPorts,omitempty"`
	ExtraAnnotations map[string]string `json:"extraAnnotations,omitempty"`
	*DeploymentSpec
}

type IngressProxyConfigMap struct {
	Data map[string]string `json:"data,omitempty"`
}
