package policyselector

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
	labelSelectorManifest = filepath.Join(fsutils.MustGetThisDir(), "testdata", "label_selector.yaml")

	proxyHostPort = fmt.Sprintf("%s.%s.svc:%d", gateway.Name, gateway.Namespace, gatewayPort)

	gateway = &gwv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "http-gw",
			Namespace: "default",
		},
	}

	httpbinRoute = &gwv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "httpbin",
			Namespace: "default",
		},
	}

	httpbinDeployment = &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "httpbin",
			Namespace: "default",
		},
	}
)
