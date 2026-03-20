//go:build e2e

package listenerset

import (
	"net/http"
	"path/filepath"

	"github.com/onsi/gomega/gstruct"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/fsutils"
	e2edefaults "github.com/kgateway-dev/kgateway/v2/test/e2e/defaults"
	"github.com/kgateway-dev/kgateway/v2/test/e2e/tests/base"
	testmatchers "github.com/kgateway-dev/kgateway/v2/test/gomega/matchers"
)

var (
	// manifests
	setupManifest             = filepath.Join(fsutils.MustGetThisDir(), "testdata", "setup.yaml")
	validListenerSetManifest  = filepath.Join(fsutils.MustGetThisDir(), "testdata", "valid-listenerset.yaml")
	validListenerSetManifest2 = filepath.Join(fsutils.MustGetThisDir(), "testdata", "valid-listenerset-2.yaml")
	policyManifest            = filepath.Join(fsutils.MustGetThisDir(), "testdata", "policies.yaml")

	gwListener1Port  = 80
	gwListener2Port  = 8081
	ls1Listener1Port = 90
	ls1Listener2Port = 8091
	ls2Listener1Port = 8095

	proxyObjectMeta = metav1.ObjectMeta{
		Name:      "gw",
		Namespace: "default",
	}
	proxyService = &corev1.Service{ObjectMeta: proxyObjectMeta}

	expectOKWithCustomHeader = func(key, value string) *testmatchers.HttpResponse {
		return &testmatchers.HttpResponse{
			StatusCode: http.StatusOK,
			Body:       gstruct.Ignore(),
			Headers: map[string]any{
				key: value,
			},
		}
	}

	setup = base.TestCase{
		Manifests: []string{e2edefaults.CurlPodManifest, setupManifest},
	}

	// test cases
	testCases = map[string]*base.TestCase{
		"TestPolicies": {
			ManifestsWithTransform: map[string]func(string) string{
				validListenerSetManifest:  base.TransformListenerSetManifest,
				validListenerSetManifest2: base.TransformListenerSetManifest,
				policyManifest:            base.TransformListenerSetManifest,
			},
		},
	}
)
