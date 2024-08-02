package admin_server

import (
	"path/filepath"

	"github.com/solo-io/skv2/codegen/util"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	upstreamManifest          = filepath.Join(util.MustGetThisDir(), "testdata/upstream.yaml")
	gatewayParametersManifest = filepath.Join(util.MustGetThisDir(), "testdata/gateway-parameters.yaml")

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
