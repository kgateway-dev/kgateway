package deployer

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1"
)

// helmConfig stores the top-level helm values used by the deployer.
type HelmConfig struct {
	Gateway            *HelmGateway            `json:"gateway,omitempty"`
	InferenceExtension *HelmInferenceExtension `json:"inferenceExtension,omitempty"`
}

type HelmGateway struct {
	// naming
	Name             *string `json:"name,omitempty"`
	GatewayName      *string `json:"gatewayName,omitempty"`
	GatewayNamespace *string `json:"gatewayNamespace,omitempty"`
	NameOverride     *string `json:"nameOverride,omitempty"`
	FullnameOverride *string `json:"fullnameOverride,omitempty"`

	// deployment/service values
	ReplicaCount   *uint32      `json:"replicaCount,omitempty"`
	Ports          []HelmPort   `json:"ports,omitempty"`
	Service        *HelmService `json:"service,omitempty"`
	FloatingUserId *bool        `json:"floatingUserId,omitempty"`

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
	ReadinessProbe                *corev1.Probe                     `json:"readinessProbe,omitempty"`
	LivenessProbe                 *corev1.Probe                     `json:"livenessProbe,omitempty"`
	GracefulShutdown              *v1alpha1.GracefulShutdownSpec    `json:"gracefulShutdown,omitempty"`
	TerminationGracePeriodSeconds *int                              `json:"terminationGracePeriodSeconds,omitempty"`
	TopologySpreadConstraints     []corev1.TopologySpreadConstraint `json:"topologySpreadConstraints,omitempty"`

	// sds container values
	SdsContainer *HelmSdsContainer `json:"sdsContainer,omitempty"`
	// istio container values
	IstioContainer *HelmIstioContainer `json:"istioContainer,omitempty"`
	// istio integration values
	Istio *HelmIstio `json:"istio,omitempty"`

	// envoy container values
	LogLevel          *string `json:"logLevel,omitempty"`
	ComponentLogLevel *string `json:"componentLogLevel,omitempty"`

	// envoy or agentgateway container values
	Image           *HelmImage                   `json:"image,omitempty"`
	Resources       *corev1.ResourceRequirements `json:"resources,omitempty"`
	SecurityContext *corev1.SecurityContext      `json:"securityContext,omitempty"`
	Env             []corev1.EnvVar              `json:"env,omitempty"`

	// xds values
	Xds *HelmXds `json:"xds,omitempty"`

	// stats values
	Stats *HelmStatsConfig `json:"stats,omitempty"`

	// AI extension values
	AIExtension *HelmAIExtension `json:"aiExtension,omitempty"`

	// agentgateway integration values
	AgentGateway *HelmAgentGateway `json:"agentGateway,omitempty"`
}

// helmPort represents a Gateway Listener port
type HelmPort struct {
	Port       *uint16 `json:"port,omitempty"`
	Protocol   *string `json:"protocol,omitempty"`
	Name       *string `json:"name,omitempty"`
	TargetPort *uint16 `json:"targetPort,omitempty"`
	NodePort   *uint16 `json:"nodePort,omitempty"`
}

type HelmImage struct {
	Registry   *string `json:"registry,omitempty"`
	Repository *string `json:"repository,omitempty"`
	Tag        *string `json:"tag,omitempty"`
	Digest     *string `json:"digest,omitempty"`
	PullPolicy *string `json:"pullPolicy,omitempty"`
}

type HelmService struct {
	Type             *string           `json:"type,omitempty"`
	ClusterIP        *string           `json:"clusterIP,omitempty"`
	ExtraAnnotations map[string]string `json:"extraAnnotations,omitempty"`
	ExtraLabels      map[string]string `json:"extraLabels,omitempty"`
}

type HelmServiceAccount struct {
	ExtraAnnotations map[string]string `json:"extraAnnotations,omitempty"`
	ExtraLabels      map[string]string `json:"extraLabels,omitempty"`
}

// helmXds represents the xds host and port to which envoy will connect
// to receive xds config updates
type HelmXds struct {
	Host *string `json:"host,omitempty"`
	Port *uint32 `json:"port,omitempty"`
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
	Enabled            *bool   `json:"enabled,omitempty"`
	RoutePrefixRewrite *string `json:"routePrefixRewrite,omitempty"`
	EnableStatsRoute   *bool   `json:"enableStatsRoute,omitempty"`
	StatsPrefixRewrite *string `json:"statsPrefixRewrite,omitempty"`
}

type HelmAIExtension struct {
	Enabled         bool                         `json:"enabled,omitempty"`
	Image           *HelmImage                   `json:"image,omitempty"`
	SecurityContext *corev1.SecurityContext      `json:"securityContext,omitempty"`
	Resources       *corev1.ResourceRequirements `json:"resources,omitempty"`
	Env             []corev1.EnvVar              `json:"env,omitempty"`
	Ports           []corev1.ContainerPort       `json:"ports,omitempty"`
	Stats           []byte                       `json:"stats,omitempty"`
	Tracing         string                       `json:"tracing,omitempty"`
}

type helmAITracing struct {
	EndPoint gwv1.AbsoluteURI      `json:"endpoint"`
	Sampler  *helmAITracingSampler `json:"sampler,omitempty"`
	Timeout  *metav1.Duration      `json:"timeout,omitempty"`
	Protocol *string               `json:"protocol,omitempty"`
}

type helmAITracingSampler struct {
	SamplerType *string `json:"type,omitempty"`
	SamplerArg  *string `json:"arg,omitempty"`
}

type HelmInferenceExtension struct {
	EndpointPicker *HelmEndpointPickerExtension `json:"endpointPicker,omitempty"`
}

type HelmEndpointPickerExtension struct {
	PoolName      string `json:"poolName"`
	PoolNamespace string `json:"poolNamespace"`
}

type HelmAgentGateway struct {
	Enabled             bool   `json:"enabled,omitempty"`
	LogLevel            string `json:"logLevel,omitempty"`
	CustomConfigMapName string `json:"customConfigMapName,omitempty"`
}
