package apiclient

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
)

// SecretsFieldSelector narrows the informer cache by excluding Secret types
// that kgateway is known never to reference. It is a best-effort memory
// optimization only; the code would behave correctly if we watched all Secrets.
//
// kgateway references Secrets of many types (TLS listener certificates,
// backend TLS material, the OAuth2 HMAC key, API-key auth secrets selected
// by TrafficPolicy, and other user-defined types), so a positive type
// allow-list would be wrong. Instead, we exclude two high-volume types that
// are never referenced by Gateway API or kgateway CRDs: Helm release storage
// and ServiceAccount token secrets. In clusters with many workloads these
// two types typically account for the bulk of Secret memory cost.
var SecretsFieldSelector = fields.AndSelectors(
	fields.OneTermNotEqualSelector("type", "helm.sh/release.v1"),
	fields.OneTermNotEqualSelector("type", string(corev1.SecretTypeServiceAccountToken))).String()

// StripUnusedMetadata is an informer ObjectTransform for Secrets and ConfigMaps.
// It drops metadata that kgateway never reads but that can dominate the informer
// cache in clusters with many workloads:
//
//   - managedFields. Istio's default transform already strips this, but supplying
//     an ObjectTransform replaces that default, so we must repeat it here.
//   - the kubectl.kubernetes.io/last-applied-configuration annotation, which holds
//     a full serialized copy of the object and therefore roughly doubles the cache
//     cost of every Secret and ConfigMap managed by `kubectl apply`.
//
// This only rewrites the informer's cached copy. kgateway never writes back a
// Secret or ConfigMap that it read from a cache, so the stripped fields cannot
// propagate to the API server. A side benefit is that updates touching only these
// fields no longer produce a krt event, avoiding spurious retranslation.
func StripUnusedMetadata(obj any) (any, error) {
	t, ok := obj.(metav1.ObjectMetaAccessor)
	if !ok {
		// Tombstones and unexpected types pass through unchanged, matching the
		// upstream Istio transform.
		return obj, nil
	}
	meta := t.GetObjectMeta()
	meta.SetManagedFields(nil)
	delete(meta.GetAnnotations(), corev1.LastAppliedConfigAnnotation)
	return obj, nil
}
