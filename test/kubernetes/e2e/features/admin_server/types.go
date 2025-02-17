//go:build ignore

package admin_server

import (
	"path/filepath"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/fsutils"
)

var (
	upstreamManifest          = filepath.Join(fsutils.MustGetThisDir(), "testdata/upstream.yaml")
	gatewayParametersManifest = filepath.Join(fsutils.MustGetThisDir(), "testdata/gateway-parameters.yaml")

	// Upstream resource to be created
	upstreamMeta = metav1.ObjectMeta{
		Name:      "json-upstream",
		Namespace: "default",
	}

	// Upstream resource to be created
	gatewayParametersMeta = metav1.ObjectMeta{
		Name:      "gw-params",
		Namespace: "default",
	}
)
