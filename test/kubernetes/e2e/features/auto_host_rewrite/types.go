package auto_host_rewrite

import (
	"path/filepath"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	v1alpha1 "github.com/kgateway-dev/kgateway/v2/api/v1alpha1"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/fsutils"
)

var (
	autoHostRewriteManifest = filepath.Join(fsutils.MustGetThisDir(), "testdata", "auto_host_rewrite.yaml")

	/* objects from gateway manifest (gw.yaml) */
	proxyObjectMeta = metav1.ObjectMeta{
		Name:      "gw",
		Namespace: "default",
	}
	proxyDeployment = &appsv1.Deployment{ObjectMeta: proxyObjectMeta}
	proxyService    = &corev1.Service{ObjectMeta: proxyObjectMeta}

	/* route + traffic-policy */
	route = &gwv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "httpbin-route",
			Namespace: "default",
		},
	}
	trafficPolicy = &v1alpha1.TrafficPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "auto-host-rewrite",
			Namespace: "default",
		},
	}
)
