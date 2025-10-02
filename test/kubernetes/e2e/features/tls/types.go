package tls

import (
	"path/filepath"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/fsutils"
)

var (
	basicGatewayManifest = filepath.Join(fsutils.MustGetThisDir(), "testdata", "gw.yaml")

	xdsTlsSecret = &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "xds-tls-secret",
			Namespace: "default",
		},
	}

	gateway = &gatewayv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "gw",
			Namespace: "default",
		},
	}

	testRoute = &gatewayv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "example-route",
			Namespace: "default",
		},
	}
)
