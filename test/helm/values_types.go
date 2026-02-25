package helm

// HelmValues is a Go struct that mirrors the top-level structure of
// install/helm/kgateway/values.yaml.  It is used in helm tests to construct test cases with various Helm values and verify the rendered output.
//
// IMPORTANT: Keep this file in sync with install/helm/kgateway/values.yaml.
// every top-level key in values.yaml must have a matching field here, and vice
// versa.
type HelmValues struct {
	ImagePullSecrets            []LocalObjectReference               `json:"imagePullSecrets,omitempty" yaml:"imagePullSecrets,omitempty"`
	CommonLabels                map[string]string                    `json:"commonLabels,omitempty" yaml:"commonLabels,omitempty"`
	NameOverride                string                               `json:"nameOverride,omitempty" yaml:"nameOverride,omitempty"`
	FullnameOverride            string                               `json:"fullnameOverride,omitempty" yaml:"fullnameOverride,omitempty"`
	ServiceAccount              ServiceAccountValues                 `json:"serviceAccount,omitempty" yaml:"serviceAccount,omitempty"`
	DeploymentAnnotations       map[string]string                    `json:"deploymentAnnotations,omitempty" yaml:"deploymentAnnotations,omitempty"`
	PodAnnotations              map[string]string                    `json:"podAnnotations,omitempty" yaml:"podAnnotations,omitempty"`
	PodSecurityContext          map[string]interface{}               `json:"podSecurityContext,omitempty" yaml:"podSecurityContext,omitempty"`
	SecurityContext             map[string]interface{}               `json:"securityContext,omitempty" yaml:"securityContext,omitempty"`
	Resources                   map[string]interface{}               `json:"resources,omitempty" yaml:"resources,omitempty"`
	NodeSelector                map[string]string                    `json:"nodeSelector,omitempty" yaml:"nodeSelector,omitempty"`
	Tolerations                 []map[string]interface{}             `json:"tolerations,omitempty" yaml:"tolerations,omitempty"`
	Affinity                    map[string]interface{}               `json:"affinity,omitempty" yaml:"affinity,omitempty"`
	Controller                  ControllerValues                     `json:"controller,omitempty" yaml:"controller,omitempty"`
	Image                       ImageValues                          `json:"image,omitempty" yaml:"image,omitempty"`
	DiscoveryNamespaceSelectors []map[string]interface{}             `json:"discoveryNamespaceSelectors,omitempty" yaml:"discoveryNamespaceSelectors,omitempty"`
	GatewayClassParametersRefs  map[string]GatewayClassParametersRef `json:"gatewayClassParametersRefs,omitempty" yaml:"gatewayClassParametersRefs,omitempty"`
	PolicyMerge                 map[string]interface{}               `json:"policyMerge,omitempty" yaml:"policyMerge,omitempty"`
	Waypoint                    WaypointValues                       `json:"waypoint,omitempty"         yaml:"waypoint,omitempty"`
	Validation                  ValidationValues                     `json:"validation,omitempty" yaml:"validation,omitempty"`
}

// LocalObjectReference holds the name of a Kubernetes object (e.g. an image pull secret).
type LocalObjectReference struct {
	Name string `json:"name" yaml:"name"`
}
type ServiceAccountValues struct {	
	Create      *bool             `json:"create" yaml:"create,omitempty"`
	Annotations map[string]string `json:"annotations,omitempty" yaml:"annotations,omitempty"`
	Name        string            `json:"name,omitempty" yaml:"name,omitempty"`
}

// ControllerValues mirrors the controller section of values.yaml.
type ControllerValues struct {
	// ReplicaCount uses a pointer so that 0 is marshalled (not omitted).
	ReplicaCount            *int32                  `json:"replicaCount,omitempty" yaml:"replicaCount,omitempty"`
	PriorityClassName       string                  `json:"priorityClassName,omitempty" yaml:"priorityClassName,omitempty"`
	LogLevel                string                  `json:"logLevel,omitempty" yaml:"logLevel,omitempty"`
	Image                   ControllerImageValues   `json:"image,omitempty" yaml:"image,omitempty"`
	Service                 ControllerServiceValues `json:"service,omitempty" yaml:"service,omitempty"`
	ExtraEnv                map[string]string       `json:"extraEnv,omitempty" yaml:"extraEnv,omitempty"`
	XDS                     XDSValues               `json:"xds,omitempty"              yaml:"xds,omitempty"`
	Strategy                map[string]interface{}  `json:"strategy,omitempty" yaml:"strategy,omitempty"`
	PodDisruptionBudget     map[string]interface{}  `json:"podDisruptionBudget,omitempty" yaml:"podDisruptionBudget,omitempty"`
	HorizontalPodAutoscaler map[string]interface{}  `json:"horizontalPodAutoscaler,omitempty" yaml:"horizontalPodAutoscaler,omitempty"`
	VerticalPodAutoscaler   map[string]interface{}  `json:"verticalPodAutoscaler,omitempty" yaml:"verticalPodAutoscaler,omitempty"`
}

// ControllerImageValues mirrors the controller.image section of values.yaml.
type ControllerImageValues struct {
	Registry   string `json:"registry,omitempty" yaml:"registry,omitempty"`
	Repository string `json:"repository,omitempty" yaml:"repository,omitempty"`
	PullPolicy string `json:"pullPolicy,omitempty" yaml:"pullPolicy,omitempty"`
	Tag        string `json:"tag,omitempty" yaml:"tag,omitempty"`
}

// ControllerServiceValues mirrors the controller.service section of values.yaml.
type ControllerServiceValues struct {
	Enabled                       *bool                  `json:"enabled,omitempty" yaml:"enabled,omitempty"`
	Type                          string                 `json:"type,omitempty" yaml:"type,omitempty"`
	Ports                         ControllerServicePorts `json:"ports,omitempty" yaml:"ports,omitempty"`
	Annotations                   map[string]string      `json:"annotations,omitempty" yaml:"annotations,omitempty"`
	ExtraLabels                   map[string]string      `json:"extraLabels,omitempty" yaml:"extraLabels,omitempty"`
	ClusterIP                     string                 `json:"clusterIP,omitempty" yaml:"clusterIP,omitempty"`
	ClusterIPs                    []string               `json:"clusterIPs,omitempty" yaml:"clusterIPs,omitempty"`
	ExternalIPs                   []string               `json:"externalIPs,omitempty" yaml:"externalIPs,omitempty"`
	ExternalName                  string                 `json:"externalName,omitempty" yaml:"externalName,omitempty"`
	LoadBalancerIP                string                 `json:"loadBalancerIP,omitempty" yaml:"loadBalancerIP,omitempty"`
	LoadBalancerSourceRanges      []string               `json:"loadBalancerSourceRanges,omitempty" yaml:"loadBalancerSourceRanges,omitempty"`
	LoadBalancerClass             string                 `json:"loadBalancerClass,omitempty" yaml:"loadBalancerClass,omitempty"`
	ExternalTrafficPolicy         string                 `json:"externalTrafficPolicy,omitempty" yaml:"externalTrafficPolicy,omitempty"`
	InternalTrafficPolicy         string                 `json:"internalTrafficPolicy,omitempty" yaml:"internalTrafficPolicy,omitempty"`
	HealthCheckNodePort           *int32                 `json:"healthCheckNodePort,omitempty" yaml:"healthCheckNodePort,omitempty"`
	SessionAffinity               string                 `json:"sessionAffinity,omitempty" yaml:"sessionAffinity,omitempty"`
	SessionAffinityConfig         map[string]interface{} `json:"sessionAffinityConfig,omitempty" yaml:"sessionAffinityConfig,omitempty"`
	IPFamilies                    []string               `json:"ipFamilies,omitempty" yaml:"ipFamilies,omitempty"`
	IPFamilyPolicy                string                 `json:"ipFamilyPolicy,omitempty" yaml:"ipFamilyPolicy,omitempty"`
	PublishNotReadyAddresses      *bool                  `json:"publishNotReadyAddresses,omitempty" yaml:"publishNotReadyAddresses,omitempty"`
	AllocateLoadBalancerNodePorts *bool                  `json:"allocateLoadBalancerNodePorts,omitempty" yaml:"allocateLoadBalancerNodePorts,omitempty"`
	TrafficDistribution           string                 `json:"trafficDistribution,omitempty" yaml:"trafficDistribution,omitempty"`
}

// ControllerServicePorts mirrors the controller.service.ports section.
type ControllerServicePorts struct {
	GRPC    *int32 `json:"grpc,omitempty" yaml:"grpc,omitempty"`
	Health  *int32 `json:"health,omitempty" yaml:"health,omitempty"`
	Metrics *int32 `json:"metrics,omitempty" yaml:"metrics,omitempty"`
}

// XDSValues mirrors the controller.xds section of values.yaml.
type XDSValues struct {
	TLS XDSTLSValues ` json:"tls,omitempty" yaml:"tls,omitempty"`
}
type XDSTLSValues struct {	
	Enabled bool `json:"enabled,omitempty" yaml:"enabled,omitempty"`
}

// ImageValues mirrors the top-level image section of values.yaml.
type ImageValues struct {
	Registry   string `json:"registry,omitempty" yaml:"registry,omitempty"`
	Tag        string `json:"tag,omitempty" yaml:"tag,omitempty"`
	PullPolicy string `json:"pullPolicy,omitempty" yaml:"pullPolicy,omitempty"`
}
type GatewayClassParametersRef struct {
	Name      string `json:"name,omitempty" yaml:"name,omitempty"`
	Namespace string `json:"namespace,omitempty" yaml:"namespace,omitempty"`
}

// WaypointValues mirrors the waypoint section of values.yaml.
type WaypointValues struct {
	Enabled bool `json:"enabled,omitempty"          yaml:"enabled,omitempty"`
}

// ValidationValues mirrors the validation section of values.yaml.
type ValidationValues struct {
	Level string `json:"level,omitempty" yaml:"level,omitempty"`
}
