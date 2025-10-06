package deployer

import (
	appsv1 "k8s.io/api/apps/v1"
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
	GatewayClassName *string `json:"gatewayClassName,omitempty"`
	NameOverride     *string `json:"nameOverride,omitempty"`
	FullnameOverride *string `json:"fullnameOverride,omitempty"`

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
	GracefulShutdown              *v1alpha1.GracefulShutdownSpec    `json:"gracefulShutdown,omitempty"`
	TerminationGracePeriodSeconds *int64                            `json:"terminationGracePeriodSeconds,omitempty"`
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
	// Note: ideally, these should be mapped to container specific values, but right now they
	// map the the proxy container
	Image             *HelmImage                   `json:"image,omitempty"`
	Resources         *corev1.ResourceRequirements `json:"resources,omitempty"`
	SecurityContext   *corev1.SecurityContext      `json:"securityContext,omitempty"`
	Env               []corev1.EnvVar              `json:"env,omitempty"`
	ExtraVolumeMounts []corev1.VolumeMount         `json:"extraVolumeMounts,omitempty"`

	// xds values
	Xds *HelmXds `json:"xds,omitempty"`
	// agentgateway xds values
	AgwXds *HelmXds `json:"agwXds,omitempty"`

	// stats values
	Stats *HelmStatsConfig `json:"stats,omitempty"`

	// AI extension values
	// Deprecated: Envoy-based AI gateway is deprecated in v2.1 and will be removed in v2.2.
	AIExtension *HelmAIExtension `json:"aiExtension,omitempty"`

	// agentgateway integration values
	Agentgateway *HelmAgentgateway `json:"agentgateway,omitempty"`
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

type HelmAgentgateway struct {
	Enabled             bool   `json:"enabled,omitempty"`
	LogLevel            string `json:"logLevel,omitempty"`
	CustomConfigMapName string `json:"customConfigMapName,omitempty"`
}
