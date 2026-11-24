package deployer

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/kgateway"
	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/shared"
)

func TestHeldBackProxyImage(t *testing.T) {
	// live returns a Deployment whose proxy runs image.
	live := func(image string) *appsv1.Deployment {
		return &appsv1.Deployment{Spec: appsv1.DeploymentSpec{Template: corev1.PodTemplateSpec{Spec: corev1.PodSpec{
			Containers: []corev1.Container{{Name: "kgateway-proxy", Image: image}},
		}}}}
	}

	// A different image is named with the running one.
	assert.Equal(t, "The proxy keeps running image a/envoy-wrapper:v1; image a/envoy-wrapper:v2 applies once the Gateway's objects are deployed.",
		HeldBackProxyImage(live("a/envoy-wrapper:v1"), RenderedProxy{DeploymentName: "gw", Image: "a/envoy-wrapper:v2"}))

	// The same image, no live Deployment, or no rendered image names nothing.
	assert.Empty(t, HeldBackProxyImage(live("a/envoy-wrapper:v1"), RenderedProxy{DeploymentName: "gw", Image: "a/envoy-wrapper:v1"}))
	assert.Empty(t, HeldBackProxyImage(nil, RenderedProxy{DeploymentName: "gw", Image: "a/envoy-wrapper:v2"}))
	assert.Empty(t, HeldBackProxyImage(live("a/envoy-wrapper:v1"), RenderedProxy{}))
}

func TestBootstrapOverlayErrorNamesRenderedProxy(t *testing.T) {
	// An invalid overlay and a new proxy image in the same GatewayParameters, with a
	// deploymentOverlay that sets the image last.
	gwp := withOverlay(t, gatewayParams(), `{layerd_runtime: {}}`)
	gwp.Spec.Kube.EnvoyContainer.Image = &kgateway.Image{Tag: new("new-tag")}
	gwp.Spec.Kube.DeploymentOverlay = &shared.KubernetesResourceOverlay{Spec: &apiextensionsv1.JSON{Raw: []byte(
		`{"template":{"spec":{"containers":[{"name":"kgateway-proxy","image":"example.com/envoy-wrapper:overlay"}]}}}`)}}
	_, _, err := bootstrapTestCase{gatewayParams: gwp}.render(t)

	// The error names the Deployment and the image the object overlays give it, which
	// the reconciler compares with the live Deployment.
	var overlayErr *BootstrapOverlayError
	require.ErrorAs(t, err, &overlayErr)
	assert.Equal(t, RenderedProxy{DeploymentName: "gw", Image: "example.com/envoy-wrapper:overlay"}, overlayErr.Proxy)
}
