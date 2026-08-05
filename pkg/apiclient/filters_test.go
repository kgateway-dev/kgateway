package apiclient

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
)

func TestSecretsFieldSelector(t *testing.T) {
	selector, err := fields.ParseSelector(SecretsFieldSelector)
	if err != nil {
		t.Fatalf("parse SecretsFieldSelector: %v", err)
	}

	tests := []struct {
		name       string
		secretType corev1.SecretType
		want       bool
	}{
		{"tls secret is watched", corev1.SecretTypeTLS, true},
		{"opaque secret is watched", corev1.SecretTypeOpaque, true},
		{"basic auth secret is watched", corev1.SecretTypeBasicAuth, true},
		{"custom user-defined type is watched", corev1.SecretType("example.com/custom"), true},
		{"empty type is watched", corev1.SecretType(""), true},
		{"helm release secret is filtered out", corev1.SecretType("helm.sh/release.v1"), false},
		{"service account token secret is filtered out", corev1.SecretTypeServiceAccountToken, false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			set := fields.Set{"type": string(tc.secretType)}
			got := selector.Matches(set)
			if got != tc.want {
				t.Errorf("selector.Matches(type=%q) = %v, want %v", tc.secretType, got, tc.want)
			}
		})
	}
}

func TestStripUnusedMetadata(t *testing.T) {
	t.Run("strips managedFields and last-applied-configuration", func(t *testing.T) {
		cm := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "ca",
				Namespace: "app",
				Annotations: map[string]string{
					corev1.LastAppliedConfigAnnotation: `{"kind":"ConfigMap","data":{"ca.crt":"..."}}`,
					"example.com/keep":                 "yes",
				},
				Labels:        map[string]string{"example.com/label": "yes"},
				ManagedFields: []metav1.ManagedFieldsEntry{{Manager: "kubectl"}},
			},
			Data: map[string]string{"ca.crt": "..."},
		}

		out, err := StripUnusedMetadata(cm)
		if err != nil {
			t.Fatalf("StripUnusedMetadata: %v", err)
		}
		got, ok := out.(*corev1.ConfigMap)
		if !ok {
			t.Fatalf("StripUnusedMetadata returned %T, want *corev1.ConfigMap", out)
		}

		if got.ManagedFields != nil {
			t.Errorf("managedFields = %v, should be stripped", got.ManagedFields)
		}
		if _, present := got.Annotations[corev1.LastAppliedConfigAnnotation]; present {
			t.Error("last-applied-configuration annotation should be stripped")
		}
		if got.Annotations["example.com/keep"] != "yes" {
			t.Errorf("unrelated annotations should be preserved, got %v", got.Annotations)
		}
		if got.Labels["example.com/label"] != "yes" {
			t.Errorf("labels should be preserved, got %v", got.Labels)
		}
		if got.Data["ca.crt"] != "..." {
			t.Errorf("data should be preserved, got %v", got.Data)
		}
		if got.Name != "ca" || got.Namespace != "app" {
			t.Errorf("identity should be preserved, got %s/%s", got.Namespace, got.Name)
		}
	})

	t.Run("secret data is preserved", func(t *testing.T) {
		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:        "tls",
				Namespace:   "app",
				Annotations: map[string]string{corev1.LastAppliedConfigAnnotation: "{}"},
			},
			Type: corev1.SecretTypeTLS,
			Data: map[string][]byte{"tls.crt": []byte("cert"), "tls.key": []byte("key")},
		}

		out, err := StripUnusedMetadata(secret)
		if err != nil {
			t.Fatalf("StripUnusedMetadata: %v", err)
		}
		got := out.(*corev1.Secret)
		if _, present := got.Annotations[corev1.LastAppliedConfigAnnotation]; present {
			t.Error("last-applied-configuration annotation should be stripped")
		}
		if string(got.Data["tls.crt"]) != "cert" || string(got.Data["tls.key"]) != "key" {
			t.Errorf("secret data should be preserved, got %v", got.Data)
		}
		if got.Type != corev1.SecretTypeTLS {
			t.Errorf("type should be preserved, got %q", got.Type)
		}
	})

	t.Run("nil annotations are tolerated", func(t *testing.T) {
		cm := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "no-annotations"}}
		if _, err := StripUnusedMetadata(cm); err != nil {
			t.Fatalf("StripUnusedMetadata: %v", err)
		}
	})

	t.Run("non-object types pass through", func(t *testing.T) {
		tombstone := "not-an-object"
		out, err := StripUnusedMetadata(tombstone)
		if err != nil {
			t.Fatalf("StripUnusedMetadata: %v", err)
		}
		if out != tombstone {
			t.Errorf("StripUnusedMetadata(%v) = %v, want passthrough", tombstone, out)
		}
	})
}
