package zero_downtime_rollout

import (
	"path/filepath"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/fsutils"
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e/defaults"
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e/tests/base"
)

var (
	serviceManifest      = filepath.Join(fsutils.MustGetThisDir(), "testdata", "service.yaml")
	gatewayManifest      = filepath.Join(fsutils.MustGetThisDir(), "testdata", "gateway.yaml")
	agentgatewayManifest = filepath.Join(fsutils.MustGetThisDir(), "testdata", "agentgateway.yaml")

	proxyObjectMeta = metav1.ObjectMeta{
		Name:      "gw",
		Namespace: "default",
	}

	agentgatewayObjectMeta = metav1.ObjectMeta{
		Name:      "agentgw",
		Namespace: "default",
	}

	setup = base.TestCase{
		Manifests: []string{serviceManifest},
	}

	testCases = map[string]*base.TestCase{
		"TestZeroDowntimeRollout": {
			Manifests: []string{gatewayManifest, defaults.CurlPodManifest},
		},
		"TestZeroDowntimeRolloutAgentGateway": {
			Manifests: []string{agentgatewayManifest, defaults.CurlPodManifest},
		},
	}
)
