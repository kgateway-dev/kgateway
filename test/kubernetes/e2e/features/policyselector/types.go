package policyselector

import (
	"fmt"
	"path/filepath"

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
)
