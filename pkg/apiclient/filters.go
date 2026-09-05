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
// allow-list would be wrong. Instead, we exclude high-volume types that are
// never referenced by Gateway API or kgateway CRDs. In clusters with many
// workloads these typically account for the bulk of Secret memory cost.
//
// Excluded types, and why each is safe:
//   - helm.sh/release.v1: Helm release storage. Never referenced, and large,
//     since each entry is a gzipped copy of a rendered release.
//   - kubernetes.io/service-account-token: legacy ServiceAccount tokens. Never
//     referenced, and present once per ServiceAccount on older clusters.
//   - kubernetes.io/dockerconfigjson and kubernetes.io/dockercfg: image pull
//     secrets. GatewayParameters exposes imagePullSecrets, but only as
//     LocalObjectReferences that are copied into the pod spec by name; kgateway
//     never reads their contents.
//   - bootstrap.kubernetes.io/token: node bootstrap tokens in kube-system.
//     Never referenced.
//
// Note that this only narrows what the API server sends us. Excluding a type here
// means a Secret of that type can never be resolved, so only add types that no
// kgateway or Gateway API field can reference.
var SecretsFieldSelector = fields.AndSelectors(
	fields.OneTermNotEqualSelector("type", "helm.sh/release.v1"),
	fields.OneTermNotEqualSelector("type", string(corev1.SecretTypeServiceAccountToken)),
	fields.OneTermNotEqualSelector("type", string(corev1.SecretTypeDockerConfigJson)),
	fields.OneTermNotEqualSelector("type", string(corev1.SecretTypeDockercfg)),
	fields.OneTermNotEqualSelector("type", string(corev1.SecretTypeBootstrapToken))).String()
