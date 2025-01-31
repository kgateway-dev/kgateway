package admin

import (
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	redactedString = "<redacted>"
)

// redactClientObject modifies the Object to remove any unwanted fields
func redactClientObject(object client.Object) {
	// ManagedFields is noise on the object, that is not relevant to the Admin API, so we sanitize it
	object.SetManagedFields(nil)

	redactKubeSecretData(object)
}

// RedactSecretData returns a copy with sensitive information redacted
func redactKubeSecretData(obj client.Object) {
	secret, ok := obj.(*corev1.Secret)
	if !ok {
		return
	}

	for k := range secret.Data {
		secret.Data[k] = []byte(redactedString)
	}

	// Also need to check for kubectl apply, last applied config.
	// Secret data can be found there as well if that's how the secret is created
	redactAnnotations(secret.GetAnnotations())
}

// redactGlooResourceMetadata modifies the metadata to remove any sensitive information
// ref: https://github.com/solo-io/skv2/blob/1583cb716c04eb3f8d01ecb179b0deeabaa6e42b/contrib/pkg/snapshot/redact.go#L20-L26
func redactAnnotations(annotations map[string]string) {
	for key, _ := range annotations {
		if key == corev1.LastAppliedConfigAnnotation {
			annotations[key] = redactedString
			break
		}
	}
}
