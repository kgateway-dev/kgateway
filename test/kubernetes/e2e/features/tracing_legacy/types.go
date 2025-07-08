//go:build ignore

package tracing

import (
	"path/filepath"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/fsutils"
)

const (
	pathWithRouteDescriptor    = "/path/with/route/descriptor"
	pathWithoutRouteDescriptor = "/path/without/route/descriptor"
	routeDescriptorSpanName    = "THISISAROUTEDESCRIPTOR"
	gatewayProxyHost           = "gateway-proxy-tracing"
	gatewayProxyPort           = 18080
)

var (
	setupOtelcolManifest        = filepath.Join(fsutils.MustGetThisDir(), "testdata", "setup-otelcol.yaml")
	tracingConfigManifest       = filepath.Join(fsutils.MustGetThisDir(), "testdata", "tracing.yaml")
	gatewayConfigManifest       = filepath.Join(fsutils.MustGetThisDir(), "testdata", "gateway.yaml")
	gatewayProxyServiceManifest = filepath.Join(fsutils.MustGetThisDir(), "testdata", "gw-proxy-tracing-service.yaml")

	otelcolPod = &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "otel-collector", Namespace: "default"},
	}
	otelcolSelector = metav1.ListOptions{
		LabelSelector: "app.kubernetes.io/name=otel-collector",
	}
	otelcolUpstream = &metav1.ObjectMeta{
		Name:      "opentelemetry-collector",
		Namespace: "default",
	}
	tracingVs = &metav1.ObjectMeta{
		Name:      "virtual-service",
		Namespace: "default",
	}
)
