//go:build e2e

package csrf

import (
	"path/filepath"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	kgatewayv1alpha1 "github.com/kgateway-dev/kgateway/v2/api/v1alpha1"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/fsutils"
)

var (
	// manifests
	commonManifest              = filepath.Join(fsutils.MustGetThisDir(), "testdata", "common.yaml")
	csrfGwTrafficPolicyManifest = filepath.Join(fsutils.MustGetThisDir(), "testdata", "csrf-gw.yaml")

	// objects
	proxyObjectMeta = metav1.ObjectMeta{
		Name:      "gw",
		Namespace: "default",
	}

	gwtrafficPolicy = &kgatewayv1alpha1.TrafficPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "csrf-gw-policy",
			Namespace: "default",
		},
	}
)
