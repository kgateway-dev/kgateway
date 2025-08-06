package path_matching

import (
	"path/filepath"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/fsutils"
	e2edefaults "github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e/defaults"
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e/tests/base"
)

var (
	// manifests
	setupManifest         = filepath.Join(fsutils.MustGetThisDir(), "testdata", "setup.yaml")
	exactManifest         = filepath.Join(fsutils.MustGetThisDir(), "testdata", "exact.yaml")
	prefixManifest        = filepath.Join(fsutils.MustGetThisDir(), "testdata", "prefix.yaml")
	regexManifest         = filepath.Join(fsutils.MustGetThisDir(), "testdata", "regex.yaml")
	prefixRewriteManifest = filepath.Join(fsutils.MustGetThisDir(), "testdata", "prefix-rewrite.yaml")

	// Core infrastructure objects that we need to track
	gatewayObjectMeta = metav1.ObjectMeta{
		Name:      "gw",
		Namespace: "default",
	}
	gatewayService    = &corev1.Service{ObjectMeta: gatewayObjectMeta}
	gatewayDeployment = &appsv1.Deployment{ObjectMeta: gatewayObjectMeta}

	httpbinObjectMeta = metav1.ObjectMeta{
		Name:      "httpbin",
		Namespace: "httpbin",
	}
	httpbinDeployment = &appsv1.Deployment{ObjectMeta: httpbinObjectMeta}

	route = &gwv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "httpbin",
			Namespace: "httpbin",
		},
	}

	setup = base.TestCase{
		Manifests: []string{e2edefaults.CurlPodManifest, setupManifest},
		Resources: []client.Object{e2edefaults.CurlPod, httpbinDeployment, gatewayService, gatewayDeployment},
	}

	// test cases
	testCases = map[string]base.TestCase{
		"TestExactMatch": base.TestCase{
			Manifests: []string{exactManifest},
			Resources: []client.Object{route},
		},
		"TestPrefixMatch": base.TestCase{
			Manifests: []string{prefixManifest},
			Resources: []client.Object{route},
		},
		"TestRegexMatch": base.TestCase{
			Manifests: []string{regexManifest},
			Resources: []client.Object{route},
		},
		"TestPrefixRewrite": base.TestCase{
			Manifests: []string{prefixRewriteManifest},
			Resources: []client.Object{route},
		},
	}
)
