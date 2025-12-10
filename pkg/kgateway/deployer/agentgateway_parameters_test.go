package deployer

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/agentgateway"
	"github.com/kgateway-dev/kgateway/v2/pkg/deployer"
)

func TestAgentgatewayParametersApplier_ApplyToHelmValues_NilParams(t *testing.T) {
	applier := NewAgentgatewayParametersApplier(nil)
	vals := &deployer.HelmConfig{
		Gateway: &deployer.HelmGateway{
			LogLevel: ptr.To("info"),
		},
	}

	applier.ApplyToHelmValues(vals)

	// Values should be unchanged
	assert.Equal(t, "info", *vals.Gateway.LogLevel)
}

func TestAgentgatewayParametersApplier_ApplyToHelmValues_Image(t *testing.T) {
	params := &agentgateway.AgentgatewayParameters{
		Spec: agentgateway.AgentgatewayParametersSpec{
			AgentgatewayParametersConfigs: agentgateway.AgentgatewayParametersConfigs{
				Image: &agentgateway.Image{
					Registry:   ptr.To("custom.registry.io"),
					Repository: ptr.To("custom/agentgateway"),
					Tag:        ptr.To("v1.0.0"),
				},
			},
		},
	}

	applier := NewAgentgatewayParametersApplier(params)
	vals := &deployer.HelmConfig{
		Gateway: &deployer.HelmGateway{},
	}

	applier.ApplyToHelmValues(vals)

	require.NotNil(t, vals.Gateway.Image)
	assert.Equal(t, "custom.registry.io", *vals.Gateway.Image.Registry)
	assert.Equal(t, "custom/agentgateway", *vals.Gateway.Image.Repository)
	assert.Equal(t, "v1.0.0", *vals.Gateway.Image.Tag)
}

func TestAgentgatewayParametersApplier_ApplyToHelmValues_Resources(t *testing.T) {
	params := &agentgateway.AgentgatewayParameters{
		Spec: agentgateway.AgentgatewayParametersSpec{
			AgentgatewayParametersConfigs: agentgateway.AgentgatewayParametersConfigs{
				Resources: &corev1.ResourceRequirements{
					Limits: corev1.ResourceList{
						corev1.ResourceMemory: resource.MustParse("512Mi"),
						corev1.ResourceCPU:    resource.MustParse("500m"),
					},
					Requests: corev1.ResourceList{
						corev1.ResourceMemory: resource.MustParse("256Mi"),
						corev1.ResourceCPU:    resource.MustParse("250m"),
					},
				},
			},
		},
	}

	applier := NewAgentgatewayParametersApplier(params)
	vals := &deployer.HelmConfig{
		Gateway: &deployer.HelmGateway{},
	}

	applier.ApplyToHelmValues(vals)

	require.NotNil(t, vals.Gateway.Resources)
	assert.Equal(t, "512Mi", vals.Gateway.Resources.Limits.Memory().String())
	assert.Equal(t, "500m", vals.Gateway.Resources.Limits.Cpu().String())
}

func TestAgentgatewayParametersApplier_ApplyToHelmValues_Env(t *testing.T) {
	params := &agentgateway.AgentgatewayParameters{
		Spec: agentgateway.AgentgatewayParametersSpec{
			AgentgatewayParametersConfigs: agentgateway.AgentgatewayParametersConfigs{
				Env: []corev1.EnvVar{
					{Name: "CUSTOM_VAR", Value: "custom_value"},
					{Name: "ANOTHER_VAR", Value: "another_value"},
				},
			},
		},
	}

	applier := NewAgentgatewayParametersApplier(params)
	vals := &deployer.HelmConfig{
		Gateway: &deployer.HelmGateway{
			Env: []corev1.EnvVar{
				{Name: "EXISTING_VAR", Value: "existing_value"},
			},
		},
	}

	applier.ApplyToHelmValues(vals)

	require.Len(t, vals.Gateway.Env, 3)
	assert.Equal(t, "EXISTING_VAR", vals.Gateway.Env[0].Name)
	assert.Equal(t, "CUSTOM_VAR", vals.Gateway.Env[1].Name)
	assert.Equal(t, "ANOTHER_VAR", vals.Gateway.Env[2].Name)
}

func TestAgentgatewayParametersApplier_ApplyToHelmValues_Logging(t *testing.T) {
	params := &agentgateway.AgentgatewayParameters{
		Spec: agentgateway.AgentgatewayParametersSpec{
			AgentgatewayParametersConfigs: agentgateway.AgentgatewayParametersConfigs{
				Logging: &agentgateway.AgentgatewayParametersLogging{
					Level: "debug",
				},
			},
		},
	}

	applier := NewAgentgatewayParametersApplier(params)
	vals := &deployer.HelmConfig{
		Gateway: &deployer.HelmGateway{},
	}

	applier.ApplyToHelmValues(vals)

	// Level should be set as RUST_LOG env var
	require.Len(t, vals.Gateway.Env, 1)
	assert.Equal(t, "RUST_LOG", vals.Gateway.Env[0].Name)
	assert.Equal(t, "debug", vals.Gateway.Env[0].Value)
}

func TestAgentgatewayParametersApplier_ApplyOverlaysToObjects(t *testing.T) {
	specPatch := []byte(`{
		"replicas": 3
	}`)

	params := &agentgateway.AgentgatewayParameters{
		Spec: agentgateway.AgentgatewayParametersSpec{
			AgentgatewayParametersOverlays: agentgateway.AgentgatewayParametersOverlays{
				Deployment: &agentgateway.KubernetesResourceOverlay{
					Metadata: &agentgateway.AgentgatewayParametersObjectMetadata{
						Labels: map[string]string{
							"overlay-label": "overlay-value",
						},
					},
					Spec: &apiextensionsv1.JSON{Raw: specPatch},
				},
			},
		},
	}

	applier := NewAgentgatewayParametersApplier(params)

	deployment := &appsv1.Deployment{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "apps/v1",
			Kind:       "Deployment",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-deployment",
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: ptr.To[int32](1),
		},
	}
	objs := []client.Object{deployment}

	err := applier.ApplyOverlaysToObjects(objs)
	require.NoError(t, err)

	result := objs[0].(*appsv1.Deployment)
	assert.Equal(t, int32(3), *result.Spec.Replicas)
	assert.Equal(t, "overlay-value", result.Labels["overlay-label"])
}

func TestAgentgatewayParametersApplier_ApplyOverlaysToObjects_NilParams(t *testing.T) {
	applier := NewAgentgatewayParametersApplier(nil)

	deployment := &appsv1.Deployment{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "apps/v1",
			Kind:       "Deployment",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-deployment",
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: ptr.To[int32](1),
		},
	}
	objs := []client.Object{deployment}

	err := applier.ApplyOverlaysToObjects(objs)
	require.NoError(t, err)

	result := objs[0].(*appsv1.Deployment)
	assert.Equal(t, int32(1), *result.Spec.Replicas)
}

func TestAgentgatewayParametersApplier_ApplyToHelmValues_RawConfig(t *testing.T) {
	rawConfigJSON := []byte(`{
		"tracing": {
			"otlpEndpoint": "http://jaeger:4317"
		},
		"metrics": {
			"enabled": true
		}
	}`)

	params := &agentgateway.AgentgatewayParameters{
		Spec: agentgateway.AgentgatewayParametersSpec{
			AgentgatewayParametersConfigs: agentgateway.AgentgatewayParametersConfigs{
				RawConfig: &apiextensionsv1.JSON{Raw: rawConfigJSON},
			},
		},
	}

	applier := NewAgentgatewayParametersApplier(params)
	vals := &deployer.HelmConfig{
		Gateway: &deployer.HelmGateway{},
	}

	applier.ApplyToHelmValues(vals)

	require.NotNil(t, vals.Gateway.RawConfig)
	tracing, ok := vals.Gateway.RawConfig["tracing"].(map[string]any)
	require.True(t, ok, "tracing should be a map")
	assert.Equal(t, "http://jaeger:4317", tracing["otlpEndpoint"])

	metrics, ok := vals.Gateway.RawConfig["metrics"].(map[string]any)
	require.True(t, ok, "metrics should be a map")
	assert.Equal(t, true, metrics["enabled"])
}

func TestAgentgatewayParametersApplier_ApplyToHelmValues_RawConfigWithLogging(t *testing.T) {
	// rawConfig has logging.format, but typed Logging.Format should take precedence
	// (merging happens in helm template, but here we test both are passed through)
	rawConfigJSON := []byte(`{
		"logging": {
			"format": "json"
		},
		"tracing": {
			"otlpEndpoint": "http://jaeger:4317"
		}
	}`)

	params := &agentgateway.AgentgatewayParameters{
		Spec: agentgateway.AgentgatewayParametersSpec{
			AgentgatewayParametersConfigs: agentgateway.AgentgatewayParametersConfigs{
				Logging: &agentgateway.AgentgatewayParametersLogging{
					Format: agentgateway.AgentgatewayParametersLoggingText,
				},
				RawConfig: &apiextensionsv1.JSON{Raw: rawConfigJSON},
			},
		},
	}

	applier := NewAgentgatewayParametersApplier(params)
	vals := &deployer.HelmConfig{
		Gateway: &deployer.HelmGateway{},
	}

	applier.ApplyToHelmValues(vals)

	// Both should be set - merging happens in helm template
	assert.Equal(t, "text", *vals.Gateway.LogFormat)
	require.NotNil(t, vals.Gateway.RawConfig)
	tracing, ok := vals.Gateway.RawConfig["tracing"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "http://jaeger:4317", tracing["otlpEndpoint"])
}
