//go:build e2e

package requestmirror

import (
	"path/filepath"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/fsutils"
	"github.com/kgateway-dev/kgateway/v2/test/e2e/tests/base"
)

var (
	setupManifest = filepath.Join(fsutils.MustGetThisDir(), "testdata", "setup.yaml")

	// The mirror-sink Gateway: mirrored traffic lands here and its proxy pod logs the
	// received :authority.
	mirrorSinkObjectMeta = metav1.ObjectMeta{
		Name:      "mirror-sink",
		Namespace: "kgateway-base",
	}

	setup = base.TestCase{
		Manifests: []string{setupManifest},
	}
)
