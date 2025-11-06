//go:build e2e

package timeoutretry

import (
	"path/filepath"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/fsutils"
	testdefaults "github.com/kgateway-dev/kgateway/v2/test/e2e/defaults"
	"github.com/kgateway-dev/kgateway/v2/test/e2e/tests/base"
)

const (
	gatewayName = "test"
)

var (
	setupManifest = filepath.Join(fsutils.MustGetThisDir(), "testdata", "setup.yaml")

	setup = base.TestCase{
		Manifests: []string{setupManifest, testdefaults.CurlPodManifest, testdefaults.HttpbinManifest},
	}

	gatewayObjectMeta = metav1.ObjectMeta{
		Name:      gatewayName,
		Namespace: "default",
	}
)
