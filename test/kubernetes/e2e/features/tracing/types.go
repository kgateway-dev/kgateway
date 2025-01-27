package tracing

import (
	"path/filepath"

	"github.com/solo-io/skv2/codegen/util"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	pathWithRouteDescriptor    = "/path/with/route/descriptor"
	pathWithoutRouteDescriptor = "/path/without/route/descriptor"
	routeDescriptorSpanName    = "THISISAROUTEDESCRIPTOR"
	gatewayProxyHost           = "gateway-proxy-tracing"
	gatewayProxyPort           = 18080
)

var (
	setupOtelcolManifest        = filepath.Join(util.MustGetThisDir(), "testdata", "setup-otelcol.yaml")
	tracingConfigManifest       = filepath.Join(util.MustGetThisDir(), "testdata", "tracing.yaml")
	gatewayConfigManifest       = filepath.Join(util.MustGetThisDir(), "testdata", "gateway.yaml")
	gatewayProxyServiceManifest = filepath.Join(util.MustGetThisDir(), "testdata", "gw-proxy-tracing-service.yaml")

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
