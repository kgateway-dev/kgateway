package apiclient

import (
	corev1 "k8s.io/api/core/v1"
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
