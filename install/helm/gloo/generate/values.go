package generate

import (
	appsv1 "k8s.io/api/core/v1"
)

type HelmConfig struct {
	Config
	Global *Global `json:"global,omitempty"`
}

type Config struct {
	Namespace      *Namespace              `json:"namespace,omitempty"`
	Rbac           *Rbac                   `json:"rbac,omitempty"`
	Crds           *Crds                   `json:"crds,omitempty"`
	Settings       *Settings               `json:"settings,omitempty"`
	Gloo           *Gloo                   `json:"gloo,omitempty"`
	Discovery      *Discovery              `json:"discovery,omitempty"`
	Gateway        *Gateway                `json:"gateway,omitempty"`
	GatewayProxies map[string]GatewayProxy `json:"gatewayProxies,omitempty"`
	Ingress        *Ingress                `json:"ingress,omitempty"`
	IngressProxy   *IngressProxy           `json:"ingressProxy,omitempty"`
	K8s            *K8s                    `json:"k8s,omitempty"`
}

type Global struct {
	Image      *Image      `json:"image,omitempty"`
	Extensions interface{} `json:"extensions,omitempty"`
}

type Namespace struct {
	Create bool `json:"create" desc:"create the installation namespace"`
}

type Rbac struct {
	Create     bool `json:"create" desc:"create rbac rules for the gloo-system service account"`
	Namespaced bool `json:"Namespaced" desc:"use Roles instead of ClusterRoles"`
}

type Crds struct {
	Create bool `json:"create" desc:"create CRDs for Gloo (turn off if installing with Helm to a cluster that already has Gloo CRDs)"`
}

// Common
type Image struct {
	Tag        string `json:"tag,omitempty"  desc:"tag for the container"`
	Repository string `json:"repository,omitempty"  desc:"image name (repository) for the container."`
	Registry   string `json:"registry,omitempty" desc:"image prefix/registry e.g. (quay.io/solo-io)"`
	PullPolicy string `json:"pullPolicy,omitempty"  desc:"image pull policy for the container"`
	PullSecret string `json:"pullSecret,omitempty" desc:"image pull policy for the container "`
}

type ResourceAllocation struct {
	Memory string `json:"memory,omitEmpty" desc:"amount of memory"`
	CPU    string `json:"cpu,omitEmpty" desc:"amount of CPUs"`
}

type ResourceRequirements struct {
	Limits   *ResourceAllocation `json:"limits,omitEmpty" desc:"resource limits of this container"`
	Requests *ResourceAllocation `json:"requests,omitEmpty" desc:"resource requests of this container"`
}
type PodSpec struct {
	RestartPolicy string `json:"restartPolicy,omitempty" desc:"restart policy to use when the pod exits"`
}

type JobSpec struct {
	*PodSpec
}

type DeploymentSpec struct {
	Replicas  int                   `json:"replicas" desc:"number of instances to deploy"`
	Resources *ResourceRequirements `json:"resources,omitEmpty" desc:"resources for the main pod in the deployment"`
}

type Integrations struct {
	Knative *Knative `json:"knative,omitEmpty"`
}

type Knative struct {
	Enabled *bool         `json:"enabled" desc:"enabled knative components"`
	Version *string       `json:"version,omitEmpty" desc:"the version of knative installed to the cluster. if using version < 0.8.0, gloo will use Knative's ClusterIngress API for configuration rather than the namespace-scoped Ingress"`
	Proxy   *KnativeProxy `json:"proxy,omitempty"`
}

type KnativeProxy struct {
	Image     *Image  `json:"image,omitempty"`
	HttpPort  int     `json:"httpPort,omitempty" desc:"HTTP port for the proxy"`
	HttpsPort int     `json:"httpsPort,omitempty" desc:"HTTPS port for the proxy"`
	Tracing   *string `json:"tracing,omitempty" desc:"tracing configuration"`
	*DeploymentSpec
}

type Settings struct {
	WatchNamespaces []string      `json:"watchNamespaces,omitempty" desc:"whitelist of namespaces for gloo to watch for services and CRDs. Empty list means all namespaces"`
	WriteNamespace  string        `json:"writeNamespace,omitempty" desc:"namespace where intermediary CRDs will be written to, e.g. Upstreams written by Gloo Discovery."`
	Integrations    *Integrations `json:"integrations,omitempty"`
	Create          bool          `json:"create,omitempty" desc:"create a Settings CRD which configures Gloo controllers at boot time"`
	Extensions      interface{}   `json:"extensions,omitempty"`
	SingleNamespace bool          `json:"singleNamespace,omitempty" desc:"Enable to use install namespace as WatchNamespace and WriteNamespace"`
}

type Gloo struct {
	Deployment *GlooDeployment `json:"deployment,omitempty"`
}

type GlooDeployment struct {
	Image   *Image `json:"image,omitempty"`
	XdsPort int    `json:"xdsPort,omitempty" desc:"port where gloo serves xDS API to Envoy"`
	Stats   bool   `json:"stats" desc:"enable prometheus stats"`
	*DeploymentSpec
}

type Discovery struct {
	Deployment *DiscoveryDeployment `json:"deployment,omitempty"`
	FdsMode    string               `json:"fdsMode" desc:"mode for function discovery (blacklist\whitelist). See more info in the settings docs"`
}

type DiscoveryDeployment struct {
	Image *Image `json:"image,omitempty"`
	Stats bool   `json:"stats" desc:"enable prometheus stats"`
	*DeploymentSpec
}

type Gateway struct {
	Enabled       *bool                 `json:"enabled" desc:"enable Gloo API Gateway features"`
	Upgrade       *bool                 `json:"upgrade" desc:"Deploy a Job to convert (but not delete) v1 Gateway resources to v2 and not add a "live" label to the gateway-proxy deployment's pod template. This allows for canary testing of gateway-v2 alongside an existing instance of gloo running with v1 gateway resources and controllers."`
	Deployment    *GatewayDeployment    `json:"deployment,omitempty"`
	ConversionJob *GatewayConversionJob `json:"conversionJob,omitempty"`
}

type GatewayDeployment struct {
	Image *Image `json:"image,omitempty"`
	Stats bool   `json:"stats" desc:"enable prometheus stats"`
	*DeploymentSpec
}

type GatewayConversionJob struct {
	Image *Image `json:"image,omitempty"`
	*JobSpec
}

type GatewayProxy struct {
	Kind                  *GatewayProxyKind        `json:"kind,omitempty"`
	PodTemplate           *GatewayProxyPodTemplate `json:"podTemplate,omitempty"`
	ConfigMap             *GatewayProxyConfigMap   `json:"configMap,omitempty"`
	Service               *GatewayProxyService     `json:"service,omitempty"`
	Tracing               *Tracing                 `json:"tracing,omitempty"`
	ExtraContainersHelper string                   `json:"extraContainersHelper,omitempty"`
}

type GatewayProxyKind struct {
	Deployment *DeploymentSpec `json:"deployment,omitempty"`
	DaemonSet  *DaemonSetSpec  `json:"daemonSet,omitempty"`
}

type DaemonSetSpec struct {
	HostPort bool `json:"hostPort" desc:"whether or not to enable host networking on the pod. Only relevant when running as a DaemonSet"`
}

type GatewayProxyPodTemplate struct {
	Image            *Image                `json:"image,omitempty"`
	HttpPort         int                   `json:"httpPort,omitempty" desc:"HTTP port for the gateway service"`
	HttpsPort        int                   `json:"httpsPort,omitempty" desc:"HTTPS port for the gateway service"`
	ExtraPorts       []interface{}         `json:"extraPorts,omitempty" desc:"extra ports for the gateway pod"`
	ExtraAnnotations map[string]string     `json:"extraAnnotations,omitempty" desc:"extra annotations to add to the pod"`
	NodeName         string                `json:"nodeName,omitempty" desc:"name of node to run on"`
	NodeSelector     map[string]string     `json:"nodeSelector,omitempty" desc:"label selector for nodes"`
	Stats            bool                  `json:"stats" desc:"enable prometheus stats"`
	Tolerations      []*appsv1.Toleration  `json:"tolerations,omitEmpty"`
	Probes           bool                  `json:"probes" desc:"enable liveness and readiness probes"`
	Resources        *ResourceRequirements `json:"resources"`
}

type GatewayProxyService struct {
	Type                  string            "json:\"type,omitempty\" desc:\"gateway [service type](https://kubernetes.io/docs/concepts/services-networking/service/#publishing-services-service-types). default is `LoadBalancer`\""
	HttpPort              int               `json:"httpPort,omitempty" desc:"HTTP port for the gateway service"`
	HttpsPort             int               `json:"httpsPort,omitempty" desc:"HTTPS port for the gateway service"`
	ClusterIP             string            "json:\"clusterIP,omitempty\" desc:\"static clusterIP (or `None`) when `gatewayProxies[].gatewayProxy.service.type` is `ClusterIP`\""
	ExtraAnnotations      map[string]string `json:"extraAnnotations,omitempty"`
	ExternalTrafficPolicy string            `json:"externalTrafficPolicy,omitempty"`
}

type Tracing struct {
	Provider string `json:"provider",omitempty`
	Cluster  string `json:"cluster",omitempty`
}

type GatewayProxyConfigMap struct {
	Data map[string]string `json:"data"`
}

type Ingress struct {
	Enabled             *bool              `json:"enabled"`
	Deployment          *IngressDeployment `json:"deployment,omitempty"`
	RequireIngressClass *bool              `json:"requireIngressClass,omitempty" desc:"only serve traffic for Ingress objects with the annotation 'kubernetes.io/ingress.class: gloo''"`
}

type IngressDeployment struct {
	Image *Image `json:"image,omitempty"`
	*DeploymentSpec
}

type IngressProxy struct {
	Deployment *IngressProxyDeployment `json:"deployment,omitempty"`
	ConfigMap  *IngressProxyConfigMap  `json:"configMap,omitempty"`
	Tracing    *string                 `json:"tracing,omitempty"`
}

type IngressProxyDeployment struct {
	Image            *Image            `json:"image,omitempty"`
	HttpPort         int               `json:"httpPort,omitempty" desc:"HTTP port for the ingress container"`
	HttpsPort        int               `json:"httpsPort,omitempty" desc:"HTTPS port for the ingress container"`
	ExtraPorts       []interface{}     `json:"extraPorts,omitempty"`
	ExtraAnnotations map[string]string `json:"extraAnnotations,omitempty"`
	*DeploymentSpec
}

type IngressProxyConfigMap struct {
	Data map[string]string `json:"data,omitempty"`
}

type K8s struct {
	ClusterName string `json:"clusterName" desc:"cluster name to use when referencing services."`
}
