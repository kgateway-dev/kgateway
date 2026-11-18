//go:build e2e

package dualstack

import (
	"path/filepath"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/fsutils"
)

var (
	// manifests
	gatewayDualStackService = filepath.Join(fsutils.MustGetThisDir(), "testdata", "gateway-dual-stack-service.yaml")

	// objects
	proxyObjectMeta = metav1.ObjectMeta{
		Name:      "gw",
		Namespace: "default",
	}
)
