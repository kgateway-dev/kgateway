package generate

import (
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	appsv1 "k8s.io/api/core/v1"
)

type HelmConfig struct {
	Config
	Global *Global `json:"global,omitempty"`
}

type Config struct {
	Namespace      *Namespace              `json:"namespace,omitempty"`
	Crds           *Crds                   `json:"crds,omitempty"`
	Settings       *Settings               `json:"settings,omitempty"`
	Gloo           *Gloo                   `json:"gloo,omitempty"`
	Discovery      *Discovery              `json:"discovery,omitempty"`
	Gateway        *Gateway                `json:"gateway,omitempty"`
	GatewayProxies map[string]GatewayProxy `json:"gatewayProxies,omitempty"`
	Ingress        *Ingress                `json:"ingress,omitempty"`
	IngressProxy   *IngressProxy           `json:"ingressProxy,omitempty"`
	K8s            *K8s                    `json:"k8s,omitempty"`
	AccessLogger   *AccessLogger           `json:"accessLogger,omitempty"`
}

type Global struct {
	Image      *Image      `json:"image,omitempty"`
	Extensions interface{} `json:"extensions,omitempty"`
	GlooRbac   *Rbac       `json:"glooRbac,omitempty"`
	Wasm       Wasm        `json:"wasm,omitempty"`
	GlooStats  Stats       `json:"glooStats,omitempty" desc:"Config used as the default values for Prometheus stats published from Gloo pods. Can be overridden by individual deployments"`
	GlooMtls   Mtls        `json:"glooMtls,omitempty" desc:"Config used to enable internal mtls authentication (currently just Gloo to Envoy communication)"`
}

type Namespace struct {
	Create bool `json:"create" desc:"create the installation namespace"`
}

type Crds struct {
	Create bool `json:"create" desc:"create CRDs for Gloo (turn off if installing with Helm to a
cluster that already has Gloo CRDs). This field is deprecated and is included only to ensure backwards-compatibility with Helm 2."`
}

type Rbac struct {
	Create     bool   `json:"create" desc:"create rbac rules for the gloo-system service account"`
	Namespaced bool   `json:"namespaced" desc:"use Roles instead of ClusterRoles"`
	NameSuffix string `json:"nameSuffix" desc:"When nameSuffix is nonempty, append '-$nameSuffix' to the names of Gloo RBAC resources; e.g. when nameSuffix is 'foo', the role 'gloo-resource-reader' will become 'gloo-resource-reader-foo'"`
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

type DeploymentSpecSansResources struct {
	Replicas  int              `json:"replicas" desc:"number of instances to deploy"`
	CustomEnv []*appsv1.EnvVar `json:"customEnv,omitempty" desc:"custom extra environment variables for the container"`
}

type DeploymentSpec struct {
	DeploymentSpecSansResources
	Resources *ResourceRequirements `json:"resources,omitempty" desc:"resources for the main pod in the deployment"`
}

type Integrations struct {
	Knative *Knative `json:"knative,omitEmpty"`
}

type Knative struct {
	Enabled             *bool         `json:"enabled" desc:"enabled knative components"`
	Version             *string       `json:"version,omitEmpty" desc:"the version of knative installed to the cluster. if using version < 0.8.0, gloo will use Knative's ClusterIngress API for configuration rather than the namespace-scoped Ingress"`
	Proxy               *KnativeProxy `json:"proxy,omitempty"`
	RequireIngressClass *bool         `json:"requireIngressClass" desc:"only serve traffic for Knative Ingress objects with the annotation 'networking.knative.dev/ingress.class: gloo.ingress.networking.knative.dev'."`
}

type KnativeProxy struct {
	Image           *Image  `json:"image,omitempty"`
	HttpPort        int     `json:"httpPort,omitempty" desc:"HTTP port for the proxy"`
	HttpsPort       int     `json:"httpsPort,omitempty" desc:"HTTPS port for the proxy"`
	Tracing         *string `json:"tracing,omitempty" desc:"tracing configuration"`
	LoopBackAddress string  `json:"loopBackAddress,omitempty" desc:"Name on which to bind the loop-back interface for this instance of Envoy. Defaults to 127.0.0.1, but other common values may be localhost or ::1"`
	*DeploymentSpec
	*ServiceSpec
}

type Settings struct {
	WatchNamespaces               []string             `json:"watchNamespaces,omitempty" desc:"whitelist of namespaces for gloo to watch for services and CRDs. Empty list means all namespaces"`
	WriteNamespace                string               `json:"writeNamespace,omitempty" desc:"namespace where intermediary CRDs will be written to, e.g. Upstreams written by Gloo Discovery."`
	Integrations                  *Integrations        `json:"integrations,omitempty"`
	Create                        bool                 `json:"create" desc:"create a Settings CRD which provides bootstrap configuration to Gloo controllers"`
	Extensions                    interface{}          `json:"extensions,omitempty"`
	SingleNamespace               bool                 `json:"singleNamespace" desc:"Enable to use install namespace as WatchNamespace and WriteNamespace"`
	InvalidConfigPolicy           *InvalidConfigPolicy `json:"invalidConfigPolicy,omitempty" desc:"Define policies for Gloo to handle invalid configuration"`
	Linkerd                       bool                 `json:"linkerd" desc:"Enable automatic Linkerd integration in Gloo."`
	DisableProxyGarbageCollection bool                 `json:"disableProxyGarbageCollection" desc:"Set this option to determine the state of an Envoy listener when the corresponding Gloo Proxy resource has no routes. If false (default), Gloo will propagate the state of the Proxy to Envoy, resetting the listener to a clean slate with no routes. If true, Gloo will keep serving the routes from the last applied valid configuration."`
	DisableKubernetesDestinations bool                 `json:"disableKubernetesDestinations" desc:"Gloo allows you to directly reference a Kubernetes service as a routing destination. To enable this feature, Gloo scans the cluster for Kubernetes services and creates a special type of in-memory Upstream to represent them. If the cluster contains a lot of services and you do not restrict the namespaces Gloo is watching, this can result in significant overhead. If you do not plan on using this feature, you can set this flag to true to turn it off."`
}

type InvalidConfigPolicy struct {
	ReplaceInvalidRoutes     bool   `json:"replaceInvalidRoutes,omitempty" desc:"Rather than pausing configuration updates, in the event of an invalid Route defined on a virtual service or route table, Gloo will serve the route with a predefined direct response action. This allows valid routes to be updated when other routes are invalid."`
	InvalidRouteResponseCode int64  `json:"invalidRouteResponseCode,omitempty" desc:"the response code for the direct response"`
	InvalidRouteResponseBody string `json:"invalidRouteResponseBody,omitempty" desc:"the response body for the direct response"`
}

type Gloo struct {
	Deployment *GlooDeployment `json:"deployment,omitempty"`
}

type GlooDeployment struct {
	Image                  *Image  `json:"image,omitempty"`
	XdsPort                int     `json:"xdsPort,omitempty" desc:"port where gloo serves xDS API to Envoy"`
	ValidationPort         int     `json:"validationPort,omitempty" desc:"port where gloo serves gRPC Proxy Validation to Gateway"`
	Stats                  *Stats  `json:"stats,omitempty" desc:"overrides for prometheus stats published by the gloo pod"`
	FloatingUserId         bool    `json:"floatingUserId" desc:"set to true to allow the cluster to dynamically assign a user ID"`
	RunAsUser              float64 `json:"runAsUser" desc:"Explicitly set the user ID for the container to run as. Default is 10101"`
	ExternalTrafficPolicy  string  `json:"externalTrafficPolicy,omitempty" desc:"Set the external traffic policy on the gloo service"`
	DisableUsageStatistics bool    `json:"disableUsageStatistics" desc:"Disable the collection of gloo usage statistics"`
	*DeploymentSpec
}

type Discovery struct {
	Deployment *DiscoveryDeployment `json:"deployment,omitempty"`
	FdsMode    string               `json:"fdsMode" desc:"mode for function discovery (blacklist or whitelist). See more info in the settings docs"`
	Enabled    *bool                `json:"enabled" desc:"enable Discovery features"`
}

type DiscoveryDeployment struct {
	Image          *Image  `json:"image,omitempty"`
	Stats          *Stats  `json:"stats,omitempty" desc:"overrides for prometheus stats published by the discovery pod"`
	FloatingUserId bool    `json:"floatingUserId" desc:"set to true to allow the cluster to dynamically assign a user ID"`
	RunAsUser      float64 `json:"runAsUser" desc:"Explicitly set the user ID for the container to run as. Default is 10101"`
	*DeploymentSpec
}

type Gateway struct {
	Enabled                       *bool              `json:"enabled" desc:"enable Gloo API Gateway features"`
	Validation                    *GatewayValidation `json:"validation" desc:"enable Validation Webhook on the Gateway. This will cause requests to modify Gateway-related Custom Resources to be validated by the Gateway."`
	Deployment                    *GatewayDeployment `json:"deployment,omitempty"`
	CertGenJob                    *CertGenJob        `json:"certGenJob,omitempty" desc:"generate self-signed certs with this job to be used with the gateway validation webhook. this job will only run if validation is enabled for the gateway"`
	UpdateValues                  bool               `json:"updateValues" desc:"if true, will use a provided helm helper 'gloo.updatevalues' to update values during template render - useful for plugins/extensions"`
	ProxyServiceAccount           ServiceAccount     `json:"proxyServiceAccount" `
	ReadGatewaysFromAllNamespaces bool               `json:"readGatewaysFromAllNamespaces" desc:"if true, read Gateway custom resources from all watched namespaces rather than just the namespace of the Gateway controller"`
}

type ServiceAccount struct {
	DisableAutomount bool `json:"disableAutomount" desc:"disable automunting the service account to the gateway proxy. not mounting the token hardens the proxy container, but may interfere with service mesh integrations"`
}

type GatewayValidation struct {
	Enabled               bool     `json:"enabled" desc:"enable Gloo API Gateway validation hook (default true)"`
	AlwaysAcceptResources bool     `json:"alwaysAcceptResources" desc:"unless this is set this to false in order to ensure validation webhook rejects invalid resources. by default, validation webhook will only log and report metrics for invalid resource admission without rejecting them outright."`
	AllowWarnings         bool     `json:"allowWarnings" desc:"set this to false in order to ensure validation webhook rejects resources that would have warning status or rejected status, rather than just rejected."`
	SecretName            string   `json:"secretName" desc:"Name of the Kubernetes Secret containing TLS certificates used by the validation webhook server. This secret will be created by the certGen Job if the certGen Job is enabled."`
	FailurePolicy         string   `json:"failurePolicy" desc:"failurePolicy defines how unrecognized errors from the Gateway validation endpoint are handled - allowed values are 'Ignore' or 'Fail'. Defaults to Ignore "`
	Webhook               *Webhook `json:"webhook" desc:"webhook specific configuration"`
}

type Webhook struct {
	Enabled bool `json:"enabled" desc:"enable validation webhook (default true)"`
}

type GatewayDeployment struct {
	Image          *Image  `json:"image,omitempty"`
	Stats          *Stats  `json:"stats,omitempty" desc:"overrides for prometheus stats published by the gateway pod"`
	FloatingUserId bool    `json:"floatingUserId" desc:"set to true to allow the cluster to dynamically assign a user ID"`
	RunAsUser      float64 `json:"runAsUser" desc:"Explicitly set the user ID for the container to run as. Default is 10101"`
	*DeploymentSpec
}

type Job struct {
	Image *Image `json:"image,omitempty"`
	*JobSpec
}

type CertGenJob struct {
	Job
	Enabled                 bool    `json:"enabled" desc:"enable the job that generates the certificates for the validating webhook at install time (default true)"`
	SetTtlAfterFinished     bool    `json:"setTtlAfterFinished" desc:"Set ttlSecondsAfterFinished (a k8s feature in Alpha) on the job. Defaults to true"`
	TtlSecondsAfterFinished int     `json:"ttlSecondsAfterFinished" desc:"Clean up the finished job after this many seconds. Defaults to 60"`
	FloatingUserId          bool    `json:"floatingUserId" desc:"set to true to allow the cluster to dynamically assign a user ID"`
	RunAsUser               float64 `json:"runAsUser" desc:"Explicitly set the user ID for the container to run as. Default is 10101"`
}

type GatewayProxy struct {
	Kind                        *GatewayProxyKind            `json:"kind,omitempty" desc:"value to determine how the gateway proxy is deployed"`
	PodTemplate                 *GatewayProxyPodTemplate     `json:"podTemplate,omitempty"`
	ConfigMap                   *GatewayProxyConfigMap       `json:"configMap,omitempty"`
	Service                     *GatewayProxyService         `json:"service,omitempty"`
	AntiAffinity                bool                         `json:"antiAffinity" desc:"configure anti affinity such that pods are prefferably not co-located"`
	Tracing                     *Tracing                     `json:"tracing,omitempty"`
	GatewaySettings             *GatewayProxyGatewaySettings `json:"gatewaySettings,omitempty" desc:"settings for the helm generated gateways, leave nil to not render"`
	ExtraEnvoyArgs              []string                     `json:"extraEnvoyArgs,omitempty" desc:"envoy container args, (e.g. https://www.envoyproxy.io/docs/envoy/latest/operations/cli)"`
	ExtraContainersHelper       string                       `json:"extraContainersHelper,omitempty"`
	ExtraInitContainersHelper   string                       `json:"extraInitContainersHelper",omitempty`
	ExtraVolumeHelper           string                       `json:"extraVolumeHelper",omitempty`
	ExtraListenersHelper        string                       `json:"extraListenersHelper",omitempty`
	Stats                       *Stats                       `json:"stats,omitempty" desc:"overrides for prometheus stats published by the gateway-proxy pod"`
	ReadConfig                  bool                         `json:"readConfig" desc:"expose a read-only subset of the envoy admin api"`
	ExtraProxyVolumeMountHelper string                       `json:"extraProxyVolumeMountHelper,omitempty" desc:"name of custom made named template allowing for extra volume mounts on the proxy container"`
	LoopBackAddress             string                       `json:"loopBackAddress,omitempty" desc:"Name on which to bind the loop-back interface for this instance of Envoy. Defaults to 127.0.0.1, but other common values may be localhost or ::1"`
}

type GatewayProxyGatewaySettings struct {
	DisableGeneratedGateways bool              `json:"disableGeneratedGateways" desc:"set to true to disable the gateway generation for a gateway proxy"`
	IPv4Only                 bool              `json:"ipv4Only,omitempty" desc:"set to true if your network allows ipv4 addresses only. Sets the Gateway spec's bindAddress to 0.0.0.0 instead of ::"`
	UseProxyProto            bool              `json:"useProxyProto" desc:"use proxy protocol"`
	CustomHttpGateway        string            `json:"customHttpGateway,omitempty" desc:"custom yaml to use for http gateway settings"`
	CustomHttpsGateway       string            `json:"customHttpsGateway,omitempty" desc:"custom yaml to use for https gateway settings"`
	GatewayOptions           v1.GatewayOptions `json:"options,omitempty" desc:"custom options for http(s) gateways"`
}

type GatewayProxyKind struct {
	Deployment *GatewayProxyDeployment `json:"deployment,omitempty" desc:"set to deploy as a kubernetes deployment, otherwise nil"`
	DaemonSet  *DaemonSetSpec          `json:"daemonSet,omitempty" desc:"set to deploy as a kubernetes daemonset, otherwise nil"`
}
type GatewayProxyDeployment struct {
	*DeploymentSpecSansResources
}

type DaemonSetSpec struct {
	HostPort bool `json:"hostPort" desc:"whether or not to enable host networking on the pod. Only relevant when running as a DaemonSet"`
}

type GatewayProxyPodTemplate struct {
	Image            *Image                `json:"image,omitempty"`
	HttpPort         int                   `json:"httpPort,omitempty" desc:"HTTP port for the gateway service target port"`
	HttpsPort        int                   `json:"httpsPort,omitempty" desc:"HTTPS port for the gateway service target port"`
	ExtraPorts       []interface{}         `json:"extraPorts,omitempty" desc:"extra ports for the gateway pod"`
	ExtraAnnotations map[string]string     `json:"extraAnnotations,omitempty" desc:"extra annotations to add to the pod"`
	NodeName         string                `json:"nodeName,omitempty" desc:"name of node to run on"`
	NodeSelector     map[string]string     `json:"nodeSelector,omitempty" desc:"label selector for nodes"`
	Tolerations      []*appsv1.Toleration  `json:"tolerations,omitEmpty"`
	Probes           bool                  `json:"probes" desc:"enable liveness and readiness probes"`
	Resources        *ResourceRequirements `json:"resources,omitempty"`
	DisableNetBind   bool                  `json:"disableNetBind" desc:"don't add the NET_BIND_SERVICE capability to the pod. This means that the gateway proxy will not be able to bind to ports below 1024"`
	RunUnprivileged  bool                  `json:"runUnprivileged" desc:"run envoy as an unprivileged user"`
	FloatingUserId   bool                  `json:"floatingUserId" desc:"set to true to allow the cluster to dynamically assign a user ID"`
	RunAsUser        float64               `json:"runAsUser" desc:"Explicitly set the user ID for the container to run as. Default is 10101"`
	FsGroup          float64               `json:"fsGroup" desc:"Explicitly set the group ID for volume ownership. Default is 10101"`
}

type GatewayProxyService struct {
	Type                     string            "json:\"type,omitempty\" desc:\"gateway [service type](https://kubernetes.io/docs/concepts/services-networking/service/#publishing-services-service-types). default is `LoadBalancer`\""
	HttpPort                 int               `json:"httpPort,omitempty" desc:"HTTP port for the gateway service"`
	HttpsPort                int               `json:"httpsPort,omitempty" desc:"HTTPS port for the gateway service"`
	HttpNodePort             int               `json:"httpNodePort,omitempty" desc:"HTTP nodeport for the gateway service if using type NodePort"`
	HttpsNodePort            int               `json:"httpsNodePort,omitempty" desc:"HTTPS nodeport for the gateway service if using type NodePort"`
	ClusterIP                string            "json:\"clusterIP,omitempty\" desc:\"static clusterIP (or `None`) when `gatewayProxies[].gatewayProxy.service.type` is `ClusterIP`\""
	ExtraAnnotations         map[string]string `json:"extraAnnotations,omitempty"`
	ExternalTrafficPolicy    string            `json:"externalTrafficPolicy,omitempty"`
	Name                     string            `json:"name,omitempty" desc:"Custom name override for the service resource of the proxy"`
	HttpsFirst               bool              `json:"httpsFirst" desc:"List HTTPS port before HTTP"`
	LoadBalancerIP           string            `json:"loadBalancerIP,omitempty" desc:"IP address of the load balancer"`
	LoadBalancerSourceRanges []string          `json:"loadBalancerSourceRanges,omitempty" desc:"List of IP CIDR ranges that are allowed to access the load balancer"`
}

type Tracing struct {
	Provider string `json:"provider,omitempty"`
	Cluster  string `json:"cluster,omitempty"`
}

type AccessLogger struct {
	Image       *Image `json:"image,omitempty"`
	Port        uint   `json:"port,omitempty"`
	ServiceName string `json:"serviceName,omitempty"`
	Enabled     bool   `json:"enabled"`
	Stats       *Stats `json:"stats,omitempty" desc:"overrides for prometheus stats published by the gloo pod"`
	*DeploymentSpec
}

type GatewayProxyConfigMap struct {
	Data map[string]string `json:"data"`
}

type Ingress struct {
	Enabled             *bool              `json:"enabled"`
	Deployment          *IngressDeployment `json:"deployment,omitempty"`
	RequireIngressClass *bool              `json:"requireIngressClass" desc:"only serve traffic for Ingress objects with the Ingress Class annotation 'kubernetes.io/ingress.class'. By default the annotation value must be set to 'gloo', however this can be overriden via customIngressClass."`
	CustomIngress       *bool              `json:"customIngressClass" desc:"Only relevant when requireIngressClass is set to true. Setting this value will cause the Gloo Ingress Controller to process only those Ingress objects which have their ingress class set to this value (e.g. 'kubernetes.io/ingress.class=SOMEVALUE')."`
}

type IngressDeployment struct {
	Image *Image `json:"image,omitempty"`
	*DeploymentSpec
}

type IngressProxy struct {
	Deployment      *IngressProxyDeployment `json:"deployment,omitempty"`
	ConfigMap       *IngressProxyConfigMap  `json:"configMap,omitempty"`
	Tracing         *string                 `json:"tracing,omitempty"`
	LoopBackAddress string                  `json:"loopBackAddress,omitempty" desc:"Name on which to bind the loop-back interface for this instance of Envoy. Defaults to 127.0.0.1, but other common values may be localhost or ::1"`
	*ServiceSpec
}

type IngressProxyDeployment struct {
	Image            *Image            `json:"image,omitempty"`
	HttpPort         int               `json:"httpPort,omitempty" desc:"HTTP port for the ingress container"`
	HttpsPort        int               `json:"httpsPort,omitempty" desc:"HTTPS port for the ingress container"`
	ExtraPorts       []interface{}     `json:"extraPorts,omitempty"`
	ExtraAnnotations map[string]string `json:"extraAnnotations,omitempty"`
	*DeploymentSpec
}

type ServiceSpec struct {
	Service *Service `json:"service,omitempty" desc:"K8s service configuration"`
}

type Service struct {
	Type             *string           `json:"type,omitempty" desc:"K8s service type"`
	ExtraAnnotations map[string]string `json:"extraAnnotations,omitempty" desc:"extra annotations to add to the service"`
	LoadBalancerIP   string            `json:"loadBalancerIP,omitempty" desc:"IP address of the load balancer"`
}

type IngressProxyConfigMap struct {
	Data map[string]string `json:"data,omitempty"`
}

type K8s struct {
	ClusterName string `json:"clusterName" desc:"cluster name to use when referencing services."`
}

type Wasm struct {
	Enabled bool `json:"enabled" desc:"switch the gateway-proxy image to one which supports WASM"`
}

type Stats struct {
	Enabled bool `json:"enabled,omitempty" desc:"Controls whether or not prometheus stats are enabled"`
}

type Mtls struct {
	Enabled      bool                  `json:"enabled" desc:"Enables internal mtls authentication"`
	Sds          SdsContainer          `json:"sds,omitempty"`
	EnvoySidecar EnvoySidecarContainer `json:"envoy,omitempty"`
}

type SdsContainer struct {
	Image *Image `json:"image,omitempty"`
}

type EnvoySidecarContainer struct {
	Image *Image `json:"image,omitempty"`
}
