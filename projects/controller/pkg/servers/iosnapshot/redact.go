package iosnapshot

import (
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
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

// redactGlooSecretData modifies the secret to remove any sensitive information
// The structure of a Secret in Gloo Gateway does not lend itself to easily redact data in different places.
// As a result, we perform a primitive redaction method, where we maintain the metadata, and remove the entire spec
func redactGlooSecretData(element *gloov1.Secret) {
	element.Kind = nil

	redactAnnotations(element.GetMetadata().GetAnnotations())
}

// redactGlooArtifactData modifies the artifact to remove any sensitive information
func redactGlooArtifactData(element *gloov1.Artifact) {
	for k := range element.GetData() {
		element.GetData()[k] = redactedString
	}

	redactAnnotations(element.GetMetadata().GetAnnotations())
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
