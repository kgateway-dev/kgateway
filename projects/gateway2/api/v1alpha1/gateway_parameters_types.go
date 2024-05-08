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
	Kube *KubernetesProxyConfig `json:"kube,omitempty"`
}
type GatewayParametersStatus struct {
}

// Configuration for the set of Kubernetes resources that will be provisioned
// for a given Gateway.
type KubernetesProxyConfig struct {
	Deployment *ProxyDeployment `json:"deployment,omitempty"`

	// Configuration for the container running Envoy.
	EnvoyContainer *EnvoyContainer `json:"envoyContainer,omitempty"`
	// Configuration for the pods that will be created.
	PodTemplate Pod `json:"podTemplate,omitempty"`
	// Configuration for the Kubernetes Service that exposes the Envoy proxy over
	// the network.
	Service Service `json:"service,omitempty"`
	// Autoscaling configuration.
	Autoscaling Autoscaling `json:"autoscaling,omitempty"`
}

type ProxyDeployment struct {
	// The number of desired pods. Defaults to 1.
	Replicas *uint32 `json:"replicas,omitempty"`
}

type EnvoyContainer struct {

	// Initial envoy configuration.
	Bootstrap EnvoyBootstrap `json:"bootstrap,omitempty"`
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
	Image Image `json:"image,omitempty"`
	// The security context for this container. See
	// https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.26/#securitycontext-v1-core
	// for details.
	SecurityContext *corev1.SecurityContext `json:"securityContext,omitempty"`
	// The compute resources required by this container. See
	// https://kubernetes.io/docs/concepts/configuration/manage-resources-containers/
	// for details.
	Resources ResourceRequirements `json:"resources,omitempty"`
}
type EnvoyBootstrap struct {

	// Envoy log level. Options include "trace", "debug", "info", "warn", "error",
	// "critical" and "off". Defaults to "info". See
	// https://www.envoyproxy.io/docs/envoy/latest/start/quick-start/run-envoy#debugging-envoy
	// for more information.
	LogLevel string `json:"logLevel,omitempty"`
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
	ComponentLogLevels map[string]string `json:"componentLogLevels,omitempty"`
}

func init() {
	SchemeBuilder.Register(&GatewayParameters{}, &GatewayParametersList{})
}
