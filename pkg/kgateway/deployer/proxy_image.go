package deployer

import (
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/kgateway-dev/kgateway/v2/pkg/deployer"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
)

// proxyImageHeldBackEventReason is the reason of the event for a proxy image the
// invalid-merge fallback holds back.
const proxyImageHeldBackEventReason = "ProxyImageHeldBack"

// proxyImage returns the image of dep's kgateway-proxy container, and whether it has one.
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
		Action: applyBootstrapOverlaysAction,
		Note:   note,
	}
}
