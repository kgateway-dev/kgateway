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

	// If specified, the pod's tolerations. See
	// https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.26/#toleration-v1-core
	// for details.
	//
	// +kubebuilder:validation:Optional
	Tolerations []*corev1.Toleration `json:"tolerations,omitempty"`
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

func (in *Pod) GetTolerations() []*corev1.Toleration {
	if in == nil {
		return nil
	}
	return in.Tolerations
}
