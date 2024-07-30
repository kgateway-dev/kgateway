package wellknown

import (
	corev1 "k8s.io/api/core/v1"
<<<<<<< HEAD
)

var (
	SecretGVK    = corev1.SchemeGroupVersion.WithKind("Secret")
	ConfigMapGVK = corev1.SchemeGroupVersion.WithKind("ConfigMap")
=======
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var (
	SecretGVK = schema.GroupVersionKind{
		Group:   corev1.GroupName,
		Version: "v1",
		Kind:    "Secret",
	}
>>>>>>> main
)
