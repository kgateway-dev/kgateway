//go:build e2e

package loadtesting

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
)

func TestRestoreControllerEnvVars(t *testing.T) {
	originalValidation := &corev1.EnvVar{Name: "KGW_VALIDATION_MODE", Value: "standard"}
	originalValidator := &corev1.EnvVar{
		Name: "KGW_VALIDATOR_MODE",
		ValueFrom: &corev1.EnvVarSource{
			ConfigMapKeyRef: &corev1.ConfigMapKeySelector{
				LocalObjectReference: corev1.LocalObjectReference{Name: "controller-env"},
				Key:                  "validator-mode",
			},
		},
	}
	container := &corev1.Container{Env: []corev1.EnvVar{
		{Name: "UNCHANGED", Value: "true"},
		{Name: "KGW_VALIDATION_MODE", Value: "STRICT"},
		{Name: "KGW_XDS_FIRST_CONNECT_DELAY", Value: "0"},
		{Name: "KGW_VALIDATOR_MODE", Value: "BINARY"},
	}}

	restoreControllerEnvVars(container, map[string]*corev1.EnvVar{
		"KGW_VALIDATION_MODE":         originalValidation,
		"KGW_XDS_FIRST_CONNECT_DELAY": nil,
		"KGW_VALIDATOR_MODE":          originalValidator,
	})

	assert.Equal(t, []corev1.EnvVar{
		{Name: "UNCHANGED", Value: "true"},
		*originalValidation,
		*originalValidator,
	}, container.Env)
	assert.Equal(t, "controller-env", container.Env[2].ValueFrom.ConfigMapKeyRef.Name)
}
