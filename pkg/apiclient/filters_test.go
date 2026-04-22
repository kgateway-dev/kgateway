package apiclient

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
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
