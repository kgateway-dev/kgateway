package leaderelection

import (
	"path/filepath"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/fsutils"
	e2edefaults "github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e/defaults"
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e/tests/base"
)

const leaseRenewPeriod = 10 * time.Second

var (
	// manifests
	setupManifest   = filepath.Join(fsutils.MustGetThisDir(), "testdata", "setup.yaml")
	gatewayManifest = filepath.Join(fsutils.MustGetThisDir(), "testdata", "gateway.yaml")
	routeManifest   = filepath.Join(fsutils.MustGetThisDir(), "testdata", "route.yaml")
	backendManifest = filepath.Join(fsutils.MustGetThisDir(), "testdata", "backend.yaml")

	// setup objects
	proxyObjectMeta = metav1.ObjectMeta{
		Name:      "gw",
		Namespace: "default",
	}
	proxyDeployment = &appsv1.Deployment{ObjectMeta: proxyObjectMeta}
	proxyService    = &corev1.Service{ObjectMeta: proxyObjectMeta}

	httpbinSvc = &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "httpbin",
			Namespace: "httpbin",
		},
	}
	httpbinDeployment = &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "httpbin",
			Namespace: "httpbin",
		},
	}

	setup = base.SimpleTestCase{
		Manifests: []string{e2edefaults.CurlPodManifest, setupManifest},
		Resources: []client.Object{e2edefaults.CurlPod, httpbinSvc, httpbinDeployment},
	}

	// test cases
	testCases = map[string]*base.TestCase{
		"TestLeaderAndFollowerAction": {
			SimpleTestCase: base.SimpleTestCase{
				Manifests: []string{gatewayManifest},
			},
		},
		"TestLeaderWritesBackendStatus": {
			SimpleTestCase: base.SimpleTestCase{},
		},
		"TestLeaderDeploysProxy": {
			SimpleTestCase: base.SimpleTestCase{},
		},
	}
)
