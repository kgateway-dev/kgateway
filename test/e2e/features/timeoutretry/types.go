//go:build e2e

package timeoutretry

import (
	"path/filepath"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/fsutils"
	"github.com/kgateway-dev/kgateway/v2/test/e2e/tests/base"
)

var (
	setupManifest = filepath.Join(fsutils.MustGetThisDir(), "testdata", "setup.yaml")

	setup = base.TestCase{
		Manifests: []string{setupManifest},
	}

	gatewayObjectMeta = metav1.ObjectMeta{
		Name:      "gateway",
		Namespace: "kgateway-base",
	}
)
