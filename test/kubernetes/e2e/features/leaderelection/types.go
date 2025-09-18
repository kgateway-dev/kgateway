package leaderelection

import (
	"path/filepath"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/fsutils"
	e2edefaults "github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e/defaults"
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e/tests/base"
)

const leaseRenewPeriod = 10 * time.Second

var (
	// manifests
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

	routeObjectMeta = metav1.ObjectMeta{
		Name:      "httpbin",
		Namespace: "default",
	}

	setup = base.TestCase{
		Manifests: []string{e2edefaults.CurlPodManifest, e2edefaults.HttpbinManifest},
	}

	// test cases
	testCases = map[string]*base.TestCase{
		"TestLeaderAndFollowerAction": {
			Manifests: []string{gatewayManifest},
		},
		"TestLeaderWritesBackendStatus": {},
		"TestLeaderDeploysProxy":        {},
	}
)
