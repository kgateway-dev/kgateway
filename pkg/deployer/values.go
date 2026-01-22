package deployer

import (
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/agentgateway"
	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/kgateway"
)

type DataPlaneType string

const (
	DataPlaneAgentgateway DataPlaneType = "agentgateway"
	DataPlaneEnvoy        DataPlaneType = "envoy"
)

// helmConfig stores the top-level helm values used by the deployer.
type HelmConfig struct {
	Gateway            *HelmGateway             `json:"gateway,omitempty"`
	Agentgateway       *AgentgatewayHelmGateway `json:"agentgateway,omitempty"`
	InferenceExtension *HelmInferenceExtension  `json:"inferenceExtension,omitempty"`
}

type HelmGateway struct {
	// not needed by the helm charts, but by the code that select the correct
	// helm chart:
	DataPlaneType DataPlaneType `json:"dataPlaneType"`

	// naming
	Name               *string           `json:"name,omitempty"`
	GatewayName        *string           `json:"gatewayName,omitempty"`
	GatewayNamespace   *string           `json:"gatewayNamespace,omitempty"`
	GatewayClassName   *string           `json:"gatewayClassName,omitempty"`
	GatewayAnnotations map[string]string `json:"gatewayAnnotations,omitempty"`
	GatewayLabels      map[string]string `json:"gatewayLabels,omitempty"`
	NameOverride       *string           `json:"nameOverride,omitempty"`
	FullnameOverride   *string           `json:"fullnameOverride,omitempty"`

	// deployment/service values
	ReplicaCount *uint32                    `json:"replicaCount,omitempty"`
	Ports        []HelmPort                 `json:"ports,omitempty"`
	Service      *HelmService               `json:"service,omitempty"`
	Strategy     *appsv1.DeploymentStrategy `json:"strategy,omitempty"`

	// serviceaccount values
	ServiceAccount *HelmServiceAccount `json:"serviceAccount,omitempty"`

	// pod template values
	ExtraPodAnnotations           map[string]string                 `json:"extraPodAnnotations,omitempty"`
	ExtraPodLabels                map[string]string                 `json:"extraPodLabels,omitempty"`
	ImagePullSecrets              []corev1.LocalObjectReference     `json:"imagePullSecrets,omitempty"`
	PodSecurityContext            *corev1.PodSecurityContext        `json:"podSecurityContext,omitempty"`
	NodeSelector                  map[string]string                 `json:"nodeSelector,omitempty"`
	Affinity                      *corev1.Affinity                  `json:"affinity,omitempty"`
	Tolerations                   []corev1.Toleration               `json:"tolerations,omitempty"`
	StartupProbe                  *corev1.Probe                     `json:"startupProbe,omitempty"`
	ReadinessProbe                *corev1.Probe                     `json:"readinessProbe,omitempty"`
	LivenessProbe                 *corev1.Probe                     `json:"livenessProbe,omitempty"`
	ExtraVolumes                  []corev1.Volume                   `json:"extraVolumes,omitempty"`
	GracefulShutdown              *kgateway.GracefulShutdownSpec    `json:"gracefulShutdown,omitempty"`
	TerminationGracePeriodSeconds *int64                            `json:"terminationGracePeriodSeconds,omitempty"`
	TopologySpreadConstraints     []corev1.TopologySpreadConstraint `json:"topologySpreadConstraints,omitempty"`
	PriorityClassName             *string                           `json:"priorityClassName,omitempty"`

	// sds container values
	SdsContainer *HelmSdsContainer `json:"sdsContainer,omitempty"`
	// istio container values
	IstioContainer *HelmIstioContainer `json:"istioContainer,omitempty"`
	// istio integration values
	Istio *HelmIstio `json:"istio,omitempty"`

	// envoy container values
	ComponentLogLevel *string `json:"componentLogLevel,omitempty"`

	// envoy or agentgateway container values
	// Note: ideally, these should be mapped to container specific values, but right now they
	// map to the proxy container
	LogLevel          *string                      `json:"logLevel,omitempty"`
	Image             *HelmImage                   `json:"image,omitempty"`
	Resources         *corev1.ResourceRequirements `json:"resources,omitempty"`
	SecurityContext   *corev1.SecurityContext      `json:"securityContext,omitempty"`
	Env               []corev1.EnvVar              `json:"env,omitempty"`
	ExtraVolumeMounts []corev1.VolumeMount         `json:"extraVolumeMounts,omitempty"`

	// envoy bootstrap values
	DnsResolver *HelmDnsResolver `json:"dnsResolver,omitempty"`

	// xds values
	Xds *HelmXds `json:"xds,omitempty"`

	// stats values
	Stats *HelmStatsConfig `json:"stats,omitempty"`

	// LogFormat specifies the logging format for agentgateway (Json or Text)
	LogFormat *string `json:"logFormat,omitempty"`
	// RawConfig provides opaque config to be merged into config.yaml
	RawConfig map[string]any `json:"rawConfig,omitempty"`
}

// helmPort represents a Gateway Listener port
type HelmPort struct {
	Port       *int32  `json:"port,omitempty"`
	Protocol   *string `json:"protocol,omitempty"`
	Name       *string `json:"name,omitempty"`
	TargetPort *int32  `json:"targetPort,omitempty"`
	NodePort   *int32  `json:"nodePort,omitempty"`
}

type HelmImage struct {
	Registry   *string `json:"registry,omitempty"`
	Repository *string `json:"repository,omitempty"`
	Tag        *string `json:"tag,omitempty"`
	Digest     *string `json:"digest,omitempty"`
	PullPolicy *string `json:"pullPolicy,omitempty"`
}

type HelmService struct {
	Type                  *string           `json:"type,omitempty"`
	ClusterIP             *string           `json:"clusterIP,omitempty"`
	LoadBalancerClass     *string           `json:"loadBalancerClass,omitempty"`
	LoadBalancerIP        *string           `json:"loadBalancerIP,omitempty"`
	ExtraAnnotations      map[string]string `json:"extraAnnotations,omitempty"`
	ExtraLabels           map[string]string `json:"extraLabels,omitempty"`
	ExternalTrafficPolicy *string           `json:"externalTrafficPolicy,omitempty"`
}

type HelmServiceAccount struct {
	ExtraAnnotations map[string]string `json:"extraAnnotations,omitempty"`
	ExtraLabels      map[string]string `json:"extraLabels,omitempty"`
}

// helmXds represents the xds host and port to which envoy will connect
// to receive xds config updates
type HelmXds struct {
	Host *string     `json:"host,omitempty"`
	Port *uint32     `json:"port,omitempty"`
	Tls  *HelmXdsTls `json:"tls,omitempty"`
}

type HelmXdsTls struct {
	Enabled *bool   `json:"enabled,omitempty"`
	CaCert  *string `json:"caCert,omitempty"`
}

type HelmDnsResolver struct {
	UdpMaxQueries *int32 `json:"udpMaxQueries,omitempty"`
}

type HelmIstio struct {
	Enabled *bool `json:"enabled,omitempty"`
}

type HelmSdsContainer struct {
	Image           *HelmImage                   `json:"image,omitempty"`
	Resources       *corev1.ResourceRequirements `json:"resources,omitempty"`
	SecurityContext *corev1.SecurityContext      `json:"securityContext,omitempty"`
	SdsBootstrap    *SdsBootstrap                `json:"sdsBootstrap,omitempty"`
}

type SdsBootstrap struct {
	LogLevel *string `json:"logLevel,omitempty"`
}

type HelmIstioContainer struct {
	Image    *HelmImage `json:"image,omitempty"`
	LogLevel *string    `json:"logLevel,omitempty"`

	Resources       *corev1.ResourceRequirements `json:"resources,omitempty"`
	SecurityContext *corev1.SecurityContext      `json:"securityContext,omitempty"`

	IstioDiscoveryAddress *string `json:"istioDiscoveryAddress,omitempty"`
	IstioMetaMeshId       *string `json:"istioMetaMeshId,omitempty"`
	IstioMetaClusterId    *string `json:"istioMetaClusterId,omitempty"`
}

type HelmStatsConfig struct {
	Enabled            *bool             `json:"enabled,omitempty"`
	RoutePrefixRewrite *string           `json:"routePrefixRewrite,omitempty"`
	EnableStatsRoute   *bool             `json:"enableStatsRoute,omitempty"`
	StatsPrefixRewrite *string           `json:"statsPrefixRewrite,omitempty"`
	Matcher            *HelmStatsMatcher `json:"matcher,omitempty"`
}

// HelmStatsMatcher represents mutually exclusive inclusion or exclusion lists for Envoy stats.
type HelmStatsMatcher struct {
	InclusionList []HelmStringMatcher `json:"inclusionList,omitempty"`
	ExclusionList []HelmStringMatcher `json:"exclusionList,omitempty"`
}

// HelmStringMatcher mirrors a subset of Envoy's StringMatcher.
// Only one of these fields should be set per matcher.
type HelmStringMatcher struct {
	Exact      *string `json:"exact,omitempty"`
	Prefix     *string `json:"prefix,omitempty"`
	Suffix     *string `json:"suffix,omitempty"`
	Contains   *string `json:"contains,omitempty"`
	SafeRegex  *string `json:"safeRegex,omitempty"`
	IgnoreCase *bool   `json:"ignoreCase,omitempty"`
}

type HelmInferenceExtension struct {
	EndpointPicker *HelmEndpointPickerExtension `json:"endpointPicker,omitempty"`
}

type HelmEndpointPickerExtension struct {
	PoolName      string `json:"poolName"`
	PoolNamespace string `json:"poolNamespace"`
}

type AgentgatewayHelmService struct {
	LoadBalancerIP *string `json:"loadBalancerIP,omitempty"`
}

type AgentgatewayHelmGateway struct {
	agentgateway.AgentgatewayParametersConfigs `json:",inline"`
	// naming
	Name               *string           `json:"name,omitempty"`
	GatewayClassName   *string           `json:"gatewayClassName,omitempty"`
	GatewayAnnotations map[string]string `json:"gatewayAnnotations,omitempty"`
	GatewayLabels      map[string]string `json:"gatewayLabels,omitempty"`

	// deployment/service values
	Ports   []HelmPort               `json:"ports,omitempty"`
	Service *AgentgatewayHelmService `json:"service,omitempty"`

	// agentgateway xds values
	Xds *HelmXds `json:"xds,omitempty"`
}
