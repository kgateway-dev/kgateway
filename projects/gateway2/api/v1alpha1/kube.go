package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
)

// Kubernetes autoscaling configuration.
type Autoscaling struct {
	// If set, a Kubernetes HorizontalPodAutoscaler will be created to scale the
	// workload to match demand. See
	// https://kubernetes.io/docs/tasks/run-application/horizontal-pod-autoscale/
	// for details.
	HorizontalPodAutoscaler *HorizontalPodAutoscaler `json:"horizontal_pod_autoscaler,omitempty"`
}

// Horizontal pod autoscaling configuration. See
// https://kubernetes.io/docs/tasks/run-application/horizontal-pod-autoscale/
// for details.
type HorizontalPodAutoscaler struct {
	// The lower limit for the number of replicas to which the autoscaler can
	// scale down. Defaults to 1.
	MinReplicas *uint32 `json:"min_replicas,omitempty"`
	// The upper limit for the number of replicas to which the autoscaler can
	// scale up. Cannot be less than `minReplicas`. Defaults to 100.
	MaxReplicas *uint32 `json:"max_replicas,omitempty"`
	// The target value of the average CPU utilization across all relevant pods,
	// represented as a percentage of the requested value of the resource for the
	// pods. Defaults to 80.
	TargetCpuUtilizationPercentage *uint32 `json:"target_cpu_utilization_percentage,omitempty"`
	// The target value of the average memory utilization across all relevant
	// pods, represented as a percentage of the requested value of the resource
	// for the pods. Defaults to 80.
	TargetMemoryUtilizationPercentage *uint32 `json:"target_memory_utilization_percentage,omitempty"`
}

// A container image. See https://kubernetes.io/docs/concepts/containers/images
// for details.
type Image struct {
	// The image registry.
	Registry string `json:"registry,omitempty"`
	// The image repository (name).
	Repository string `json:"repository,omitempty"`
	// The image tag.
	Tag string `json:"tag,omitempty"`
	// The hash digest of the image, e.g. `sha256:12345...`
	Digest string `json:"digest,omitempty"`
	// The image pull policy for the container. See
	// https://kubernetes.io/docs/concepts/containers/images/#image-pull-policy
	// for details.
	PullPolicy corev1.PullPolicy `json:"pull_policy,omitempty"`
}

// Compute resources required by this container. See
// https://kubernetes.io/docs/concepts/configuration/manage-resources-containers/
// for details.
type ResourceRequirements struct {
	// The maximum amount of compute resources allowed.
	Limits map[string]string `json:"limits,omitempty"`
	// The minimum amount of compute resources required.
	Requests map[string]string `json:"requests,omitempty"`
}

// Configuration for a Kubernetes Service.
type Service struct {

	// The Kubernetes Service type.
	Type corev1.ServiceType `json:"type,omitempty"`
	// The manually specified IP address of the service, if a randomly assigned
	// IP is not desired. See
	// https://kubernetes.io/docs/concepts/services-networking/service/#choosing-your-own-ip-address
	// and
	// https://kubernetes.io/docs/concepts/services-networking/service/#headless-services
	// on the implications of setting `clusterIP`.
	ClusterIP string `json:"clusterIP,omitempty"`
	// Additional labels to add to the Service object metadata.
	ExtraLabels map[string]string `json:"extra_labels,omitempty"`
	// Additional annotations to add to the Service object metadata.
	ExtraAnnotations map[string]string `json:"extra_annotations,omitempty"`
}

// Configuration for a Kubernetes Pod template.
type Pod struct {
	// Additional labels to add to the Pod object metadata.
	ExtraLabels map[string]string `json:"extra_labels,omitempty"`
	// Additional annotations to add to the Pod object metadata.
	ExtraAnnotations map[string]string `json:"extra_annotations,omitempty"`
	// The pod security context. See
	// https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.26/#podsecuritycontext-v1-core
	// for details.
	SecurityContext *corev1.PodSecurityContext `json:"security_context,omitempty"`
	// An optional list of references to secrets in the same namespace to use for
	// pulling any of the images used by this Pod spec. See
	// https://kubernetes.io/docs/concepts/containers/images/#specifying-imagepullsecrets-on-a-pod
	// for details.
	ImagePullSecrets []corev1.LocalObjectReference `json:"image_pull_secrets,omitempty"`
	// A selector which must be true for the pod to fit on a node. See
	// https://kubernetes.io/docs/concepts/configuration/assign-pod-node/ for
	// details.
	NodeSelector map[string]string `json:"node_selector,omitempty" protobuf_key:"bytes,1,opt,name=key,proto3"`
	// If specified, the pod's scheduling constraints. See
	// https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.26/#affinity-v1-core
	// for details.
	Affinity *corev1.Affinity `json:"affinity,omitempty"`
	// If specified, the pod's tolerations. See
	// https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.26/#toleration-v1-core
	// for details.
	Tolerations []*corev1.Toleration `json:"tolerations,omitempty"`
}
