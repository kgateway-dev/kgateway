package strategicpatch

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/kgateway"
	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/shared"
)

func TestOverlayApplier_ApplyOverlays_NilParams(t *testing.T) {
	applier := NewOverlayApplierFromGatewayParameters(nil)
	objs := []client.Object{
		&appsv1.Deployment{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "apps/v1",
				Kind:       "Deployment",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-deployment",
			},
		},
	}

	result, err := applier.ApplyOverlays(objs)
	require.NoError(t, err)
	assert.Len(t, result, 1)
}

func TestOverlayApplier_ApplyOverlays_MetadataLabels(t *testing.T) {
	params := &kgateway.GatewayParameters{
		Spec: kgateway.GatewayParametersSpec{
			Kube: &kgateway.KubernetesProxyConfig{
				GatewayParametersOverlays: kgateway.GatewayParametersOverlays{
					DeploymentOverlay: &shared.KubernetesResourceOverlay{
						Metadata: &shared.ObjectMetadata{
							Labels: map[string]*string{
								"custom-label": new("custom-value"),
							},
						},
					},
				},
			},
		},
	}

	applier := NewOverlayApplierFromGatewayParameters(params)
	deployment := &appsv1.Deployment{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "apps/v1",
			Kind:       "Deployment",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-deployment",
			Labels: map[string]string{
				"existing-label": "existing-value",
			},
		},
	}
	objs := []client.Object{deployment}

	objs, err := applier.ApplyOverlays(objs)
	require.NoError(t, err)

	result := objs[0].(*appsv1.Deployment)
	assert.Equal(t, "custom-value", result.Labels["custom-label"])
	assert.Equal(t, "existing-value", result.Labels["existing-label"])
}

func TestOverlayApplier_ApplyOverlays_MetadataLabelDeletion(t *testing.T) {
	// Nil *string values in overlay labels delete existing keys via SMP null.
	params := &kgateway.GatewayParameters{
		Spec: kgateway.GatewayParametersSpec{
			Kube: &kgateway.KubernetesProxyConfig{
				GatewayParametersOverlays: kgateway.GatewayParametersOverlays{
					DeploymentOverlay: &shared.KubernetesResourceOverlay{
						Metadata: &shared.ObjectMetadata{
							Labels: map[string]*string{
								"label-to-delete": nil,
								"new-label":       new("new-value"),
							},
						},
					},
				},
			},
		},
	}

	applier := NewOverlayApplierFromGatewayParameters(params)
	deployment := &appsv1.Deployment{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "apps/v1",
			Kind:       "Deployment",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-deployment",
			Labels: map[string]string{
				"label-to-delete": "old-value",
				"label-to-keep":   "keep-value",
			},
		},
	}
	objs := []client.Object{deployment}

	objs, err := applier.ApplyOverlays(objs)
	require.NoError(t, err)

	result := objs[0].(*appsv1.Deployment)
	assert.NotContains(t, result.Labels, "label-to-delete", "nil overlay value should delete the label")
	assert.Equal(t, "keep-value", result.Labels["label-to-keep"], "unaffected labels should remain")
	assert.Equal(t, "new-value", result.Labels["new-label"], "new labels should be added")
}

func TestOverlayApplier_ApplyOverlays_NilDeleteNonExistentKey(t *testing.T) {
	// Nil *string for a non-existent key is a no-op (deletion of nothing).
	params := &kgateway.GatewayParameters{
		Spec: kgateway.GatewayParametersSpec{
			Kube: &kgateway.KubernetesProxyConfig{
				GatewayParametersOverlays: kgateway.GatewayParametersOverlays{
					DeploymentOverlay: &shared.KubernetesResourceOverlay{
						Metadata: &shared.ObjectMetadata{
							Labels: map[string]*string{
								"does-not-exist": nil,
							},
							Annotations: map[string]*string{
								"does-not-exist": nil,
							},
						},
					},
				},
			},
		},
	}

	applier := NewOverlayApplierFromGatewayParameters(params)
	deployment := &appsv1.Deployment{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "apps/v1",
			Kind:       "Deployment",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-deployment",
			Labels: map[string]string{
				"existing": "value",
			},
			Annotations: map[string]string{
				"existing": "value",
			},
		},
	}
	objs := []client.Object{deployment}

	objs, err := applier.ApplyOverlays(objs)
	require.NoError(t, err)

	result := objs[0].(*appsv1.Deployment)
	assert.NotContains(t, result.Labels, "does-not-exist",
		"nil for a non-existent key should not create the label")
	assert.Equal(t, "value", result.Labels["existing"],
		"existing labels should be unaffected")
	assert.NotContains(t, result.Annotations, "does-not-exist",
		"nil for a non-existent key should not create the annotation")
	assert.Equal(t, "value", result.Annotations["existing"],
		"existing annotations should be unaffected")
}

func TestOverlayApplier_ApplyOverlays_EmptyStringNewKey(t *testing.T) {
	// ptr.To("") for a non-existent key creates it with an empty string value.
	params := &kgateway.GatewayParameters{
		Spec: kgateway.GatewayParametersSpec{
			Kube: &kgateway.KubernetesProxyConfig{
				GatewayParametersOverlays: kgateway.GatewayParametersOverlays{
					DeploymentOverlay: &shared.KubernetesResourceOverlay{
						Metadata: &shared.ObjectMetadata{
							Labels: map[string]*string{
								"new-empty": new(""),
							},
							Annotations: map[string]*string{
								"new-empty": new(""),
							},
						},
					},
				},
			},
		},
	}

	applier := NewOverlayApplierFromGatewayParameters(params)
	deployment := &appsv1.Deployment{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "apps/v1",
			Kind:       "Deployment",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-deployment",
			Labels: map[string]string{
				"existing": "value",
			},
			Annotations: map[string]string{
				"existing": "value",
			},
		},
	}
	objs := []client.Object{deployment}

	objs, err := applier.ApplyOverlays(objs)
	require.NoError(t, err)

	result := objs[0].(*appsv1.Deployment)
	assert.Contains(t, result.Labels, "new-empty",
		"ptr.To empty string should create the label")
	assert.Equal(t, "", result.Labels["new-empty"],
		"label value should be empty string")
	assert.Equal(t, "value", result.Labels["existing"],
		"existing labels should be unaffected")
	assert.Contains(t, result.Annotations, "new-empty",
		"ptr.To empty string should create the annotation")
	assert.Equal(t, "", result.Annotations["new-empty"],
		"annotation value should be empty string")
	assert.Equal(t, "value", result.Annotations["existing"],
		"existing annotations should be unaffected")
}

func TestOverlayApplier_ApplyOverlays_EmptyStringExistingKey(t *testing.T) {
	// ptr.To("") for an existing key sets it to empty string (does not delete).
	params := &kgateway.GatewayParameters{
		Spec: kgateway.GatewayParametersSpec{
			Kube: &kgateway.KubernetesProxyConfig{
				GatewayParametersOverlays: kgateway.GatewayParametersOverlays{
					DeploymentOverlay: &shared.KubernetesResourceOverlay{
						Metadata: &shared.ObjectMetadata{
							Labels: map[string]*string{
								"to-empty": new(""),
							},
							Annotations: map[string]*string{
								"to-empty": new(""),
							},
						},
					},
				},
			},
		},
	}

	applier := NewOverlayApplierFromGatewayParameters(params)
	deployment := &appsv1.Deployment{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "apps/v1",
			Kind:       "Deployment",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-deployment",
			Labels: map[string]string{
				"to-empty": "old-value",
				"to-keep":  "keep-value",
			},
			Annotations: map[string]string{
				"to-empty": "old-value",
				"to-keep":  "keep-value",
			},
		},
	}
	objs := []client.Object{deployment}

	objs, err := applier.ApplyOverlays(objs)
	require.NoError(t, err)

	result := objs[0].(*appsv1.Deployment)
	assert.Contains(t, result.Labels, "to-empty",
		"ptr.To empty string should keep the label")
	assert.Equal(t, "", result.Labels["to-empty"],
		"label should be set to empty string")
	assert.Equal(t, "keep-value", result.Labels["to-keep"],
		"other labels should be unaffected")
	assert.Contains(t, result.Annotations, "to-empty",
		"ptr.To empty string should keep the annotation")
	assert.Equal(t, "", result.Annotations["to-empty"],
		"annotation should be set to empty string")
	assert.Equal(t, "keep-value", result.Annotations["to-keep"],
		"other annotations should be unaffected")
}

func TestOverlayApplier_ApplyOverlays_MetadataAnnotations(t *testing.T) {
	params := &kgateway.GatewayParameters{
		Spec: kgateway.GatewayParametersSpec{
			Kube: &kgateway.KubernetesProxyConfig{
				GatewayParametersOverlays: kgateway.GatewayParametersOverlays{
					ServiceOverlay: &shared.KubernetesResourceOverlay{
						Metadata: &shared.ObjectMetadata{
							Annotations: map[string]*string{
								"custom-annotation": new("custom-value"),
							},
						},
					},
				},
			},
		},
	}

	applier := NewOverlayApplierFromGatewayParameters(params)
	svc := &corev1.Service{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Service",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-service",
		},
	}
	objs := []client.Object{svc}

	objs, err := applier.ApplyOverlays(objs)
	require.NoError(t, err)

	result := objs[0].(*corev1.Service)
	assert.Equal(t, "custom-value", result.Annotations["custom-annotation"])
}

func TestOverlayApplier_ApplyOverlays_DeploymentSpec(t *testing.T) {
	// Test strategic merge patch for deployment spec
	specPatch := []byte(`{
		"replicas": 3,
		"template": {
			"spec": {
				"containers": [{
					"name": "kgateway-proxy",
					"resources": {
						"limits": {
							"memory": "512Mi"
						}
					}
				}]
			}
		}
	}`)

	params := &kgateway.GatewayParameters{
		Spec: kgateway.GatewayParametersSpec{
			Kube: &kgateway.KubernetesProxyConfig{
				GatewayParametersOverlays: kgateway.GatewayParametersOverlays{
					DeploymentOverlay: &shared.KubernetesResourceOverlay{
						Spec: &apiextensionsv1.JSON{Raw: specPatch},
					},
				},
			},
		},
	}

	applier := NewOverlayApplierFromGatewayParameters(params)
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
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "kgateway-proxy",
							Image: "foo/envoy-wrapper:latest",
						},
					},
				},
			},
		},
	}
	objs := []client.Object{deployment}

	objs, err := applier.ApplyOverlays(objs)
	require.NoError(t, err)

	result := objs[0].(*appsv1.Deployment)
	assert.Equal(t, int32(3), *result.Spec.Replicas)
	assert.Equal(t, "foo/envoy-wrapper:latest", result.Spec.Template.Spec.Containers[0].Image)
	assert.NotNil(t, result.Spec.Template.Spec.Containers[0].Resources.Limits)
	assert.Equal(t, "512Mi", result.Spec.Template.Spec.Containers[0].Resources.Limits.Memory().String())
}

func TestOverlayApplier_ApplyOverlays_DeleteContainerWithPatchDirective(t *testing.T) {
	// Test strategic merge patch with $patch: delete directive
	specPatch := []byte(`{
		"template": {
			"spec": {
				"containers": [{
					"name": "sidecar",
					"$patch": "delete"
				}]
			}
		}
	}`)

	params := &kgateway.GatewayParameters{
		Spec: kgateway.GatewayParametersSpec{
			Kube: &kgateway.KubernetesProxyConfig{
				GatewayParametersOverlays: kgateway.GatewayParametersOverlays{
					DeploymentOverlay: &shared.KubernetesResourceOverlay{
						Spec: &apiextensionsv1.JSON{Raw: specPatch},
					},
				},
			},
		},
	}

	applier := NewOverlayApplierFromGatewayParameters(params)
	deployment := &appsv1.Deployment{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "apps/v1",
			Kind:       "Deployment",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-deployment",
		},
		Spec: appsv1.DeploymentSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "kgateway-proxy",
							Image: "foo/envoy-wrapper:latest",
						},
						{
							Name:  "sidecar",
							Image: "sidecar:latest",
						},
					},
				},
			},
		},
	}
	objs := []client.Object{deployment}

	objs, err := applier.ApplyOverlays(objs)
	require.NoError(t, err)

	result := objs[0].(*appsv1.Deployment)
	require.Len(t, result.Spec.Template.Spec.Containers, 1)
	assert.Equal(t, "kgateway-proxy", result.Spec.Template.Spec.Containers[0].Name)
}

func TestOverlayApplier_ApplyOverlays_ServiceSpec(t *testing.T) {
	specPatch := []byte(`{
		"type": "NodePort"
	}`)

	params := &kgateway.GatewayParameters{
		Spec: kgateway.GatewayParametersSpec{
			Kube: &kgateway.KubernetesProxyConfig{
				GatewayParametersOverlays: kgateway.GatewayParametersOverlays{
					ServiceOverlay: &shared.KubernetesResourceOverlay{
						Spec: &apiextensionsv1.JSON{Raw: specPatch},
					},
				},
			},
		},
	}

	applier := NewOverlayApplierFromGatewayParameters(params)
	svc := &corev1.Service{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Service",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-service",
		},
		Spec: corev1.ServiceSpec{
			Type: corev1.ServiceTypeLoadBalancer,
		},
	}
	objs := []client.Object{svc}

	objs, err := applier.ApplyOverlays(objs)
	require.NoError(t, err)

	result := objs[0].(*corev1.Service)
	assert.Equal(t, corev1.ServiceTypeNodePort, result.Spec.Type)
}

func TestOverlayApplier_ApplyOverlays_MultipleObjects(t *testing.T) {
	params := &kgateway.GatewayParameters{
		Spec: kgateway.GatewayParametersSpec{
			Kube: &kgateway.KubernetesProxyConfig{
				GatewayParametersOverlays: kgateway.GatewayParametersOverlays{
					DeploymentOverlay: &shared.KubernetesResourceOverlay{
						Metadata: &shared.ObjectMetadata{
							Labels: map[string]*string{"app": new("modified")},
						},
					},
					ServiceOverlay: &shared.KubernetesResourceOverlay{
						Metadata: &shared.ObjectMetadata{
							Labels: map[string]*string{"svc": new("modified")},
						},
					},
					ServiceAccountOverlay: &shared.KubernetesResourceOverlay{
						Metadata: &shared.ObjectMetadata{
							Labels: map[string]*string{"sa": new("modified")},
						},
					},
				},
			},
		},
	}

	applier := NewOverlayApplierFromGatewayParameters(params)
	objs := []client.Object{
		&appsv1.Deployment{
			TypeMeta:   metav1.TypeMeta{APIVersion: "apps/v1", Kind: "Deployment"},
			ObjectMeta: metav1.ObjectMeta{Name: "test-deployment"},
		},
		&corev1.Service{
			TypeMeta:   metav1.TypeMeta{APIVersion: "v1", Kind: "Service"},
			ObjectMeta: metav1.ObjectMeta{Name: "test-service"},
		},
		&corev1.ServiceAccount{
			TypeMeta:   metav1.TypeMeta{APIVersion: "v1", Kind: "ServiceAccount"},
			ObjectMeta: metav1.ObjectMeta{Name: "test-sa"},
		},
		&corev1.ConfigMap{
			TypeMeta:   metav1.TypeMeta{APIVersion: "v1", Kind: "ConfigMap"},
			ObjectMeta: metav1.ObjectMeta{Name: "test-cm"},
		},
	}

	objs, err := applier.ApplyOverlays(objs)
	require.NoError(t, err)

	// Check deployment
	deploy := objs[0].(*appsv1.Deployment)
	assert.Equal(t, "modified", deploy.Labels["app"])

	// Check service
	svc := objs[1].(*corev1.Service)
	assert.Equal(t, "modified", svc.Labels["svc"])

	// Check service account
	sa := objs[2].(*corev1.ServiceAccount)
	assert.Equal(t, "modified", sa.Labels["sa"])

	// Check configmap (should be unchanged, no overlay for it)
	cm := objs[3].(*corev1.ConfigMap)
	assert.Empty(t, cm.Labels)
}
