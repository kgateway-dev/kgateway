package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +kubebuilder:rbac:groups=gateway.kgateway.dev,resources=gatewayparameters,verbs=get;list;watch
// +kubebuilder:rbac:groups=gateway.kgateway.dev,resources=gatewayparameters/status,verbs=get;update;patch

// A GatewayParameters contains configuration that is used to dynamically
// provision kgateway's data plane (Envoy proxy instance), based on a
// Kubernetes Gateway.
//
// +genclient
// +kubebuilder:object:root=true
// +kubebuilder:metadata:labels={app=kgateway,app.kubernetes.io/name=kgateway}
// +kubebuilder:resource:categories=kgateway
// +kubebuilder:subresource:status
type GatewayParameters struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   GatewayParametersSpec   `json:"spec,omitempty"`
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
// +kubebuilder:validation:XValidation:message="exactly one of 'kube' or 'selfManaged' must be set",rule="has(self.kube) ? !has(self.selfManaged) : has(self.selfManaged)"
type GatewayParametersSpec struct {
	// The proxy will be deployed on Kubernetes.
	//
	// +kubebuilder:validation:Optional
	Kube *KubernetesProxyConfig `json:"kube,omitempty"`

	// The proxy will be self-managed and not auto-provisioned.
	//
	// +kubebuilder:validation:Optional
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
type GatewayParametersStatus struct {
}

type SelfManagedGateway struct {
}

// Configuration for the set of Kubernetes resources that will be provisioned
// for a given Gateway.
type KubernetesProxyConfig struct {
	// Use a Kubernetes deployment as the proxy workload type. Currently, this is the only
	// supported workload type.
	//
	// +kubebuilder:validation:Optional
	Deployment *ProxyDeployment `json:"deployment,omitempty"`

	// Configuration for the container running Envoy.
	//
	// +kubebuilder:validation:Optional
	EnvoyContainer *EnvoyContainer `json:"envoyContainer,omitempty"`

	// Configuration for the container running the Secret Discovery Service (SDS).
	//
	// +kubebuilder:validation:Optional
	SdsContainer *SdsContainer `json:"sdsContainer,omitempty"`

	// Configuration for the pods that will be created.
	//
	// +kubebuilder:validation:Optional
	PodTemplate *Pod `json:"podTemplate,omitempty"`

	// Configuration for the Kubernetes Service that exposes the Envoy proxy over
	// the network.
	//
	// +kubebuilder:validation:Optional
	Service *Service `json:"service,omitempty"`

	// Configuration for the Kubernetes ServiceAccount used by the Envoy pod.
	//
	// +kubebuilder:validation:Optional
	ServiceAccount *ServiceAccount `json:"serviceAccount,omitempty"`

	// Configuration for the Istio integration.
	//
	// +kubebuilder:validation:Optional
	Istio *IstioIntegration `json:"istio,omitempty"`

	// Configuration for the stats server.
	//
	// +kubebuilder:validation:Optional
	Stats *StatsConfig `json:"stats,omitempty"`

	// Configuration for the AI extension.
	//
	// +kubebuilder:validation:Optional
	AiExtension *AiExtension `json:"aiExtension,omitempty"`

	// Configure the AgentGateway integration
	//
	// +kubebuilder:validation:Optional
	AgentGateway *AgentGateway `json:"agentGateway,omitempty"`

	// Used to unset the `runAsUser` values in security contexts.
	FloatingUserId *bool `json:"floatingUserId,omitempty"`
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

func (in *KubernetesProxyConfig) GetAiExtension() *AiExtension {
	if in == nil {
		return nil
	}
	return in.AiExtension
}

func (in *KubernetesProxyConfig) GetAgentGateway() *AgentGateway {
	if in == nil {
		return nil
	}
	return in.AgentGateway
}

func (in *KubernetesProxyConfig) GetFloatingUserId() *bool {
	if in == nil {
		return nil
	}
	return in.FloatingUserId
}

// Configuration for the Proxy deployment in Kubernetes.
type ProxyDeployment struct {
	// The number of desired pods. Defaults to 1.
	//
	// +kubebuilder:validation:Optional
	Replicas *uint32 `json:"replicas,omitempty"`
}

func (in *ProxyDeployment) GetReplicas() *uint32 {
	if in == nil {
		return nil
	}
	return in.Replicas
}

// Configuration for the container running Envoy.
type EnvoyContainer struct {

	// Initial envoy configuration.
	//
	// +kubebuilder:validation:Optional
	Bootstrap *EnvoyBootstrap `json:"bootstrap,omitempty"`

	// The envoy container image. See
	// https://kubernetes.io/docs/concepts/containers/images
	// for details.
	//
	// Default values, which may be overridden individually:
	//
	//	registry: quay.io/solo-io
	//	repository: gloo-envoy-wrapper (OSS) / gloo-ee-envoy-wrapper (EE)
	//	tag: <gloo version> (OSS) / <gloo-ee version> (EE)
	//	pullPolicy: IfNotPresent
	//
	// +kubebuilder:validation:Optional
	Image *Image `json:"image,omitempty"`

	// The security context for this container. See
	// https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.26/#securitycontext-v1-core
	// for details.
	//
	// +kubebuilder:validation:Optional
	SecurityContext *corev1.SecurityContext `json:"securityContext,omitempty"`

	// The compute resources required by this container. See
	// https://kubernetes.io/docs/concepts/configuration/manage-resources-containers/
	// for details.
	//
	// +kubebuilder:validation:Optional
	Resources *corev1.ResourceRequirements `json:"resources,omitempty"`
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

// Configuration for the Envoy proxy instance that is provisioned from a
// Kubernetes Gateway.
type EnvoyBootstrap struct {
	// Envoy log level. Options include "trace", "debug", "info", "warn", "error",
	// "critical" and "off". Defaults to "info". See
	// https://www.envoyproxy.io/docs/envoy/latest/start/quick-start/run-envoy#debugging-envoy
	// for more information.
	//
	// +kubebuilder:validation:Optional
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
	// +kubebuilder:validation:Optional
	ComponentLogLevels map[string]string `json:"componentLogLevels,omitempty"`
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

// Configuration for the container running Gloo SDS.
type SdsContainer struct {
	// The SDS container image. See
	// https://kubernetes.io/docs/concepts/containers/images
	// for details.
	//
	// +kubebuilder:validation:Optional
	Image *Image `json:"image,omitempty"`

	// The security context for this container. See
	// https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.26/#securitycontext-v1-core
	// for details.
	//
	// +kubebuilder:validation:Optional
	SecurityContext *corev1.SecurityContext `json:"securityContext,omitempty"`

	// The compute resources required by this container. See
	// https://kubernetes.io/docs/concepts/configuration/manage-resources-containers/
	// for details.
	//
	// +kubebuilder:validation:Optional
	Resources *corev1.ResourceRequirements `json:"resources,omitempty"`

	// Initial SDS container configuration.
	//
	// +kubebuilder:validation:Optional
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

// Configuration for the SDS instance that is provisioned from a Kubernetes Gateway.
type SdsBootstrap struct {
	// Log level for SDS. Options include "info", "debug", "warn", "error", "panic" and "fatal".
	// Default level is "info".
	//
	// +kubebuilder:validation:Optional
	LogLevel *string `json:"logLevel,omitempty"`
}

func (in *SdsBootstrap) GetLogLevel() *string {
	if in == nil {
		return nil
	}
	return in.LogLevel
}

// Configuration for the Istio integration settings used by a Gloo Gateway's data plane (Envoy proxy instance)
type IstioIntegration struct {
	// Configuration for the container running istio-proxy.
	// Note that if Istio integration is not enabled, the istio container will not be injected
	// into the gateway proxy deployment.
	//
	// +kubebuilder:validation:Optional
	IstioProxyContainer *IstioContainer `json:"istioProxyContainer,omitempty"`

	// do not use slice of pointers: https://github.com/kubernetes/code-generator/issues/166
	// Override the default Istio sidecar in gateway-proxy with a custom container.
	//
	// +kubebuilder:validation:Optional
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

// Configuration for the container running the istio-proxy.
type IstioContainer struct {
	// The envoy container image. See
	// https://kubernetes.io/docs/concepts/containers/images
	// for details.
	//
	// +kubebuilder:validation:Optional
	Image *Image `json:"image,omitempty"`

	// The security context for this container. See
	// https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.26/#securitycontext-v1-core
	// for details.
	//
	// +kubebuilder:validation:Optional
	SecurityContext *corev1.SecurityContext `json:"securityContext,omitempty"`

	// The compute resources required by this container. See
	// https://kubernetes.io/docs/concepts/configuration/manage-resources-containers/
	// for details.
	//
	// +kubebuilder:validation:Optional
	Resources *corev1.ResourceRequirements `json:"resources,omitempty"`

	// Log level for istio-proxy. Options include "info", "debug", "warning", and "error".
	// Default level is info Default is "warning".
	//
	// +kubebuilder:validation:Optional
	LogLevel *string `json:"logLevel,omitempty"`

	// The address of the istio discovery service. Defaults to "istiod.istio-system.svc:15012".
	//
	// +kubebuilder:validation:Optional
	IstioDiscoveryAddress *string `json:"istioDiscoveryAddress,omitempty"`

	// The mesh id of the istio mesh. Defaults to "cluster.local".
	//
	// +kubebuilder:validation:Optional
	IstioMetaMeshId *string `json:"istioMetaMeshId,omitempty"`

	// The cluster id of the istio cluster. Defaults to "Kubernetes".
	//
	// +kubebuilder:validation:Optional
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
	// +kubebuilder:validation:Optional
	Enabled *bool `json:"enabled,omitempty"`

	// The Envoy stats endpoint to which the metrics are written
	//
	// +kubebuilder:validation:Optional
	RoutePrefixRewrite *string `json:"routePrefixRewrite,omitempty"`

	// Enables an additional route to the stats cluster defaulting to /stats
	//
	// +kubebuilder:validation:Optional
	EnableStatsRoute *bool `json:"enableStatsRoute,omitempty"`

	// The Envoy stats endpoint with general metrics for the additional stats route
	//
	// +kubebuilder:validation:Optional
	StatsRoutePrefixRewrite *string `json:"statsRoutePrefixRewrite,omitempty"`
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

// Configuration for the AI extension.
type AiExtension struct {
	// Whether to enable the extension.
	//
	// +kubebuilder:validation:Optional
	Enabled *bool `json:"enabled,omitempty"`

	// The extension's container image. See
	// https://kubernetes.io/docs/concepts/containers/images
	// for details.
	//
	// +kubebuilder:validation:Optional
	Image *Image `json:"image,omitempty"`

	// The security context for this container. See
	// https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.26/#securitycontext-v1-core
	// for details.
	//
	// +kubebuilder:validation:Optional
	SecurityContext *corev1.SecurityContext `json:"securityContext,omitempty"`

	// The compute resources required by this container. See
	// https://kubernetes.io/docs/concepts/configuration/manage-resources-containers/
	// for details.
	//
	// +kubebuilder:validation:Optional
	Resources *corev1.ResourceRequirements `json:"resources,omitempty"`

	// do not use slice of pointers: https://github.com/kubernetes/code-generator/issues/166

	// The extension's container environment variables.
	//
	// +kubebuilder:validation:Optional
	Env []corev1.EnvVar `json:"env,omitempty"`

	// The extension's container ports.
	//
	// +kubebuilder:validation:Optional
	Ports []corev1.ContainerPort `json:"ports,omitempty"`

	// Additional stats config for AI Extension.
	// This config can be useful for adding custom labels to the request metrics.
	// +optional
	//
	// Example:
	// ```yaml
	// stats:
	//   customLabels:
	//     - name: "subject"
	//       metadataNamespace: "envoy.filters.http.jwt_authn"
	//       metadataKey: "principal:sub"
	//     - name: "issuer"
	//       metadataNamespace: "envoy.filters.http.jwt_authn"
	//       metadataKey: "principal:iss"
	// ```
	Stats *AiExtensionStats `json:"stats,omitempty"`
}

func (in *AiExtension) GetEnabled() *bool {
	if in == nil {
		return nil
	}
	return in.Enabled
}

func (in *AiExtension) GetImage() *Image {
	if in == nil {
		return nil
	}
	return in.Image
}

func (in *AiExtension) GetSecurityContext() *corev1.SecurityContext {
	if in == nil {
		return nil
	}
	return in.SecurityContext
}

func (in *AiExtension) GetResources() *corev1.ResourceRequirements {
	if in == nil {
		return nil
	}
	return in.Resources
}

func (in *AiExtension) GetEnv() []corev1.EnvVar {
	if in == nil {
		return nil
	}
	return in.Env
}

func (in *AiExtension) GetPorts() []corev1.ContainerPort {
	if in == nil {
		return nil
	}
	return in.Ports
}

func (in *AiExtension) GetStats() *AiExtensionStats {
	if in == nil {
		return nil
	}
	return in.Stats
}

type AiExtensionStats struct {
	// Set of custom labels to be added to the request metrics.
	// These will be added on each request which goes through the AI Extension.
	// +optional
	CustomLabels []*CustomLabel `json:"customLabels,omitempty"`
}

func (in *AiExtensionStats) GetCustomLabels() []*CustomLabel {
	if in == nil {
		return nil
	}
	return in.CustomLabels
}

type CustomLabel struct {
	// Name of the label to use in the prometheus metrics
	//
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// The dynamic metadata namespace to get the data from. If not specified, the default namespace will be
	// the envoy JWT filter namespace.
	// This can also be used in combination with early_transformations to insert custom data.
	// +optional
	//
	// +kubebuilder:validation:Enum=envoy.filters.http.jwt_authn;io.solo.transformation
	MetadataNamespace *string `json:"metadataNamespace,omitempty"`

	// The key to use to get the data from the metadata namespace.
	// If using a JWT data please see the following envoy docs: https://www.envoyproxy.io/docs/envoy/latest/api-v3/extensions/filters/http/jwt_authn/v3/config.proto#envoy-v3-api-field-extensions-filters-http-jwt-authn-v3-jwtprovider-payload-in-metadata
	// This key follows the same format as the envoy access logging for dynamic metadata.
	// Examples can be found here: https://www.envoyproxy.io/docs/envoy/latest/configuration/observability/access_log/usage
	//
	// +kubebuilder:validation:MinLength=1
	MetdataKey string `json:"metadataKey"`

	// The key delimiter to use, by default this is set to `:`.
	// This allows for keys with `.` in them to be used.
	// For example, if you have keys in your path with `:` in them, (e.g. `key1:key2:value`)
	// you can instead set this to `~` to be able to split those keys properly.
	// +optional
	KeyDelimiter *string `json:"keyDelimiter,omitempty"`
}

func (in *CustomLabel) GetName() string {
	if in == nil {
		return ""
	}
	return in.Name
}

func (in *CustomLabel) GetMetadataNamespace() *string {
	if in == nil {
		return nil
	}
	return in.MetadataNamespace
}

func (in *CustomLabel) GetMetdataKey() string {
	if in == nil {
		return ""
	}
	return in.MetdataKey
}

func (in *CustomLabel) GetKeyDelimiter() *string {
	if in == nil {
		return nil
	}
	return in.KeyDelimiter
}

// Configuration of the AgentGateway integration
type AgentGateway struct {
	// Whether to enable the extension.
	//
	// +kubebuilder:validation:Optional
	Enabled *bool `json:"enabled,omitempty"`

	// Log level for the agentgateway. Defaults to info.
	// Levels include "trace", "debug", "info", "error", "warn". See: https://docs.rs/tracing/latest/tracing/struct.Level.html
	//
	// +kubebuilder:validation:Optional
	LogLevel *string `json:"logLevel,omitempty"`
}

func (in *AgentGateway) GetEnabled() *bool {
	if in == nil {
		return nil
	}
	return in.Enabled
}

func (in *AgentGateway) GetLogLevel() *string {
	if in == nil {
		return nil
	}
	return in.LogLevel
}
