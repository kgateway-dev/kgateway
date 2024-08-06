package wellknown

import (
	corev1 "k8s.io/api/core/v1"
)

var (
	SecretGVK    = corev1.SchemeGroupVersion.WithKind("Secret")
	ConfigMapGVK = corev1.SchemeGroupVersion.WithKind("ConfigMap")
)
