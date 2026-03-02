//go:build e2e

package auto_host_rewrite

import (
	"path/filepath"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/kgateway"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/fsutils"
)

var (
	autoHostRewriteManifest = filepath.Join(fsutils.MustGetThisDir(), "testdata", "auto_host_rewrite.yaml")

	/* route + traffic-policy */
	route = &gwv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "httpbin-route",
			Namespace: "kgateway-base",
		},
	}
	trafficPolicy = &kgateway.TrafficPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "auto-host-rewrite",
			Namespace: "kgateway-base",
		},
	}
)
