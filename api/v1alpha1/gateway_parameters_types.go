package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
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
// +kubebuilder:validation:ExactlyOneOf=kube;selfManaged
type GatewayParametersSpec struct {
	// The proxy will be deployed on Kubernetes.
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

// KubernetesProxyConfig configures the set of Kubernetes resources that will be provisioned
// for a given Gateway.
type KubernetesProxyConfig struct {
	// Use a Kubernetes deployment as the proxy workload type. Currently, this is the only
	// supported workload type.
	//
	// +optional
	Deployment *ProxyDeployment `json:"deployment,omitempty"`

	// Configuration for the container running Envoy.
	// If AgentGateway is enabled, the EnvoyContainer values will be ignored.
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

	// Configuration for the Kubernetes Service that exposes the Envoy proxy over
	// the network.
	//
	// +optional
	Service *Service `json:"service,omitempty"`

	// Configuration for the Kubernetes ServiceAccount used by the Envoy pod.
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

	// Configuration for the AI extension.
	//
	// +optional
	AiExtension *AiExtension `json:"aiExtension,omitempty"`

	// Configure the AgentGateway integration. If AgentGateway is disabled, the EnvoyContainer values will be used by
	// default to configure the data plane proxy.
	//
	// +optional
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

// ProxyDeployment configures the Proxy deployment in Kubernetes.
type ProxyDeployment struct {
	// The number of desired pods. Defaults to 1.
	//
	// +optional
	Replicas *uint32 `json:"replicas,omitempty"`
}

func (in *ProxyDeployment) GetReplicas() *uint32 {
	if in == nil {
		return nil
	}
	return in.Replicas
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

// IstioIntegration configures the Istio integration settings used by a kgateway's data plane (Envoy proxy instance)
type IstioIntegration struct {
	// Configuration for the container running istio-proxy.
	// Note that if Istio integration is not enabled, the istio container will not be injected
	// into the gateway proxy deployment.
	//
	// +optional
	IstioProxyContainer *IstioContainer `json:"istioProxyContainer,omitempty"`

	// do not use slice of pointers: https://github.com/kubernetes/code-generator/issues/166
	// Override the default Istio sidecar in gateway-proxy with a custom container.
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
	// The envoy container image. See
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
	// +optional
	Enabled *bool `json:"enabled,omitempty"`

	// The extension's container image. See
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

	// do not use slice of pointers: https://github.com/kubernetes/code-generator/issues/166

	// The extension's container environment variables.
	//
	// +optional
	Env []corev1.EnvVar `json:"env,omitempty"`

	// The extension's container ports.
	//
	// +optional
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

	// Additional OTel tracing config for AI Extension.
	//
	// +optional
	Tracing *AiExtensionTrace `json:"tracing,omitempty"`
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

func (in *AiExtension) GetTracing() *AiExtensionTrace {
	if in == nil {
		return nil
	}
	return in.Tracing
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

// AiExtensionTrace defines the tracing configuration for the AI extension
type AiExtensionTrace struct {
	// EndPoint specifies the URL of the OTLP Exporter for traces.
	// Example: "http://my-otel-collector.svc.cluster.local:4317"
	// https://opentelemetry.io/docs/languages/sdk-configuration/otlp-exporter/#otel_exporter_otlp_traces_endpoint
	//
	// +required
	EndPoint gwv1.AbsoluteURI `json:"endpoint"`

	// Sampler defines the sampling strategy for OpenTelemetry traces.
	// Sampling helps in reducing the volume of trace data by selectively
	// recording only a subset of traces.
	// https://opentelemetry.io/docs/languages/sdk-configuration/general/#otel_traces_sampler
	//
	// +optional
	Sampler *OTelTracesSampler `json:"sampler,omitempty"`

	// OTLPTimeout specifies timeout configurations for OTLP (OpenTelemetry Protocol) exports.
	// It allows setting general and trace-specific timeouts for sending data.
	// https://opentelemetry.io/docs/languages/sdk-configuration/otlp-exporter/#otel_exporter_otlp_traces_timeout
	//
	// +optional
	Timeout *gwv1.Duration `json:"timeout,omitempty"`

	// OTLPProtocol specifies the protocol to be used for OTLP exports.
	// This determines how tracing data is serialized and transported (e.g., gRPC, HTTP/Protobuf).
	// https://opentelemetry.io/docs/languages/sdk-configuration/otlp-exporter/#otel_exporter_otlp_traces_protocol
	//
	// +optional
	// +kubebuilder:validation:Enum=grpc;http/protobuf;http/json
	Protocol *OTLPTracesProtocolType `json:"protocol,omitempty"`

	// TransportSecurity controls the TLS (Transport Layer Security) settings when connecting
	// to the tracing server. It determines whether certificate verification should be skipped.
	//
	// +optional
	// +kubebuilder:validation:Enum=secure;insecure
	TransportSecurity *OTLPTransportSecurityMode `json:"transportSecurity,omitempty"`
}

func (in *AiExtensionTrace) GetTimeout() *gwv1.Duration {
	if in == nil {
		return nil
	}
	return in.Timeout
}

// OTelTracesSamplerType defines the available OpenTelemetry trace sampler types.
// These samplers determine which traces are recorded and exported.
type OTelTracesSamplerType string

const (
	// OTelTracesSamplerAlwaysOn enables always-on sampling.
	// All traces will be recorded and exported. Useful for development or low-traffic systems.
	OTelTracesSamplerAlwaysOn OTelTracesSamplerType = "alwaysOn"

	// OTelTracesSamplerAlwaysOff enables always-off sampling.
	// No traces will be recorded or exported. Effectively disables tracing.
	OTelTracesSamplerAlwaysOff OTelTracesSamplerType = "alwaysOff"

	// OTelTracesSamplerTraceidratio enables trace ID ratio based sampling.
	// Traces are sampled based on a configured probability derived from their trace ID.
	OTelTracesSamplerTraceidratio OTelTracesSamplerType = "traceidratio"

	// OTelTracesSamplerParentbasedAlwaysOn enables parent-based always-on sampling.
	// If a parent span exists and is sampled, the child span is also sampled.
	OTelTracesSamplerParentbasedAlwaysOn OTelTracesSamplerType = "parentbasedAlwaysOn"

	// OTelTracesSamplerParentbasedAlwaysOff enables parent-based always-off sampling.
	// If a parent span exists and is not sampled, the child span is also not sampled.
	OTelTracesSamplerParentbasedAlwaysOff OTelTracesSamplerType = "parentbasedAlwaysOff"

	// OTelTracesSamplerParentbasedTraceidratio enables parent-based trace ID ratio sampling.
	// If a parent span exists and is sampled, the child span is also sampled.
	OTelTracesSamplerParentbasedTraceidratio OTelTracesSamplerType = "parentbasedTraceidratio"
)

func (otelSamplerType OTelTracesSamplerType) String() string {
	switch otelSamplerType {
	case OTelTracesSamplerAlwaysOn:
		return "alwaysOn"
	case OTelTracesSamplerAlwaysOff:
		return "alwaysOff"
	case OTelTracesSamplerTraceidratio:
		return "traceidratio"
	case OTelTracesSamplerParentbasedAlwaysOn:
		return "parentbasedAlwaysOn"
	case OTelTracesSamplerParentbasedAlwaysOff:
		return "parentbasedAlwaysOff"
	case OTelTracesSamplerParentbasedTraceidratio:
		return "parentbasedTraceidratio"
	default:
		return ""
	}
}

// OTelTracesSampler defines the configuration for an OpenTelemetry trace sampler.
// It combines the sampler type with any required arguments for that type.
type OTelTracesSampler struct {
	// SamplerType specifies the type of sampler to use (default value: "parentbased_always_on").
	// Refer to OTelTracesSamplerType for available options.
	// https://opentelemetry.io/docs/languages/sdk-configuration/general/#otel_traces_sampler
	//
	//+optional
	// +kubebuilder:validation:Enum=alwaysOn;alwaysOff;traceidratio;parentbasedAlwaysOn;parentbasedAlwaysOff;parentbasedTraceidratio
	SamplerType *OTelTracesSamplerType `json:"type,omitempty"`
	// SamplerArg provides an argument for the chosen sampler type.
	// For "traceidratio" or "parentbased_traceidratio" samplers: Sampling probability, a number in the [0..1] range,
	// e.g. 0.25. Default is 1.0 if unset.
	// https://opentelemetry.io/docs/languages/sdk-configuration/general/#otel_traces_sampler_arg
	//
	//+optional
	// +kubebuilder:validation:Pattern=`^0(\.\d+)?|1(\.0+)?$`
	SamplerArg *string `json:"arg,omitempty"`
}

func (in *AiExtensionTrace) GetSampler() *OTelTracesSampler {
	if in == nil {
		return nil
	}
	return in.Sampler
}

func (in *AiExtensionTrace) GetSamplerType() *string {
	if in == nil || in.Sampler == nil || in.Sampler.SamplerType == nil {
		return nil
	}
	value := in.Sampler.SamplerType.String()
	return &value
}

func (in *AiExtensionTrace) GetSamplerArg() *string {
	if in == nil || in.Sampler == nil {
		return nil
	}
	return in.GetSampler().SamplerArg
}

// OTLPTracesProtocolType defines the supported protocols for OTLP exporter.
type OTLPTracesProtocolType string

const (
	// OTLPTracesProtocolTypeGrpc specifies OTLP over gRPC protocol.
	// This is typically the most efficient protocol for OpenTelemetry data transfer.
	OTLPTracesProtocolTypeGrpc OTLPTracesProtocolType = "grpc"
	// OTLPTracesProtocolTypeProtobuf specifies OTLP over HTTP with Protobuf serialization.
	// Data is sent via HTTP POST requests with Protobuf message bodies.
	OTLPTracesProtocolTypeProtobuf OTLPTracesProtocolType = "http/protobuf"
	// OTLPTracesProtocolTypeJson specifies OTLP over HTTP with JSON serialization.
	// Data is sent via HTTP POST requests with JSON message bodies.
	OTLPTracesProtocolTypeJson OTLPTracesProtocolType = "http/json"
)

func (in *AiExtensionTrace) GetOTLPProtocolType() *string {
	if in == nil || in.Protocol == nil {
		return nil
	}
	value := in.Protocol.String()
	return &value
}

func (otelProtocolType OTLPTracesProtocolType) String() string {
	switch otelProtocolType {
	case OTLPTracesProtocolTypeGrpc:
		return "grpc"
	case OTLPTracesProtocolTypeProtobuf:
		return "http/protobuf"
	case OTLPTracesProtocolTypeJson:
		return "http/json"
	default:
		return ""
	}
}

// OTLPTransportSecurityMode defines the transport security options for OTLP connections.
type OTLPTransportSecurityMode string

const (
	// OTLPTransportSecuritySecure enables TLS (client transport security) for OTLP connections.
	// This means the client will verify the server's certificate.
	OTLPTransportSecuritySecure OTLPTransportSecurityMode = "secure"

	// OTLPTransportSecurityInsecure disables TLS for OTLP connections,
	// meaning certificate verification is skipped. This is generally not recommended
	// for production environments due to security risks.
	OTLPTransportSecurityInsecure OTLPTransportSecurityMode = "insecure"
)

func (otelTransportSecurityMode OTLPTransportSecurityMode) String() string {
	switch otelTransportSecurityMode {
	case OTLPTransportSecuritySecure:
		return "secure"
	case OTLPTransportSecurityInsecure:
		return "insecure"
	default:
		return ""
	}
}

func (in *AiExtensionTrace) GetTransportSecurityMode() *string {
	if in == nil || in.TransportSecurity == nil {
		return nil
	}
	value := in.TransportSecurity.String()
	return &value
}

// AgentGateway configures the AgentGateway integration. If AgentGateway is enabled, Envoy
type AgentGateway struct {
	// Whether to enable the extension.
	//
	// +optional
	Enabled *bool `json:"enabled,omitempty"`

	// Log level for the agentgateway. Defaults to info.
	// Levels include "trace", "debug", "info", "error", "warn". See: https://docs.rs/tracing/latest/tracing/struct.Level.html
	//
	// +optional
	LogLevel *string `json:"logLevel,omitempty"`

	// The agentgateway container image. See
	// https://kubernetes.io/docs/concepts/containers/images
	// for details.
	//
	// Default values, which may be overridden individually:
	//
	//	registry: ghcr.io/agentgateway
	//	repository: agentgateway
	//	tag: <agentgateway version>
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

func (in *AgentGateway) GetImage() *Image {
	if in == nil {
		return nil
	}
	return in.Image
}

func (in *AgentGateway) GetSecurityContext() *corev1.SecurityContext {
	if in == nil {
		return nil
	}
	return in.SecurityContext
}

func (in *AgentGateway) GetResources() *corev1.ResourceRequirements {
	if in == nil {
		return nil
	}
	return in.Resources
}

func (in *AgentGateway) GetEnv() []corev1.EnvVar {
	if in == nil {
		return nil
	}
	return in.Env
}
