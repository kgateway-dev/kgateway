package kgateway

import (
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/shared"
)

// +kubebuilder:rbac:groups=gateway.kgateway.dev,resources=gatewayparameters,verbs=get;list;watch
// +kubebuilder:rbac:groups=gateway.kgateway.dev,resources=gatewayparameters/status,verbs=get;update;patch

// A GatewayParameters contains configuration that is used to dynamically
// provision kgateway's data plane (Envoy or agentgateway proxy instance), based on a
// Kubernetes Gateway.
//
// +genclient
// +kubebuilder:object:root=true
// +kubebuilder:metadata:labels={app=kgateway,app.kubernetes.io/name=kgateway}
// +kubebuilder:resource:categories=kgateway,path=gatewayparameters
// +kubebuilder:subresource:status
type GatewayParameters struct {
	metav1.TypeMeta `json:",inline"`
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty"`
	// +required
	Spec GatewayParametersSpec `json:"spec"`
	// +optional
	Status GatewayParametersStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
type GatewayParametersList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []GatewayParameters `json:"items"`
}

// A GatewayParametersSpec describes the type of environment/platform in which
// the proxy will be provisioned.
//
// +kubebuilder:validation:ExactlyOneOf=kube;selfManaged
type GatewayParametersSpec struct {
	// The proxy will be deployed on Kubernetes. Overlays (fields named with
	// the suffix 'Overlay') are applied after non-overlay configurations
	// ("configs"). Configs on a GatewayClass (inside of GatewayParameters) are
	// applied before configs on a Gateway (inside of GatewayParameters), which
	// merge together. Overlays on a GatewayClass are then applied, and
	// finally, overlays on a Gateway. It is recommended to use an overlay only
	// if necessary (no config exists that can achieve the same goal) for
	// smoother upgrades, readability, and earlier and improved validation.
	//
	// +optional
	Kube *KubernetesProxyConfig `json:"kube,omitempty"`

	// The proxy will be self-managed and not auto-provisioned.
	//
	// +optional
	// +kubebuilder:pruning:PreserveUnknownFields
	SelfManaged *SelfManagedGateway `json:"selfManaged,omitempty"`
}

func (in *GatewayParametersSpec) GetKube() *KubernetesProxyConfig {
	if in == nil {
		return nil
	}
	return in.Kube
}

func (in *GatewayParametersSpec) GetSelfManaged() *SelfManagedGateway {
	if in == nil {
		return nil
	}
	return in.SelfManaged
}

// The current conditions of the GatewayParameters. This is not currently implemented.
type GatewayParametersStatus struct{}

type SelfManagedGateway struct{}

// KubernetesProxyConfig configures the set of Kubernetes resources that will
// be provisioned for a given Gateway. Overlays (fields named with the suffix
// 'Overlay') are applied after non-overlay configurations ("configs"). Configs
// on a GatewayClass (inside of GatewayParameters) are applied before configs
// on a Gateway (inside of GatewayParameters), which merge together. Overlays
// on a GatewayClass are then applied, and finally, overlays on a Gateway. It
// is recommended to use an overlay only if necessary (no config exists that
// can achieve the same goal) for smoother upgrades, readability, and earlier
// and improved validation.
type KubernetesProxyConfig struct {
	// Use a Kubernetes deployment as the proxy workload type. Currently, this is the only
	// supported workload type.
	//
	// +optional
	Deployment *ProxyDeployment `json:"deployment,omitempty"`

	// Configuration for the container running Envoy.
	//
	// +optional
	EnvoyContainer *EnvoyContainer `json:"envoyContainer,omitempty"`

	// Configuration for the container running the Secret Discovery Service (SDS).
	//
	// +optional
	SdsContainer *SdsContainer `json:"sdsContainer,omitempty"`

	// Configuration for the pods that will be created.
	//
	// +optional
	PodTemplate *Pod `json:"podTemplate,omitempty"`

	// Configuration for the Kubernetes Service that exposes the proxy over
	// the network.
	//
	// +optional
	Service *Service `json:"service,omitempty"`

	// Configuration for the Kubernetes ServiceAccount used by the proxy pods.
	//
	// +optional
	ServiceAccount *ServiceAccount `json:"serviceAccount,omitempty"`

	// Configuration for the Istio integration.
	//
	// +optional
	Istio *IstioIntegration `json:"istio,omitempty"`

	// Configuration for the stats server.
	//
	// +optional
	Stats *StatsConfig `json:"stats,omitempty"`

	// OmitDefaultSecurityContext is used to control whether or not
	// `securityContext` fields should be rendered for the various generated
	// Deployments/Containers that are dynamically provisioned by the deployer.
	//
	// When set to true, no `securityContexts` will be provided and will left
	// to the user/platform to be provided.
	//
	// This should be enabled on platforms such as Red Hat OpenShift where the
	// `securityContext` will be dynamically added to enforce the appropriate
	// level of security.
	//
	// +optional
	OmitDefaultSecurityContext *bool `json:"omitDefaultSecurityContext,omitempty"`

	GatewayParametersOverlays `json:",inline"`
}

func (in *KubernetesProxyConfig) GetDeployment() *ProxyDeployment {
	if in == nil {
		return nil
	}
	return in.Deployment
}

func (in *KubernetesProxyConfig) GetEnvoyContainer() *EnvoyContainer {
	if in == nil {
		return nil
	}
	return in.EnvoyContainer
}

func (in *KubernetesProxyConfig) GetSdsContainer() *SdsContainer {
	if in == nil {
		return nil
	}
	return in.SdsContainer
}

func (in *KubernetesProxyConfig) GetPodTemplate() *Pod {
	if in == nil {
		return nil
	}
	return in.PodTemplate
}

func (in *KubernetesProxyConfig) GetService() *Service {
	if in == nil {
		return nil
	}
	return in.Service
}

func (in *KubernetesProxyConfig) GetServiceAccount() *ServiceAccount {
	if in == nil {
		return nil
	}
	return in.ServiceAccount
}

func (in *KubernetesProxyConfig) GetIstio() *IstioIntegration {
	if in == nil {
		return nil
	}
	return in.Istio
}

func (in *KubernetesProxyConfig) GetStats() *StatsConfig {
	if in == nil {
		return nil
	}
	return in.Stats
}

func (in *KubernetesProxyConfig) GetOmitDefaultSecurityContext() *bool {
	if in == nil {
		return nil
	}
	return in.OmitDefaultSecurityContext
}

// ProxyDeployment configures the Proxy deployment in Kubernetes.
type ProxyDeployment struct {
	// The number of desired pods.
	// If omitted, behavior will be managed by the K8s control plane, and will default to 1.
	// If you are using an HPA, make sure to not explicitly define this.
	// K8s reference: https://kubernetes.io/docs/concepts/workloads/controllers/deployment/#replicas
	//
	// +optional
	// +kubebuilder:validation:Minimum=0
	Replicas *int32 `json:"replicas,omitempty"`

	// The deployment strategy to use to replace existing pods with new
	// ones. The Kubernetes default is a RollingUpdate with 25% maxUnavailable,
	// 25% maxSurge.
	//
	// E.g., to recreate pods, minimizing resources for the rollout but causing downtime:
	// strategy:
	//   type: Recreate
	// E.g., to roll out as a RollingUpdate but with non-default parameters:
	// strategy:
	//   type: RollingUpdate
	//   rollingUpdate:
	//     maxSurge: 100%
	//
	// +optional
	Strategy *appsv1.DeploymentStrategy `json:"strategy,omitempty"`
}

func (in *ProxyDeployment) GetReplicas() *int32 {
	if in == nil {
		return nil
	}
	return in.Replicas
}

func (in *ProxyDeployment) GetStrategy() *appsv1.DeploymentStrategy {
	if in == nil {
		return nil
	}
	return in.Strategy
}

// EnvoyContainer configures the container running Envoy.
type EnvoyContainer struct {
	// Initial envoy configuration.
	//
	// +optional
	Bootstrap *EnvoyBootstrap `json:"bootstrap,omitempty"`

	// The envoy container image. See
	// https://kubernetes.io/docs/concepts/containers/images
	// for details.
	//
	// Default values, which may be overridden individually:
	//
	//	registry: quay.io/solo-io
	//	repository: envoy-wrapper
	//	tag: <kgateway version>
	//	pullPolicy: IfNotPresent
	//
	// +optional
	Image *Image `json:"image,omitempty"`

	// The security context for this container. See
	// https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.26/#securitycontext-v1-core
	// for details.
	//
	// +optional
	SecurityContext *corev1.SecurityContext `json:"securityContext,omitempty"`

	// The compute resources required by this container. See
	// https://kubernetes.io/docs/concepts/configuration/manage-resources-containers/
	// for details.
	//
	// +optional
	Resources *corev1.ResourceRequirements `json:"resources,omitempty"`

	// do not use slice of pointers: https://github.com/kubernetes/code-generator/issues/166

	// The container environment variables.
	//
	// +optional
	Env []corev1.EnvVar `json:"env,omitempty"`

	// Additional volume mounts to add to the container. See
	// https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.26/#volumemount-v1-core
	// for details.
	//
	// +optional
	ExtraVolumeMounts []corev1.VolumeMount `json:"extraVolumeMounts,omitempty"`
}

func (in *EnvoyContainer) GetBootstrap() *EnvoyBootstrap {
	if in == nil {
		return nil
	}
	return in.Bootstrap
}

func (in *EnvoyContainer) GetImage() *Image {
	if in == nil {
		return nil
	}
	return in.Image
}

func (in *EnvoyContainer) GetSecurityContext() *corev1.SecurityContext {
	if in == nil {
		return nil
	}
	return in.SecurityContext
}

func (in *EnvoyContainer) GetResources() *corev1.ResourceRequirements {
	if in == nil {
		return nil
	}
	return in.Resources
}

func (in *EnvoyContainer) GetEnv() []corev1.EnvVar {
	if in == nil {
		return nil
	}
	return in.Env
}

// EnvoyBootstrap configures the Envoy proxy instance that is provisioned from a
// Kubernetes Gateway.
type EnvoyBootstrap struct {
	// Envoy log level. Options include "trace", "debug", "info", "warn", "error",
	// "critical" and "off". Defaults to "info". See
	// https://www.envoyproxy.io/docs/envoy/latest/start/quick-start/run-envoy#debugging-envoy
	// for more information.
	//
	// +optional
	LogLevel *string `json:"logLevel,omitempty"`

	// Envoy log levels for specific components. The keys are component names and
	// the values are one of "trace", "debug", "info", "warn", "error",
	// "critical", or "off", e.g.
	//
	//	```yaml
	//	componentLogLevels:
	//	  upstream: debug
	//	  connection: trace
	//	```
	//
	// These will be converted to the `--component-log-level` Envoy argument
	// value. See
	// https://www.envoyproxy.io/docs/envoy/latest/start/quick-start/run-envoy#debugging-envoy
	// for more information.
	//
	// Note: the keys and values cannot be empty, but they are not otherwise validated.
	//
	// +optional
	ComponentLogLevels map[string]string `json:"componentLogLevels,omitempty"`

	// DNS resolver configuration for Envoy's CARES DNS resolver.
	// This configuration applies to all clusters and affects DNS query behavior.
	// See https://www.envoyproxy.io/docs/envoy/latest/api-v3/extensions/network/dns_resolver/cares/v3/cares_dns_resolver.proto
	// for more information.
	//
	// +optional
	DnsResolver *DnsResolver `json:"dnsResolver,omitempty"`
}

// DnsResolver configures the CARES DNS resolver for Envoy.
type DnsResolver struct {
	// Maximum number of UDP queries to be issued on a single UDP channel.
	// This helps prevent DNS query pinning to a single resolver, addressing
	// the issue described in https://github.com/istio/istio/issues/53577.
	// Defaults to 100 if not specified. Set to 0 to disable this limit.
	// See https://www.envoyproxy.io/docs/envoy/latest/api-v3/extensions/network/dns_resolver/cares/v3/cares_dns_resolver.proto#extensions-network-dns-resolver-cares-v3-caresdnsresolverconfig
	//
	// +optional
	// +kubebuilder:validation:Minimum=0
	UdpMaxQueries *int32 `json:"udpMaxQueries,omitempty"`
}

func (in *EnvoyBootstrap) GetLogLevel() *string {
	if in == nil {
		return nil
	}
	return in.LogLevel
}

func (in *EnvoyBootstrap) GetComponentLogLevels() map[string]string {
	if in == nil {
		return nil
	}
	return in.ComponentLogLevels
}

func (in *EnvoyBootstrap) GetDnsResolver() *DnsResolver {
	if in == nil {
		return nil
	}
	return in.DnsResolver
}

func (in *DnsResolver) GetUdpMaxQueries() *int32 {
	if in == nil {
		return nil
	}
	return in.UdpMaxQueries
}

// SdsContainer configures the container running SDS sidecar.
type SdsContainer struct {
	// The SDS container image. See
	// https://kubernetes.io/docs/concepts/containers/images
	// for details.
	//
	// +optional
	Image *Image `json:"image,omitempty"`

	// The security context for this container. See
	// https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.26/#securitycontext-v1-core
	// for details.
	//
	// +optional
	SecurityContext *corev1.SecurityContext `json:"securityContext,omitempty"`

	// The compute resources required by this container. See
	// https://kubernetes.io/docs/concepts/configuration/manage-resources-containers/
	// for details.
	//
	// +optional
	Resources *corev1.ResourceRequirements `json:"resources,omitempty"`

	// Initial SDS container configuration.
	//
	// +optional
	Bootstrap *SdsBootstrap `json:"bootstrap,omitempty"`
}

func (in *SdsContainer) GetImage() *Image {
	if in == nil {
		return nil
	}
	return in.Image
}

func (in *SdsContainer) GetSecurityContext() *corev1.SecurityContext {
	if in == nil {
		return nil
	}
	return in.SecurityContext
}

func (in *SdsContainer) GetResources() *corev1.ResourceRequirements {
	if in == nil {
		return nil
	}
	return in.Resources
}

func (in *SdsContainer) GetBootstrap() *SdsBootstrap {
	if in == nil {
		return nil
	}
	return in.Bootstrap
}

// SdsBootstrap configures the SDS instance that is provisioned from a Kubernetes Gateway.
type SdsBootstrap struct {
	// Log level for SDS. Options include "info", "debug", "warn", "error", "panic" and "fatal".
	// Default level is "info".
	//
	// +optional
	LogLevel *string `json:"logLevel,omitempty"`
}

func (in *SdsBootstrap) GetLogLevel() *string {
	if in == nil {
		return nil
	}
	return in.LogLevel
}

// IstioIntegration configures the Istio integration settings used by kgateway's data plane
type IstioIntegration struct {
	// Configuration for the container running istio-proxy.
	// Note that if Istio integration is not enabled, the istio container will not be injected
	// into the gateway proxy deployment.
	//
	// +optional
	IstioProxyContainer *IstioContainer `json:"istioProxyContainer,omitempty"`

	// Deprecated: This field was never implemented in v2 and will be deleted.
	// If you need custom TLS certificate handling, use the built-in SDS (Secret Discovery
	// Service) container via the sdsContainer field instead. For other sidecar needs,
	// use a deployment overlay. Example overlay that adds a sidecar:
	//
	//	spec:
	//	  kube:
	//	    deploymentOverlay:
	//	      spec:
	//	        template:
	//	          spec:
	//	            containers:
	//	              - name: my-sidecar
	//	                image: my-sidecar:latest
	//
	// +optional
	CustomSidecars []corev1.Container `json:"customSidecars,omitempty"`
}

func (in *IstioIntegration) GetIstioProxyContainer() *IstioContainer {
	if in == nil {
		return nil
	}
	return in.IstioProxyContainer
}

func (in *IstioIntegration) GetCustomSidecars() []corev1.Container {
	if in == nil {
		return nil
	}
	return in.CustomSidecars
}

// IstioContainer configures the container running the istio-proxy.
type IstioContainer struct {
	// The container image. See
	// https://kubernetes.io/docs/concepts/containers/images
	// for details.
	//
	// +optional
	Image *Image `json:"image,omitempty"`

	// The security context for this container. See
	// https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.26/#securitycontext-v1-core
	// for details.
	//
	// +optional
	SecurityContext *corev1.SecurityContext `json:"securityContext,omitempty"`

	// The compute resources required by this container. See
	// https://kubernetes.io/docs/concepts/configuration/manage-resources-containers/
	// for details.
	//
	// +optional
	Resources *corev1.ResourceRequirements `json:"resources,omitempty"`

	// Log level for istio-proxy. Options include "info", "debug", "warning", and "error".
	// Default level is info Default is "warning".
	//
	// +optional
	LogLevel *string `json:"logLevel,omitempty"`

	// The address of the istio discovery service. Defaults to "istiod.istio-system.svc:15012".
	//
	// +optional
	IstioDiscoveryAddress *string `json:"istioDiscoveryAddress,omitempty"`

	// The mesh id of the istio mesh. Defaults to "cluster.local".
	//
	// +optional
	IstioMetaMeshId *string `json:"istioMetaMeshId,omitempty"`

	// The cluster id of the istio cluster. Defaults to "Kubernetes".
	//
	// +optional
	IstioMetaClusterId *string `json:"istioMetaClusterId,omitempty"`
}

func (in *IstioContainer) GetImage() *Image {
	if in == nil {
		return nil
	}
	return in.Image
}

func (in *IstioContainer) GetSecurityContext() *corev1.SecurityContext {
	if in == nil {
		return nil
	}
	return in.SecurityContext
}

func (in *IstioContainer) GetResources() *corev1.ResourceRequirements {
	if in == nil {
		return nil
	}
	return in.Resources
}

func (in *IstioContainer) GetLogLevel() *string {
	if in == nil {
		return nil
	}
	return in.LogLevel
}

func (in *IstioContainer) GetIstioDiscoveryAddress() *string {
	if in == nil {
		return nil
	}
	return in.IstioDiscoveryAddress
}

func (in *IstioContainer) GetIstioMetaMeshId() *string {
	if in == nil {
		return nil
	}
	return in.IstioMetaMeshId
}

func (in *IstioContainer) GetIstioMetaClusterId() *string {
	if in == nil {
		return nil
	}
	return in.IstioMetaClusterId
}

// Configuration for the stats server.
type StatsConfig struct {
	// Whether to expose metrics annotations and ports for scraping metrics.
	//
	// +optional
	Enabled *bool `json:"enabled,omitempty"`

	// The Envoy stats endpoint to which the metrics are written
	//
	// +optional
	RoutePrefixRewrite *string `json:"routePrefixRewrite,omitempty"`

	// Enables an additional route to the stats cluster defaulting to /stats
	//
	// +optional
	EnableStatsRoute *bool `json:"enableStatsRoute,omitempty"`

	// The Envoy stats endpoint with general metrics for the additional stats route
	//
	// +optional
	StatsRoutePrefixRewrite *string `json:"statsRoutePrefixRewrite,omitempty"`

	// Matcher configures inclusion or exclusion lists for Envoy stats.
	// Only one of inclusionList or exclusionList may be set.
	// If unset, Envoy's default stats emission behavior applies.
	//
	// +optional
	Matcher *StatsMatcher `json:"matcher,omitempty"`
}

func (in *StatsConfig) GetEnabled() *bool {
	if in == nil {
		return nil
	}
	return in.Enabled
}

func (in *StatsConfig) GetRoutePrefixRewrite() *string {
	if in == nil {
		return nil
	}
	return in.RoutePrefixRewrite
}

func (in *StatsConfig) GetEnableStatsRoute() *bool {
	if in == nil {
		return nil
	}
	return in.EnableStatsRoute
}

func (in *StatsConfig) GetStatsRoutePrefixRewrite() *string {
	if in == nil {
		return nil
	}
	return in.StatsRoutePrefixRewrite
}

func (in *StatsConfig) GetMatcher() *StatsMatcher {
	if in == nil {
		return nil
	}
	return in.Matcher
}

// StatsMatcher specifies either an inclusion or exclusion list for Envoy stats.
// See Envoy's envoy.config.metrics.v3.StatsMatcher for details.
// +kubebuilder:validation:MaxProperties=1
// +kubebuilder:validation:MinProperties=1
type StatsMatcher struct {
	// inclusionList specifies which stats to include, using string matchers.
	// +optional
	// +kubebuilder:validation:MaxItems=16
	InclusionList []shared.StringMatcher `json:"inclusionList,omitempty"`

	// exclusionList specifies which stats to exclude, using string matchers.
	// +optional
	// +kubebuilder:validation:MaxItems=16
	ExclusionList []shared.StringMatcher `json:"exclusionList,omitempty"`
}

func (in *StatsMatcher) GetInclusionList() []shared.StringMatcher {
	if in == nil {
		return nil
	}
	return in.InclusionList
}

func (in *StatsMatcher) GetExclusionList() []shared.StringMatcher {
	if in == nil {
		return nil
	}
	return in.ExclusionList
}

type GatewayParametersOverlays struct {
	// deploymentOverlay allows specifying overrides for the generated Deployment resource.
	// +optional
	DeploymentOverlay *shared.KubernetesResourceOverlay `json:"deploymentOverlay,omitempty"`

	// serviceOverlay allows specifying overrides for the generated Service resource.
	// +optional
	ServiceOverlay *shared.KubernetesResourceOverlay `json:"serviceOverlay,omitempty"`

	// serviceAccountOverlay allows specifying overrides for the generated ServiceAccount resource.
	// +optional
	ServiceAccountOverlay *shared.KubernetesResourceOverlay `json:"serviceAccountOverlay,omitempty"`

	// podDisruptionBudget allows creating a PodDisruptionBudget for the proxy.
	// If absent, no PDB is created. If present, a PDB is created with its selector
	// automatically configured to target the proxy Deployment.
	// The metadata and spec fields from this overlay are applied to the generated PDB.
	// +optional
	PodDisruptionBudget *shared.KubernetesResourceOverlay `json:"podDisruptionBudget,omitempty"`

	// horizontalPodAutoscaler allows creating a HorizontalPodAutoscaler for the proxy.
	// If absent, no HPA is created. If present, an HPA is created with its scaleTargetRef
	// automatically configured to target the proxy Deployment.
	// The metadata and spec fields from this overlay are applied to the generated HPA.
	// +optional
	HorizontalPodAutoscaler *shared.KubernetesResourceOverlay `json:"horizontalPodAutoscaler,omitempty"`

	// verticalPodAutoscaler allows creating a VerticalPodAutoscaler for the proxy.
	// If absent, no VPA is created. If present, a VPA is created with its targetRef
	// automatically configured to target the proxy Deployment.
	// The metadata and spec fields from this overlay are applied to the generated VPA.
	// +optional
	VerticalPodAutoscaler *shared.KubernetesResourceOverlay `json:"verticalPodAutoscaler,omitempty"`
}
