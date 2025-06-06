package inferenceextension

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/suite"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/kubeutils"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/requestutils/curl"
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e"
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e/defaults"
)

// testingSuite is the entire Suite of tests for testing K8s Service-specific features/fixes
type testingSuite struct {
	suite.Suite

	ctx context.Context

	// testInstallation contains all the metadata/utilities necessary to execute a series of tests
	// against a kgateway installation
	testInstallation *e2e.TestInstallation
}

func NewTestingSuite(ctx context.Context, testInst *e2e.TestInstallation) suite.TestingSuite {
	return &testingSuite{
		ctx:              ctx,
		testInstallation: testInst,
	}
}

func (s *testingSuite) TestHTTPRouteWithInferencePool() {
	s.T().Cleanup(func() {
		for _, m := range [][]byte{
			// reverse order of install
			clientManifest,
			routeManifest,
			gtwManifest,
			poolManifest,
			eppManifest,
			modelsManifest,
			vllmManifest,
		} {
			err := s.testInstallation.Actions.Kubectl().Delete(s.ctx, m)
			s.NoError(err, "can delete manifest %s", m)
		}
	})

	// Install test manifests
	for _, m := range [][]byte{
		vllmManifest,
		modelsManifest,
		poolManifest,
		eppManifest,
		gtwManifest,
		routeManifest,
		clientManifest,
	} {
		err := s.testInstallation.Actions.Kubectl().Apply(s.ctx, m)
		s.NoError(err, "can apply manifest %s", m)
	}

	// Assert test pods are running using key=pod_name and value=pod_namespace map.
	for k, v := range map[string]string{
		vllmDeployName:          testNS,
		vllmDeployName + "-epp": testNS,
		"curl":                  "curl"} {
		s.testInstallation.Assertions.EventuallyPodsRunning(s.ctx, v, metav1.ListOptions{
			LabelSelector: "app=" + k,
		}, podRunTimeout)
	}

	// Assert gateway service and deployment are created
	s.testInstallation.Assertions.EventuallyObjectsExist(s.ctx, gtwService, gtwDeployment)

	// Assert gateway programmed condition
	s.testInstallation.Assertions.EventuallyGatewayCondition(
		s.ctx,
		gtwObjectMeta.Name,
		gtwObjectMeta.Namespace,
		gwv1.GatewayConditionProgrammed,
		metav1.ConditionTrue,
		gtwProgramTimeout,
	)

	conditions := []gwv1.RouteConditionType{gwv1.RouteConditionAccepted, gwv1.RouteConditionResolvedRefs}
	for _, c := range conditions {
		s.testInstallation.Assertions.EventuallyHTTPRouteCondition(
			s.ctx,
			"llm-route", // Hardcoded name in route.yaml
			testNS,
			c,
			metav1.ConditionTrue,
		)
	}

	// TODO [danehans]: Assert InferencePool status when https://github.com/kgateway-dev/kgateway/pull/11230 merges

	// Exercise OpenAI API endpoint test cases
	type apiTest struct {
		api              string
		promptOrMessages string
	}

	tests := []apiTest{
		// Call with a single "prompt" field
		{
			api:              "/v1/completions",
			promptOrMessages: "Write as if you were a critic: San Francisco",
		},
		// Call with one user message
		{
			api:              "/v1/chat/completions",
			promptOrMessages: `[{"role":"user","content":"Write as if you were a critic: San Francisco"}]`,
		},
		// Call with a user–assistant–user message sequence
		{
			api: "/v1/chat/completions",
			promptOrMessages: `[{"role":"user","content":"Write as if you were a critic: San Francisco"},` +
				`{"role":"assistant","content":"Okay, let's see..."},` +
				`{"role":"user","content":"Now summarize your thoughts."}]`,
		},
	}

	for i := range tests {
		tc := tests[i]
		testName := fmt.Sprintf("CurlTestCase%d", i)

		s.T().Run(testName, func(t *testing.T) {
			// Build the "prompt" or "messages" fragment of the request body.
			var fieldJSON string
			if tc.api == "/v1/completions" {
				fieldJSON = fmt.Sprintf(`"prompt":"%s"`, tc.promptOrMessages)
			} else {
				fieldJSON = fmt.Sprintf(`"messages":%s`, tc.promptOrMessages)
			}

			// Inject that field into the rest of the body template
			body := fmt.Sprintf(
				`{"model":"%s",%s,"max_tokens":100,"temperature":0}`,
				baseModelName,
				fieldJSON,
			)

			// Assert expected curl response
			s.testInstallation.Assertions.AssertEventualCurlResponse(
				s.ctx,
				defaults.CurlPodExecOpt,
				[]curl.Option{
					curl.WithHost(kubeutils.ServiceFQDN(gtwService.ObjectMeta)),
					curl.WithHeader("Content-Type", "application/json"),
					curl.WithPath(tc.api),
					curl.WithBody(body),
				},
				expectedVllmResp,
			)
		})
	}
}
