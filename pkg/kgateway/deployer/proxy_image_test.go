package deployer

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/kgateway"
	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/shared"
)

func TestParseImageRef(t *testing.T) {
	cases := map[string]imageRef{
		"envoy-wrapper": {repository: "envoy-wrapper"},
		"cr.kgateway.dev/kgateway-dev/envoy-wrapper:v2.1.0":  {repository: "envoy-wrapper", tag: "v2.1.0"},
		"localhost:5000/envoy-wrapper:v2.1.0":                {repository: "envoy-wrapper", tag: "v2.1.0"},
		"localhost:5000/envoy-wrapper":                       {repository: "envoy-wrapper"},
		"ghcr.io/envoy-wrapper@sha256:abc":                   {repository: "envoy-wrapper", digest: "sha256:abc"},
		"ghcr.io/envoy-wrapper:v2.1.0@sha256:abc":            {repository: "envoy-wrapper", tag: "v2.1.0", digest: "sha256:abc"},
		"registry.example.com:443/team/custom-envoy:v1.36.0": {repository: "custom-envoy", tag: "v1.36.0"},
	}
	for image, want := range cases {
		assert.Equal(t, want, parseImageRef(image), image)
	}
}

func TestBootstrapPatchesProxyImage(t *testing.T) {
	const overlay = `{stats_flush_interval: 10s}`
	const differs = "The proxy runs image %s, not envoy-wrapper at the controller's image tag bar; the bootstrap patch and routes were validated with the Envoy in the controller's image, which may accept or reject different configuration than the proxy's Envoy."

	// withImage returns patched GatewayParameters that set the proxy image.
	withImage := func(gwp *kgateway.GatewayParameters, image kgateway.Image) *kgateway.GatewayParameters {
		gwp = withOverlay(t, gwp, overlay)
		gwp.Spec.Kube.EnvoyContainer.Image = &image
		return gwp
	}

	// message renders the Gateway and returns its condition message.
	message := func(t *testing.T, tc bootstrapTestCase) string {
		t.Helper()
		if tc.controllerImageTag == "" {
			tc.controllerImageTag = "bar"
		}
		_, report, err := tc.render(t)
		require.NoError(t, err)
		return bootstrapCondition(t, report).Message
	}

	t.Run("matching image", func(t *testing.T) {
		// The default proxy image, foo/envoy-wrapper:bar, matches the controller's tag.
		assert.Equal(t, "The bootstrap patch is applied.", message(t, bootstrapTestCase{gatewayParams: withOverlay(t, gatewayParams(), overlay)}))
	})

	t.Run("only the registry differs", func(t *testing.T) {
		// A mirror serves the same repository and tag, so it is not reported.
		gwp := withImage(gatewayParams(), kgateway.Image{Registry: new("mirror.example.com/kgateway")})
		assert.Equal(t, "The bootstrap patch is applied.", message(t, bootstrapTestCase{gatewayParams: gwp}))
	})

	t.Run("repository, tag or digest differs", func(t *testing.T) {
		// Each is reported with the proxy image; the status and reason do not change.
		for image, want := range map[*kgateway.Image]string{
			{Repository: new("custom-envoy")}:   "foo/custom-envoy:bar",
			{Tag: new("other")}:                 "foo/envoy-wrapper:other",
			{Digest: new("sha256:abc")}:         "foo/envoy-wrapper@sha256:abc",
			{Tag: new("bar"), Digest: new("x")}: "foo/envoy-wrapper:bar@x",
		} {
			_, report, err := bootstrapTestCase{gatewayParams: withImage(gatewayParams(), *image), controllerImageTag: "bar"}.render(t)
			require.NoError(t, err)
			cond := bootstrapCondition(t, report)
			assert.Equal(t, "The bootstrap patch is applied. "+fmt.Sprintf(differs, want), cond.Message)
			assert.Equal(t, "Applied", cond.Reason)
		}
	})

	t.Run("image set on the GatewayClass", func(t *testing.T) {
		// The GatewayClass's GatewayParameters set the image; the Gateway's set the patch.
		classParams := emptyGatewayParameters()
		classParams.Spec.Kube = &kgateway.KubernetesProxyConfig{EnvoyContainer: &kgateway.EnvoyContainer{Image: &kgateway.Image{Tag: new("other")}}}
		got := message(t, bootstrapTestCase{classParams: classParams, gatewayParams: withOverlay(t, gatewayParams(), overlay)})
		assert.Equal(t, "The bootstrap patch is applied. "+fmt.Sprintf(differs, "foo/envoy-wrapper:other"), got)
	})

	t.Run("controller tag differs from the default proxy tag", func(t *testing.T) {
		// Every patched Gateway is reported.
		got := message(t, bootstrapTestCase{gatewayParams: withOverlay(t, gatewayParams(), overlay), controllerImageTag: "controller-tag"})
		assert.Contains(t, got, "The proxy runs image foo/envoy-wrapper:bar, not envoy-wrapper at the controller's image tag controller-tag")
	})

	t.Run("deploymentOverlay sets the image", func(t *testing.T) {
		// The object overlay's image is the one compared.
		gwp := withOverlay(t, gatewayParams(), overlay)
		gwp.Spec.Kube.DeploymentOverlay = &shared.KubernetesResourceOverlay{Spec: &apiextensionsv1.JSON{Raw: []byte(
			`{"template":{"spec":{"containers":[{"name":"kgateway-proxy","image":"example.com/custom-envoy:v1"}]}}}`)}}
		got := message(t, bootstrapTestCase{gatewayParams: gwp})
		assert.Equal(t, "The bootstrap patch is applied. "+fmt.Sprintf(differs, "example.com/custom-envoy:v1"), got)
	})

	t.Run("deploymentOverlay removes the proxy container", func(t *testing.T) {
		// The condition says the image could not be checked.
		gwp := withOverlay(t, gatewayParams(), overlay)
		gwp.Spec.Kube.DeploymentOverlay = &shared.KubernetesResourceOverlay{Spec: &apiextensionsv1.JSON{Raw: []byte(
			`{"template":{"spec":{"containers":[{"name":"kgateway-proxy","$patch":"delete"}]}}}`)}}
		got := message(t, bootstrapTestCase{gatewayParams: gwp})
		assert.Contains(t, got, "The proxy image could not be checked against the controller's: the Deployment has no kgateway-proxy container.")
	})

	t.Run("without the controller tag", func(t *testing.T) {
		// Nothing is compared.
		gwp := withImage(gatewayParams(), kgateway.Image{Tag: new("other")})
		_, report, err := bootstrapTestCase{gatewayParams: gwp}.render(t)
		require.NoError(t, err)
		assert.Equal(t, "The bootstrap patch is applied.", bootstrapCondition(t, report).Message)
	})
}

func TestProxyImageNoteWithoutContainer(t *testing.T) {
	// A Deployment without the proxy container cannot be checked.
	dep := &appsv1.Deployment{Spec: appsv1.DeploymentSpec{Template: corev1.PodTemplateSpec{Spec: corev1.PodSpec{
		Containers: []corev1.Container{{Name: "other", Image: "other:v1"}},
	}}}}
	assert.Equal(t, "The proxy image could not be checked against the controller's: the Deployment has no kgateway-proxy container.",
		proxyImageNote([]client.Object{dep}, "bar"))
}

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

func TestBootstrapPatchErrorNamesRenderedProxy(t *testing.T) {
	// An invalid patch and a new proxy image in the same GatewayParameters, with a
	// deploymentOverlay that sets the image last.
	gwp := withOverlay(t, gatewayParams(), `{layerd_runtime: {}}`)
	gwp.Spec.Kube.EnvoyContainer.Image = &kgateway.Image{Tag: new("new-tag")}
	gwp.Spec.Kube.DeploymentOverlay = &shared.KubernetesResourceOverlay{Spec: &apiextensionsv1.JSON{Raw: []byte(
		`{"template":{"spec":{"containers":[{"name":"kgateway-proxy","image":"example.com/envoy-wrapper:overlay"}]}}}`)}}
	_, _, err := bootstrapTestCase{gatewayParams: gwp}.render(t)

	// The error names the Deployment and the image the object overlays give it, which
	// the reconciler compares with the live Deployment.
	var patchErr *BootstrapPatchError
	require.ErrorAs(t, err, &patchErr)
	assert.Equal(t, RenderedProxy{DeploymentName: "gw", Image: "example.com/envoy-wrapper:overlay"}, patchErr.Proxy)
}
