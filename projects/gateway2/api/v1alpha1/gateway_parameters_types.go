package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +kubebuilder:object:root=true
// +kubebuilder:resource:categories=gateway-gloo,shortName=gwp
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

type GatewayParametersSpec struct {
	SelfManaged *SelfManagedGateway    `json:"selfManaged,omitempty"`
	Kube        *KubernetesProxyConfig `json:"kube,omitempty"`
}

type GatewayParametersStatus struct {
}

type SelfManagedGateway struct {
}

// Configuration for the set of Kubernetes resources that will be provisioned
// for a given Gateway.
type KubernetesProxyConfig struct {
	// +optional
	Deployment *ProxyDeployment `json:"deployment,omitempty"`

	// Configuration for the container running Envoy.
	// +optional
	EnvoyContainer *EnvoyContainer `json:"envoyContainer,omitempty"`
	// Configuration for the container running the Secret Discovery Service (SDS).
	// +optional
	SdsContainer *SdsContainer `json:"sdsContainer,omitempty"`
	// Configuration for the pods that will be created.
	// +optional
	PodTemplate *Pod `json:"podTemplate,omitempty"`
	// Configuration for the Kubernetes Service that exposes the Envoy proxy over
	// the network.
	// +optional
	Service *Service `json:"service,omitempty"`
	// Autoscaling configuration.
	// Autoscaling Autoscaling `json:"autoscaling,omitempty"`
	// Configuration for the Istio integration.
	// +optional
	Istio *IstioIntegration `json:"istioIntegration,omitempty"`
	// Configuration for the stats server.
	// +optional
	Stats *StatsConfig `json:"statsConfig,omitempty"`
	// Configuration for the AI extension.
	// +optional
	AiExtension *AiExtension `json:"aiExtension,omitempty"`
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

type ProxyDeployment struct {
	// The number of desired pods. Defaults to 1.
	// +optional
	Replicas *uint32 `json:"replicas,omitempty"`
}

func (in *ProxyDeployment) GetReplicas() *uint32 {
	if in == nil {
		return nil
	}
	return in.Replicas
}

type EnvoyContainer struct {

	// Initial envoy configuration.
	// +optional
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
	// +optional
	Image *Image `json:"image,omitempty"`
	// The security context for this container. See
	// https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.26/#securitycontext-v1-core
	// for details.
	// +optional
	SecurityContext *corev1.SecurityContext `json:"securityContext,omitempty"`
	// The compute resources required by this container. See
	// https://kubernetes.io/docs/concepts/configuration/manage-resources-containers/
	// for details.
	// +optional
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

type EnvoyBootstrap struct {

	// Envoy log level. Options include "trace", "debug", "info", "warn", "error",
	// "critical" and "off". Defaults to "info". See
	// https://www.envoyproxy.io/docs/envoy/latest/start/quick-start/run-envoy#debugging-envoy
	// for more information.
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

type SdsContainer struct {
	Image           *Image                       `json:"image,omitempty"`
	SecurityContext *corev1.SecurityContext      `json:"securityContext,omitempty"`
	Resources       *corev1.ResourceRequirements `json:"resources,omitempty"`
	Bootstrap       *SdsBootstrap                `json:"bootstrap,omitempty"`
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

type SdsBootstrap struct {
	LogLevel *string `json:"logLevel,omitempty"`
}

func (in *SdsBootstrap) GetLogLevel() *string {
	if in == nil {
		return nil
	}
	return in.LogLevel
}

type IstioIntegration struct {
	// +optional
	IstioProxyContainer *IstioContainer `json:"istioProxyContainer,omitempty"`
	// +optional
	CustomSidecars []*corev1.Container `json:"customSidecars,omitempty"`
}

func (in *IstioIntegration) GetIstioProxyContainer() *IstioContainer {
	if in == nil {
		return nil
	}
	return in.IstioProxyContainer
}

func (in *IstioIntegration) GetCustomSidecars() []*corev1.Container {
	if in == nil {
		return nil
	}
	return in.CustomSidecars
}

type IstioContainer struct {
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
	// +optional
	Image *Image `json:"image,omitempty"`
	// The security context for this container. See
	// https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.26/#securitycontext-v1-core
	// for details.
	// +optional
	SecurityContext *corev1.SecurityContext `json:"securityContext,omitempty"`
	// The compute resources required by this container. See
	// https://kubernetes.io/docs/concepts/configuration/manage-resources-containers/
	// for details.
	// +optional
	Resources *corev1.ResourceRequirements `json:"resources,omitempty"`

	// +optional
	LogLevel *string `json:"logLevel,omitempty"`

	// +optional
	IstioDiscoveryAddress *string `json:"istioDiscoveryAddress,omitempty"`

	// +optional
	IstioMetaMeshId *string `json:"istioMetaMeshId,omitempty"`

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

type StatsConfig struct {
	// +optional
	Enabled *bool `json:"enabled,omitempty"`

	// +optional
	RoutePrefixRewrite *string `json:"routePrefixRewrite,omitempty"`

	// +optional
	EnableStatsRoute *bool `json:"enableStatsRoute,omitempty"`

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
	// +optional
	Enabled *bool `json:"enabled,omitempty"`
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
	// +optional
	Image *Image `json:"image,omitempty"`
	// The security context for this container. See
	// https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.26/#securitycontext-v1-core
	// for details.
	// +optional
	SecurityContext *corev1.SecurityContext `json:"securityContext,omitempty"`
	// The compute resources required by this container. See
	// https://kubernetes.io/docs/concepts/configuration/manage-resources-containers/
	// for details.
	// +optional
	Resources *corev1.ResourceRequirements `json:"resources,omitempty"`

	// +optional
	Env []*corev1.EnvVar `json:"env,omitempty"`

	// +optional
	Ports []*corev1.ContainerPort `json:"ports,omitempty"`
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

func (in *AiExtension) GetEnv() []*corev1.EnvVar {
	if in == nil {
		return nil
	}
	return in.Env
}

func (in *AiExtension) GetPorts() []*corev1.ContainerPort {
	if in == nil {
		return nil
	}
	return in.Ports
}

func init() {
	SchemeBuilder.Register(&GatewayParameters{}, &GatewayParametersList{})
}
