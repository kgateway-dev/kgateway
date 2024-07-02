package deployer

import (
	extcorev1 "github.com/solo-io/gloo/projects/gateway2/pkg/api/external/kubernetes/api/core/v1"
	v1alpha1kube "github.com/solo-io/gloo/projects/gateway2/pkg/api/gateway.gloo.solo.io/v1alpha1/kube"
)

// The top-level helm values used by the deployer.
type helmConfig struct {
	Gateway *helmGateway `json:"gateway,omitempty"`
}

type helmGateway struct {
	// naming
	Name             *string `json:"name,omitempty"`
	GatewayName      *string `json:"gatewayName,omitempty"`
	GatewayNamespace *string `json:"gatewayNamespace,omitempty"`
	NameOverride     *string `json:"nameOverride,omitempty"`
	FullnameOverride *string `json:"fullnameOverride,omitempty"`

	// deployment/service values
	ReplicaCount *uint32          `json:"replicaCount,omitempty"`
	Autoscaling  *helmAutoscaling `json:"autoscaling,omitempty"`
	Ports        []helmPort       `json:"ports,omitempty"`
	Service      *helmService     `json:"service,omitempty"`

	// pod template values
	ExtraPodAnnotations map[string]string                 `json:"extraPodAnnotations,omitempty"`
	ExtraPodLabels      map[string]string                 `json:"extraPodLabels,omitempty"`
	ImagePullSecrets    []*extcorev1.LocalObjectReference `json:"imagePullSecrets,omitempty"`
	PodSecurityContext  *extcorev1.PodSecurityContext     `json:"podSecurityContext,omitempty"`
	NodeSelector        map[string]string                 `json:"nodeSelector,omitempty"`
	Affinity            *extcorev1.Affinity               `json:"affinity,omitempty"`
	Tolerations         []*extcorev1.Toleration           `json:"tolerations,omitempty"`

	// sds container values
	SdsContainer *helmSdsContainer `json:"sdsContainer,omitempty"`
	// istio container values
	IstioContainer *helmIstioContainer `json:"istioContainer,omitempty"`
	// istio integration values
	Istio *helmIstio `json:"istio,omitempty"`

	// envoy container values
	LogLevel          *string                            `json:"logLevel,omitempty"`
	ComponentLogLevel *string                            `json:"componentLogLevel,omitempty"`
	Image             *helmImage                         `json:"image,omitempty"`
	Resources         *v1alpha1kube.ResourceRequirements `json:"resources,omitempty"`
	SecurityContext   *extcorev1.SecurityContext         `json:"securityContext,omitempty"`

	// xds values
	Xds *helmXds `json:"xds,omitempty"`

	// stats values
	Stats *helmStatsConfig `json:"stats,omitempty"`

	// AI extension values
	AIExtension *helmAIExtension `json:"aiExtension,omitempty"`
}

// helmPort represents a Gateway Listener port
type helmPort struct {
	Port       *uint16 `json:"port,omitempty"`
	Protocol   *string `json:"protocol,omitempty"`
	Name       *string `json:"name,omitempty"`
	TargetPort *uint16 `json:"targetPort,omitempty"`
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

// helmXds represents the xds host and port to which envoy will connect
// to receive xds config updates
type helmXds struct {
	Host *string `json:"host,omitempty"`
	Port *int32  `json:"port,omitempty"`
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
	Image           *helmImage                         `json:"image,omitempty"`
	Resources       *v1alpha1kube.ResourceRequirements `json:"resources,omitempty"`
	SecurityContext *extcorev1.SecurityContext         `json:"securityContext,omitempty"`
	SdsBootstrap    *sdsBootstrap                      `json:"sdsBootstrap,omitempty"`
}

type sdsBootstrap struct {
	LogLevel *string `json:"logLevel,omitempty"`
}

type helmIstioContainer struct {
	Image    *helmImage `json:"image,omitempty"`
	LogLevel *string    `json:"logLevel,omitempty"`

	Resources       *v1alpha1kube.ResourceRequirements `json:"resources,omitempty"`
	SecurityContext *extcorev1.SecurityContext         `json:"securityContext,omitempty"`

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
	Enabled         bool                               `json:"enabled,omitempty"`
	Image           *helmImage                         `json:"image,omitempty"`
	SecurityContext *extcorev1.SecurityContext         `json:"securityContext,omitempty"`
	Resources       *v1alpha1kube.ResourceRequirements `json:"resources,omitempty"`
	Env             []*extcorev1.EnvVar                `json:"env,omitempty"`
	Ports           []*extcorev1.ContainerPort         `json:"ports,omitempty"`
}
