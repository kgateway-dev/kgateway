package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
)

// A container image. See https://kubernetes.io/docs/concepts/containers/images
// for details.
type Image struct {
	// The image registry.
	//
	// +kubebuilder:validation:Optional
	Registry *string `json:"registry,omitempty"`

	// The image repository (name).
	//
	// +kubebuilder:validation:Optional
	Repository *string `json:"repository,omitempty"`

	// The image tag.
	//
	// +kubebuilder:validation:Optional
	Tag *string `json:"tag,omitempty"`

	// The hash digest of the image, e.g. `sha256:12345...`
	//
	// +kubebuilder:validation:Optional
	Digest *string `json:"digest,omitempty"`

	// The image pull policy for the container. See
	// https://kubernetes.io/docs/concepts/containers/images/#image-pull-policy
	// for details.
	//
	// +kubebuilder:validation:Optional
	PullPolicy *corev1.PullPolicy `json:"pullPolicy,omitempty"`
}

func (in *Image) GetRegistry() *string {
	if in == nil {
		return nil
	}
	return in.Registry
}

func (in *Image) GetRepository() *string {
	if in == nil {
		return nil
	}
	return in.Repository
}

func (in *Image) GetTag() *string {
	if in == nil {
		return nil
	}
	return in.Tag
}

func (in *Image) GetDigest() *string {
	if in == nil {
		return nil
	}
	return in.Digest
}

func (in *Image) GetPullPolicy() *corev1.PullPolicy {
	if in == nil {
		return nil
	}
	return in.PullPolicy
}

// Configuration for a Kubernetes Service.
type Service struct {
	// The Kubernetes Service type.
	//
	// +kubebuilder:validation:Optional
	Type *corev1.ServiceType `json:"type,omitempty"`

	// The manually specified IP address of the service, if a randomly assigned
	// IP is not desired. See
	// https://kubernetes.io/docs/concepts/services-networking/service/#choosing-your-own-ip-address
	// and
	// https://kubernetes.io/docs/concepts/services-networking/service/#headless-services
	// on the implications of setting `clusterIP`.
	//
	// +kubebuilder:validation:Optional
	ClusterIP *string `json:"clusterIP,omitempty"`

	// Additional labels to add to the Service object metadata.
	//
	// +kubebuilder:validation:Optional
	ExtraLabels map[string]string `json:"extraLabels,omitempty"`

	// Additional annotations to add to the Service object metadata.
	//
	// +kubebuilder:validation:Optional
	ExtraAnnotations map[string]string `json:"extraAnnotations,omitempty"`
}

func (in *Service) GetType() *corev1.ServiceType {
	if in == nil {
		return nil
	}
	return in.Type
}

func (in *Service) GetClusterIP() *string {
	if in == nil {
		return nil
	}
	return in.ClusterIP
}

func (in *Service) GetExtraLabels() map[string]string {
	if in == nil {
		return nil
	}
	return in.ExtraLabels
}

func (in *Service) GetExtraAnnotations() map[string]string {
	if in == nil {
		return nil
	}
	return in.ExtraAnnotations
}

type ServiceAccount struct {
	// Additional labels to add to the ServiceAccount object metadata.
	//
	// +kubebuilder:validation:Optional
	ExtraLabels map[string]string `json:"extraLabels,omitempty"`

	// Additional annotations to add to the ServiceAccount object metadata.
	//
	// +kubebuilder:validation:Optional
	ExtraAnnotations map[string]string `json:"extraAnnotations,omitempty"`
}

func (in *ServiceAccount) GetExtraLabels() map[string]string {
	if in == nil {
		return nil
	}
	return in.ExtraLabels
}

func (in *ServiceAccount) GetExtraAnnotations() map[string]string {
	if in == nil {
		return nil
	}
	return in.ExtraAnnotations
}

// Configuration for a Kubernetes Pod template.
type Pod struct {
	// Additional labels to add to the Pod object metadata.
	//
	// +kubebuilder:validation:Optional
	ExtraLabels map[string]string `json:"extraLabels,omitempty"`

	// Additional annotations to add to the Pod object metadata.
	//
	// +kubebuilder:validation:Optional
	ExtraAnnotations map[string]string `json:"extraAnnotations,omitempty"`

	// The pod security context. See
	// https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.26/#podsecuritycontext-v1-core
	// for details.
	//
	// +kubebuilder:validation:Optional
	SecurityContext *corev1.PodSecurityContext `json:"securityContext,omitempty"`

	// An optional list of references to secrets in the same namespace to use for
	// pulling any of the images used by this Pod spec. See
	// https://kubernetes.io/docs/concepts/containers/images/#specifying-imagepullsecrets-on-a-pod
	// for details.
	//
	// +kubebuilder:validation:Optional
	ImagePullSecrets []corev1.LocalObjectReference `json:"imagePullSecrets,omitempty"`

	// A selector which must be true for the pod to fit on a node. See
	// https://kubernetes.io/docs/concepts/configuration/assign-pod-node/ for
	// details.
	//
	// +kubebuilder:validation:Optional
	NodeSelector map[string]string `json:"nodeSelector,omitempty"`

	// If specified, the pod's scheduling constraints. See
	// https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.26/#affinity-v1-core
	// for details.
	//
	// +kubebuilder:validation:Optional
	Affinity *corev1.Affinity `json:"affinity,omitempty"`

	// do not use slice of pointers: https://github.com/kubernetes/code-generator/issues/166
	// If specified, the pod's tolerations. See
	// https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.26/#toleration-v1-core
	// for details.
	//
	// +kubebuilder:validation:Optional
	Tolerations []corev1.Toleration `json:"tolerations,omitempty"`

	// If specified, the pod's graceful shutdown spec.
	//
	// +kubebuilder:validation:Optional
	GracefulShutdown *GracefulShutdownSpec `json:"gracefulShutdown,omitempty"`

	// If specified, the pod's termination grace period in seconds. See
	// https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.26/#pod-v1-core
	// for details
	//
	// +kubebuilder:validation:Optional
	TerminationGracePeriodSeconds *int `json:"terminationGracePeriodSeconds,omitempty"`

	// If specified, the pod's readiness probe. Periodic probe of container service readiness.
	// Container will be removed from service endpoints if the probe fails. See
	// https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.26/#probe-v1-core
	// for details.
	//
	// +kubebuilder:validation:Optional
	ReadinessProbe *corev1.Probe `json:"readinessProbe,omitempty"`

	// If specified, the pod's liveness probe. Periodic probe of container service readiness.
	// Container will be restarted if the probe fails. See
	// https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.26/#probe-v1-core
	// for details.
	//
	// +kubebuilder:validation:Optional
	LivenessProbe *corev1.Probe `json:"livenessProbe,omitempty"`
}

func (in *Pod) GetExtraLabels() map[string]string {
	if in == nil {
		return nil
	}
	return in.ExtraLabels
}

func (in *Pod) GetExtraAnnotations() map[string]string {
	if in == nil {
		return nil
	}
	return in.ExtraAnnotations
}

func (in *Pod) GetSecurityContext() *corev1.PodSecurityContext {
	if in == nil {
		return nil
	}
	return in.SecurityContext
}

func (in *Pod) GetImagePullSecrets() []corev1.LocalObjectReference {
	if in == nil {
		return nil
	}
	return in.ImagePullSecrets
}

func (in *Pod) GetNodeSelector() map[string]string {
	if in == nil {
		return nil
	}
	return in.NodeSelector
}

func (in *Pod) GetAffinity() *corev1.Affinity {
	if in == nil {
		return nil
	}
	return in.Affinity
}

func (in *Pod) GetTolerations() []corev1.Toleration {
	if in == nil {
		return nil
	}
	return in.Tolerations
}

func (in *Pod) GetReadinessProbe() *corev1.Probe {
	if in == nil {
		return nil
	}
	return in.ReadinessProbe
}

func (in *Pod) GetGracefulShutdown() *GracefulShutdownSpec {
	if in == nil {
		return nil
	}
	return in.GracefulShutdown
}

func (in *Pod) GetTerminationGracePeriodSeconds() *int {
	if in == nil {
		return nil
	}
	return in.TerminationGracePeriodSeconds
}

func (in *Pod) GetLivenessProbe() *corev1.Probe {
	if in == nil {
		return nil
	}
	return in.LivenessProbe
}

type GracefulShutdownSpec struct {
	// Enable grace period before shutdown to finish current requests while Envoy health checks fail to e.g. notify external load balancers. *NOTE:* This will not have any effect if you have not defined health checks via the health check filter
	//
	// +kubebuilder:validation:Optional
	Enabled *bool `json:"enabled,omitempty"`

	// Time (in seconds) for the preStop hook to wait before allowing Envoy to terminate
	//
	// +kubebuilder:validation:Optional
	SleepTimeSeconds *int `json:"sleepTimeSeconds,omitempty"`
}

func (in *GracefulShutdownSpec) GetEnabled() *bool {
	if in == nil {
		return nil
	}
	return in.Enabled
}

func (in *GracefulShutdownSpec) GetSleepTimeSeconds() *int {
	if in == nil {
		return nil
	}
	return in.SleepTimeSeconds
}
