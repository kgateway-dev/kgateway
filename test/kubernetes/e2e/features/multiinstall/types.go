package multiinstall

import (
	"fmt"
	"path/filepath"

	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/fsutils"
)

const (
	gatewayPort = 8080
)

var (
	BasicManifest = filepath.Join(fsutils.MustGetThisDir(), "testdata", "basic.yaml")

	ProxyHostPort = func(ns string) string {
		return fmt.Sprintf("%s.%s.svc:%d", Gateway(ns).Name, ns, gatewayPort)
	}

	Gateway = func(ns string) *gwv1.Gateway {
		return &gwv1.Gateway{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "http-gw",
				Namespace: ns,
			},
		}
	}

	HttpbinRoute = func(ns string) *gwv1.HTTPRoute {
		return &gwv1.HTTPRoute{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "httpbin",
				Namespace: ns,
			},
		}
	}

	HttpbinDeployment = func(ns string) *appsv1.Deployment {
		return &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "httpbin",
				Namespace: ns,
			},
		}
	}
)
