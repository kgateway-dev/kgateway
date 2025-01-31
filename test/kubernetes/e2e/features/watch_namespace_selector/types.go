//go:build ignore

package watch_namespace_selector

import (
	"path/filepath"

	"github.com/kgateway-dev/kgateway/pkg/utils/fsutils"
	"github.com/kgateway-dev/kgateway/test/kubernetes/e2e/tests/base"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	gatewayv1 "github.com/kgateway-dev/kgateway/projects/gateway/pkg/api/v1/kube/apis/gateway.solo.io/v1"
	e2edefaults "github.com/kgateway-dev/kgateway/test/kubernetes/e2e/defaults"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	installNamespaceVSManifest = filepath.Join(fsutils.MustGetThisDir(), "testdata", "vs-install-ns.yaml")

	unlabeledRandomNamespaceManifest = filepath.Join(fsutils.MustGetThisDir(), "testdata", "random-ns-unlabeled.yaml")
	randomNamespace                  = &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "random",
		},
	}

	randomVSManifest = filepath.Join(fsutils.MustGetThisDir(), "testdata", "vs-random.yaml")

	randomUpstreamManifest                       = filepath.Join(fsutils.MustGetThisDir(), "testdata", "upstream-random.yaml")
	installNamespaceWithRandomUpstreamVSManifest = filepath.Join(fsutils.MustGetThisDir(), "testdata", "vs-upstream.yaml")

	randomNamespaceVS = &gatewayv1.VirtualService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "vs-random",
			Namespace: "random",
		},
	}

	setupSuite = base.SimpleTestCase{
		Manifests: []string{e2edefaults.CurlPodManifest},
	}

	testCases = map[string]*base.TestCase{
		"TestMatchLabels": {
			SimpleTestCase: base.SimpleTestCase{
				Manifests: []string{unlabeledRandomNamespaceManifest, randomVSManifest},
				Resources: []client.Object{randomNamespace, randomNamespaceVS},
			},
		},
		"TestMatchExpressions": {
			SimpleTestCase: base.SimpleTestCase{
				Manifests: []string{unlabeledRandomNamespaceManifest, randomVSManifest},
				Resources: []client.Object{randomNamespace, randomNamespaceVS},
			},
		},
		"TestUnwatchedNamespaceValidation": {
			SimpleTestCase: base.SimpleTestCase{},
		},
		"TestWatchedNamespaceValidation": {
			SimpleTestCase: base.SimpleTestCase{},
		},
	}
)
