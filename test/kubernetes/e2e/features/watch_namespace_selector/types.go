package watch_namespace_selector

import (
	"path/filepath"

	"github.com/solo-io/gloo/test/kubernetes/e2e/tests/base"
	"github.com/solo-io/skv2/codegen/util"
	"sigs.k8s.io/controller-runtime/pkg/client"

	gatewayv1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1/kube/apis/gateway.solo.io/v1"
	e2edefaults "github.com/solo-io/gloo/test/kubernetes/e2e/defaults"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	installNamespaceVSManifest = filepath.Join(util.MustGetThisDir(), "testdata", "vs-install-ns.yaml")

	matchLabelsSetup      = filepath.Join(util.MustGetThisDir(), "testdata", "match-labels.yaml")
	matchExpressionsSetup = filepath.Join(util.MustGetThisDir(), "testdata", "match-expressions.yaml")

	unlabeledRandomNamespaceManifest = filepath.Join(util.MustGetThisDir(), "testdata", "random-ns-unlabeled.yaml")
	randomVSManifest                 = filepath.Join(util.MustGetThisDir(), "testdata", "vs-random.yaml")

	randomUpstreamManifest                       = filepath.Join(util.MustGetThisDir(), "testdata", "upstream-random.yaml")
	installNamespaceWithRandomUpstreamVSManifest = filepath.Join(util.MustGetThisDir(), "testdata", "vs-upstream.yaml")

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
				UpgradeValues: matchLabelsSetup,
				Manifests:     []string{unlabeledRandomNamespaceManifest, randomVSManifest},
				Resources:     []client.Object{randomNamespaceVS},
			},
		},
		"TestMatchExpressions": {
			SimpleTestCase: base.SimpleTestCase{
				UpgradeValues: matchExpressionsSetup,
				Manifests:     []string{unlabeledRandomNamespaceManifest, randomVSManifest},
				Resources:     []client.Object{randomNamespaceVS},
			},
		},
		"TestUnwatchedNamespaceValidation": {
			SimpleTestCase: base.SimpleTestCase{
				UpgradeValues: matchLabelsSetup,
			},
		},
		"TestWatchedNamespaceValidation": {
			SimpleTestCase: base.SimpleTestCase{
				UpgradeValues: matchExpressionsSetup,
			},
		},
	}
)
