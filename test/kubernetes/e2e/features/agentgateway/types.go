package agentgateway

import (
	"path/filepath"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/fsutils"
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e/defaults"
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e/tests/base"
)

var (
	// The self-managed Gateway and deployed Gateway should have the same name
	proxyObjMeta = metav1.ObjectMeta{
		Name:      "agent-gateway",
		Namespace: "default",
	}
	proxyDeployment = &appsv1.Deployment{ObjectMeta: proxyObjMeta}
	proxyService    = &corev1.Service{ObjectMeta: proxyObjMeta}

	// kgateway managed deployment for the agentgateway
	deployAgentGatewayManifest = filepath.Join(fsutils.MustGetThisDir(), "testdata", "agentgateway-deploy.yaml")

	// Core infrastructure objects that we need to track
	gatewayObjectMeta = metav1.ObjectMeta{
		Name:      "agent-gateway",
		Namespace: "default",
	}
	gatewayService = &corev1.Service{ObjectMeta: gatewayObjectMeta}

	testCases = map[string]base.TestCase{
		"TestAgentGatewayDeployment": {
			Manifests: []string{defaults.HttpbinManifest, defaults.CurlPodManifest, deployAgentGatewayManifest},
			Resources: []client.Object{proxyService, proxyDeployment, defaults.CurlPod},
		},
	}
)
