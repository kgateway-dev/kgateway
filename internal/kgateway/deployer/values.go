package deployer

import (
	corev1 "k8s.io/api/core/v1"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1"
)

// helmConfig stores the top-level helm values used by the deployer.
type helmConfig struct {
	Gateway            *helmGateway            `json:"gateway,omitempty"`
	InferenceExtension *helmInferenceExtension `json:"inferenceExtension,omitempty"`
}

type helmGateway struct {
	// naming
	Name             *string `json:"name,omitempty"`
	GatewayName      *string `json:"gatewayName,omitempty"`
	GatewayNamespace *string `json:"gatewayNamespace,omitempty"`
	NameOverride     *string `json:"nameOverride,omitempty"`
	FullnameOverride *string `json:"fullnameOverride,omitempty"`

	// deployment/service values
	ReplicaCount   *uint32          `json:"replicaCount,omitempty"`
	Autoscaling    *helmAutoscaling `json:"autoscaling,omitempty"`
	Ports          []helmPort       `json:"ports,omitempty"`
	Service        *helmService     `json:"service,omitempty"`
	FloatingUserId *bool            `json:"floatingUserId,omitempty"`

	// serviceaccount values
	ServiceAccount *helmServiceAccount `json:"serviceAccount,omitempty"`

	// pod template values
	ExtraPodAnnotations           map[string]string              `json:"extraPodAnnotations,omitempty"`
	ExtraPodLabels                map[string]string              `json:"extraPodLabels,omitempty"`
	ImagePullSecrets              []corev1.LocalObjectReference  `json:"imagePullSecrets,omitempty"`
	PodSecurityContext            *corev1.PodSecurityContext     `json:"podSecurityContext,omitempty"`
	NodeSelector                  map[string]string              `json:"nodeSelector,omitempty"`
	Affinity                      *corev1.Affinity               `json:"affinity,omitempty"`
	Tolerations                   []corev1.Toleration            `json:"tolerations,omitempty"`
	ReadinessProbe                *corev1.Probe                  `json:"readinessProbe,omitempty"`
	LivenessProbe                 *corev1.Probe                  `json:"livenessProbe,omitempty"`
	GracefulShutdown              *v1alpha1.GracefulShutdownSpec `json:"gracefulShutdown,omitempty"`
	TerminationGracePeriodSeconds *int                           `json:"terminationGracePeriodSeconds,omitempty"`

	// sds container values
	SdsContainer *helmSdsContainer `json:"sdsContainer,omitempty"`
	// istio container values
	IstioContainer *helmIstioContainer `json:"istioContainer,omitempty"`
	// istio integration values
	Istio *helmIstio `json:"istio,omitempty"`

	// envoy container values
	LogLevel          *string                      `json:"logLevel,omitempty"`
	ComponentLogLevel *string                      `json:"componentLogLevel,omitempty"`
	Image             *helmImage                   `json:"image,omitempty"`
	Resources         *corev1.ResourceRequirements `json:"resources,omitempty"`
	SecurityContext   *corev1.SecurityContext      `json:"securityContext,omitempty"`

	// xds values
	Xds *helmXds `json:"xds,omitempty"`

	// stats values
	Stats *helmStatsConfig `json:"stats,omitempty"`

	// AI extension values
	AIExtension *helmAIExtension `json:"aiExtension,omitempty"`

	// agentgateway integration values
	AgentGateway *helmAgentGateway `json:"agentGateway,omitempty"`
}

// helmPort represents a Gateway Listener port
type helmPort struct {
	Port       *uint16 `json:"port,omitempty"`
	Protocol   *string `json:"protocol,omitempty"`
	Name       *string `json:"name,omitempty"`
	TargetPort *uint16 `json:"targetPort,omitempty"`
	NodePort   *uint16 `json:"nodePort,omitempty"`
}

type helmImage struct {
	Registry   *string `json:"registry,omitempty"`
	Repository *string `json:"repository,omitempty"`
	Tag        *string `json:"tag,omitempty"`
	Digest     *string `json:"digest,omitempty"`
	PullPolicy *string `json:"pullPolicy,omitempty"`
}

type helmService struct {
	Type             *string           `json:"type,omitempty"`
	ClusterIP        *string           `json:"clusterIP,omitempty"`
	ExtraAnnotations map[string]string `json:"extraAnnotations,omitempty"`
	ExtraLabels      map[string]string `json:"extraLabels,omitempty"`
}

type helmServiceAccount struct {
	ExtraAnnotations map[string]string `json:"extraAnnotations,omitempty"`
	ExtraLabels      map[string]string `json:"extraLabels,omitempty"`
}

// helmXds represents the xds host and port to which envoy will connect
// to receive xds config updates
type helmXds struct {
	Host *string `json:"host,omitempty"`
	Port *uint32 `json:"port,omitempty"`
}

type helmAutoscaling struct {
	Enabled                           *bool   `json:"enabled,omitempty"`
	MinReplicas                       *uint32 `json:"minReplicas,omitempty"`
	MaxReplicas                       *uint32 `json:"maxReplicas,omitempty"`
	TargetCPUUtilizationPercentage    *uint32 `json:"targetCPUUtilizationPercentage,omitempty"`
	TargetMemoryUtilizationPercentage *uint32 `json:"targetMemoryUtilizationPercentage,omitempty"`
}

type helmIstio struct {
	Enabled *bool `json:"enabled,omitempty"`
}

type helmSdsContainer struct {
	Image           *helmImage                   `json:"image,omitempty"`
	Resources       *corev1.ResourceRequirements `json:"resources,omitempty"`
	SecurityContext *corev1.SecurityContext      `json:"securityContext,omitempty"`
	SdsBootstrap    *sdsBootstrap                `json:"sdsBootstrap,omitempty"`
}

type sdsBootstrap struct {
	LogLevel *string `json:"logLevel,omitempty"`
}

type helmIstioContainer struct {
	Image    *helmImage `json:"image,omitempty"`
	LogLevel *string    `json:"logLevel,omitempty"`

	Resources       *corev1.ResourceRequirements `json:"resources,omitempty"`
	SecurityContext *corev1.SecurityContext      `json:"securityContext,omitempty"`

	IstioDiscoveryAddress *string `json:"istioDiscoveryAddress,omitempty"`
	IstioMetaMeshId       *string `json:"istioMetaMeshId,omitempty"`
	IstioMetaClusterId    *string `json:"istioMetaClusterId,omitempty"`
}

type helmStatsConfig struct {
	Enabled            *bool   `json:"enabled,omitempty"`
	RoutePrefixRewrite *string `json:"routePrefixRewrite,omitempty"`
	EnableStatsRoute   *bool   `json:"enableStatsRoute,omitempty"`
	StatsPrefixRewrite *string `json:"statsPrefixRewrite,omitempty"`
}

type helmAIExtension struct {
	Enabled         bool                         `json:"enabled,omitempty"`
	Image           *helmImage                   `json:"image,omitempty"`
	SecurityContext *corev1.SecurityContext      `json:"securityContext,omitempty"`
	Resources       *corev1.ResourceRequirements `json:"resources,omitempty"`
	Env             []corev1.EnvVar              `json:"env,omitempty"`
	Ports           []corev1.ContainerPort       `json:"ports,omitempty"`
	Stats           []byte                       `json:"stats,omitempty"`
}

type helmInferenceExtension struct {
	EndpointPicker *helmEndpointPickerExtension `json:"endpointPicker,omitempty"`
}

type helmEndpointPickerExtension struct {
	PoolName      string `json:"poolName"`
	PoolNamespace string `json:"poolNamespace"`
}

type helmAgentGateway struct {
	Enabled  bool   `json:"enabled,omitempty"`
	LogLevel string `json:"logLevel,omitempty"`
}
