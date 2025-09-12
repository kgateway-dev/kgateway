package agentgateway

import (
	"path/filepath"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/fsutils"
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e/defaults"
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e/tests/base"
)

var (
	// kgateway managed deployment for the agentgateway
	deployAgentGatewayManifest = filepath.Join(fsutils.MustGetThisDir(), "testdata", "agentgateway-deploy.yaml")

	// Core infrastructure objects that we need to track
	gatewayObjectMeta = metav1.ObjectMeta{
		Name:      "agent-gateway",
		Namespace: "default",
	}

	gatewayParamsObjectMeta = metav1.ObjectMeta{
		Name:      "kgateway",
		Namespace: "default",
	}

	testCases = map[string]*base.TestCase{
		"TestAgentGatewayDeployment": {
			Manifests: []string{defaults.HttpbinManifest, defaults.CurlPodManifest, deployAgentGatewayManifest},
		},
	}
)
