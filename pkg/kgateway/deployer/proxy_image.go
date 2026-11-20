package deployer

import (
	"fmt"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/kgateway-dev/kgateway/v2/pkg/deployer"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
)

// proxyImageHeldBackEventReason is the reason of the event for a proxy image the
// invalid-merge fallback holds back.
const proxyImageHeldBackEventReason = "ProxyImageHeldBack"

// proxyImageNote returns the sentence the kgateway.dev/BootstrapPatchApplied message
// adds about the proxy image in objs, the final objects after the object overlays.
// The controller validates bootstrap patches with the Envoy in its own image, so a
// proxy image that is not envoy-wrapper at controllerTag, or is pinned by digest, may
// run an Envoy that accepts or rejects different configuration. The registry is not
// compared, so a mirror is not reported. It returns "" when the image matches, or when
// controllerTag is empty.
func proxyImageNote(objs []client.Object, controllerTag string) string {
	if controllerTag == "" {
		return ""
	}

	// Find the proxy image.
	image, ok := proxyImage(findDeployment(objs))
	if !ok {
		return fmt.Sprintf("The proxy image could not be checked against the controller's: the Deployment has no %s container.", wellknown.KgatewayContainerName)
	}

	// Compare it with envoy-wrapper at the controller's tag.
	ref := parseImageRef(image)
	if ref.repository == wellknown.EnvoyWrapperImage && ref.tag == controllerTag && ref.digest == "" {
		return ""
	}
	return fmt.Sprintf("The proxy runs image %s, not %s at the controller's image tag %s; the bootstrap patch and routes were validated with the Envoy in the controller's image, which may accept or reject different configuration than the proxy's Envoy.",
		image, wellknown.EnvoyWrapperImage, controllerTag)
}

// proxyImage returns the image of dep's kgateway-proxy container, and whether it has
// one.
func proxyImage(dep *appsv1.Deployment) (string, bool) {
	if dep == nil {
		return "", false
	}
	for _, c := range dep.Spec.Template.Spec.Containers {
		if c.Name == wellknown.KgatewayContainerName {
			return c.Image, true
		}
	}
	return "", false
}

// imageRef is a container image reference split into the parts the proxy image check
// compares.
type imageRef struct {
	// repository is the last path segment of the image name, without the registry or
	// the path before it, such as envoy-wrapper.
	repository string
	// tag is the tag, or "" without one.
	tag string
	// digest is the digest, or "" without one.
	digest string
}

// parseImageRef splits image, in the form [registry/][path/]name[:tag][@digest].
func parseImageRef(image string) imageRef {
	// Split off the digest.
	var ref imageRef
	name, digest, ok := strings.Cut(image, "@")
	if ok {
		ref.digest = digest
	}

	// Split off the tag, which follows the last colon after the last slash; a colon
	// before it belongs to a registry port.
	last := name[strings.LastIndex(name, "/")+1:]
	ref.repository, ref.tag, _ = strings.Cut(last, ":")
	return ref
}

// RenderedProxy names the proxy Deployment the deployer rendered for a Gateway and the
// image of its kgateway-proxy container, after the object overlays when they could be
// applied. Both are empty when they were not rendered.
type RenderedProxy struct {
	DeploymentName string
	Image          string
}

// renderedProxy returns the proxy Deployment and image in objs.
func renderedProxy(objs []client.Object) RenderedProxy {
	dep := findDeployment(objs)
	if dep == nil {
		return RenderedProxy{}
	}
	image, _ := proxyImage(dep)
	return RenderedProxy{DeploymentName: dep.Name, Image: image}
}

// HeldBackProxyImage returns the sentence that reports a proxy image the
// invalid-merge fallback holds back: live is the Gateway's Deployment as the cluster
// has it, and rendered the proxy the deployer did not deploy. It returns "" when
// either image is unknown or they are the same.
func HeldBackProxyImage(live *appsv1.Deployment, rendered RenderedProxy) string {
	liveImage, ok := proxyImage(live)
	if !ok || liveImage == "" || rendered.Image == "" || liveImage == rendered.Image {
		return ""
	}
	return fmt.Sprintf("The proxy keeps running image %s; image %s applies once the Gateway's objects are deployed.", liveImage, rendered.Image)
}

// HeldBackProxyImageEvent returns the Warning event that reports a proxy image the
// invalid-merge fallback holds back, with note from HeldBackProxyImage.
func HeldBackProxyImageEvent(note string) deployer.ReportEvent {
	return deployer.ReportEvent{
		Type:   corev1.EventTypeWarning,
		Reason: proxyImageHeldBackEventReason,
		Action: applyBootstrapPatchesAction,
		Note:   note,
	}
}
