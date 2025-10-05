package inferenceextension

import (
	"bytes"
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	inf "sigs.k8s.io/gateway-api-inference-extension/api/v1"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	"sigs.k8s.io/yaml"

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

	// manifests is a map of manifests keyed by a test name
	manifests map[string][][]byte
}

func NewTestingSuite(ctx context.Context, testInst *e2e.TestInstallation) suite.TestingSuite {
	return &testingSuite{
		ctx:              ctx,
		testInstallation: testInst,
		manifests:        map[string][][]byte{},
	}
}

// TearDownTest runs after each test to clean up CRDs
func (s *testingSuite) TearDownTest() {
	// Clean up CRDs that may have been left behind with old namespace annotations
	crds := []string{
		"gatewayparameters.gateway.kgateway.dev",
		"xlistenersets.gateway.kgateway.dev",
		"listenersets.gateway.kgateway.dev",
		"inferencepools.gateway.kgateway.dev",
		// Add any other CRDs used by your tests
	}

	for _, crd := range crds {
		// Delete CRD - ignore errors as it may not exist
		crdManifest := []byte(fmt.Sprintf(`
apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: %s
`, crd))
		_ = s.testInstallation.Actions.Kubectl().Delete(s.ctx, crdManifest)
	}

	// Wait for cleanup to complete
	time.Sleep(2 * time.Second)
}

// injectNamespace injects namespace into manifests that need it
func injectNamespace(manifest []byte, namespace string) []byte {
	// Only inject namespace for XListenerSet manifests
	if !bytes.Contains(manifest, []byte("kind: XListenerSet")) {
		return manifest
	}

	var obj map[string]interface{}
	if err := yaml.Unmarshal(manifest, &obj); err != nil {
		return manifest // Return original on error
	}

	// Inject namespace into metadata
	if metadata, ok := obj["metadata"].(map[string]interface{}); ok {
		metadata["namespace"] = namespace
		if modified, err := yaml.Marshal(obj); err == nil {
			return modified
		}
	}

	return manifest
}

func (s *testingSuite) TestHTTPRouteWithInferencePool() {
	testName := "TestHTTPRouteWithInferencePool"

	s.T().Cleanup(func() {
		manifests, ok := s.manifests[testName]
		if !ok {
			s.FailNow("no manifests found for %s", testName)
		}

		// Delete manifests in reverse order
		for i := len(manifests) - 1; i >= 0; i-- {
			err := s.testInstallation.Actions.Kubectl().Delete(s.ctx, manifests[i])
			s.NoError(err, "can delete manifest %s", manifests[i])
		}
	})

	// Add the testdata manifests to the manifests map
	s.manifests[testName] = [][]byte{
		clientManifest,
		vllmManifest,
		gtwManifest,
		poolManifest,
		eppManifest,
		routeManifest,
	}

	// Apply the testdata manifests
	for _, m := range s.manifests[testName] {
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
			testRouteName,
			testNS,
			c,
			metav1.ConditionTrue,
		)
	}

	s.testInstallation.Assertions.EventuallyInferencePoolCondition(
		s.ctx,
		vllmDeployName,
		testNS,
		inf.InferencePoolConditionAccepted,
		metav1.ConditionTrue,
	)

	s.testInstallation.Assertions.EventuallyInferencePoolCondition(
		s.ctx,
		vllmDeployName,
		testNS,
		inf.InferencePoolConditionResolvedRefs,
		metav1.ConditionTrue,
	)

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
				targetModelName,
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

func (s *testingSuite) TestHTTPRouteWithListenerSetParentRef() {
	testName := "TestHTTPRouteWithListenerSetParentRef"

	s.T().Cleanup(func() {
		manifests, ok := s.manifests[testName]
		if !ok {
			s.FailNow("no manifests found for %s", testName)
		}

		// Delete manifests in reverse order
		for i := len(manifests) - 1; i >= 0; i-- {
			err := s.testInstallation.Actions.Kubectl().Delete(s.ctx, manifests[i])
			s.NoError(err, "can delete manifest %s", manifests[i])
		}
	})

	// Add the testdata manifests to the manifests map
	s.manifests[testName] = [][]byte{
		clientManifest,
		vllmManifest,
		gtwManifest,
		injectNamespace(listenersetManifest, testNS),
		poolManifest,
		eppManifest,
		routeListenerSetManifest,
	}

	// Apply the testdata manifests
	for _, m := range s.manifests[testName] {
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

	// Assert HTTPRoute conditions
	conditions := []gwv1.RouteConditionType{gwv1.RouteConditionAccepted, gwv1.RouteConditionResolvedRefs}
	for _, c := range conditions {
		s.testInstallation.Assertions.EventuallyHTTPRouteCondition(
			s.ctx,
			"llm-route-listenerset",
			testNS,
			c,
			metav1.ConditionTrue,
		)
	}

	// Assert InferencePool conditions
	s.testInstallation.Assertions.EventuallyInferencePoolCondition(
		s.ctx,
		vllmDeployName,
		testNS,
		inf.InferencePoolConditionAccepted,
		metav1.ConditionTrue,
	)

	s.testInstallation.Assertions.EventuallyInferencePoolCondition(
		s.ctx,
		vllmDeployName,
		testNS,
		inf.InferencePoolConditionResolvedRefs,
		metav1.ConditionTrue,
	)

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
				targetModelName,
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

func (s *testingSuite) TestHTTPRouteWithXListenerSetParentRef() {
	testName := "TestHTTPRouteWithXListenerSetParentRef"

	s.T().Cleanup(func() {
		manifests, ok := s.manifests[testName]
		if !ok {
			s.FailNow("no manifests found for %s", testName)
		}

		// Delete manifests in reverse order
		for i := len(manifests) - 1; i >= 0; i-- {
			err := s.testInstallation.Actions.Kubectl().Delete(s.ctx, manifests[i])
			s.NoError(err, "can delete manifest %s", manifests[i])
		}
	})

	// Add the testdata manifests to the manifests map
	s.manifests[testName] = [][]byte{
		clientManifest,
		vllmManifest,
		gtwManifest,
		injectNamespace(xlistenersetManifest, testNS),
		poolManifest,
		eppManifest,
		routeXListenerSetManifest,
	}

	// Apply the testdata manifests
	for _, m := range s.manifests[testName] {
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

	// Assert HTTPRoute conditions
	conditions := []gwv1.RouteConditionType{gwv1.RouteConditionAccepted, gwv1.RouteConditionResolvedRefs}
	for _, c := range conditions {
		s.testInstallation.Assertions.EventuallyHTTPRouteCondition(
			s.ctx,
			"llm-route-xlistenerset",
			testNS,
			c,
			metav1.ConditionTrue,
		)
	}

	// Assert InferencePool conditions
	s.testInstallation.Assertions.EventuallyInferencePoolCondition(
		s.ctx,
		vllmDeployName,
		testNS,
		inf.InferencePoolConditionAccepted,
		metav1.ConditionTrue,
	)

	s.testInstallation.Assertions.EventuallyInferencePoolCondition(
		s.ctx,
		vllmDeployName,
		testNS,
		inf.InferencePoolConditionResolvedRefs,
		metav1.ConditionTrue,
	)

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
				targetModelName,
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

func (s *testingSuite) TestHTTPRouteWithMixedParentRefs() {
	testName := "TestHTTPRouteWithMixedParentRefs"

	s.T().Cleanup(func() {
		manifests, ok := s.manifests[testName]
		if !ok {
			s.FailNow("no manifests found for %s", testName)
		}

		// Delete manifests in reverse order
		for i := len(manifests) - 1; i >= 0; i-- {
			err := s.testInstallation.Actions.Kubectl().Delete(s.ctx, manifests[i])
			s.NoError(err, "can delete manifest %s", manifests[i])
		}
	})

	// Add the testdata manifests to the manifests map
	s.manifests[testName] = [][]byte{
		clientManifest,
		vllmManifest,
		gtwManifest,
		injectNamespace(listenersetManifest, testNS),
		injectNamespace(xlistenersetManifest, testNS),
		poolManifest,
		eppManifest,
		routeMixedParentsManifest,
	}

	// Apply the testdata manifests
	for _, m := range s.manifests[testName] {
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

	// Assert HTTPRoute conditions
	conditions := []gwv1.RouteConditionType{gwv1.RouteConditionAccepted, gwv1.RouteConditionResolvedRefs}
	for _, c := range conditions {
		s.testInstallation.Assertions.EventuallyHTTPRouteCondition(
			s.ctx,
			"llm-route-mixed-parents",
			testNS,
			c,
			metav1.ConditionTrue,
		)
	}

	// Assert InferencePool conditions
	s.testInstallation.Assertions.EventuallyInferencePoolCondition(
		s.ctx,
		vllmDeployName,
		testNS,
		inf.InferencePoolConditionAccepted,
		metav1.ConditionTrue,
	)

	s.testInstallation.Assertions.EventuallyInferencePoolCondition(
		s.ctx,
		vllmDeployName,
		testNS,
		inf.InferencePoolConditionResolvedRefs,
		metav1.ConditionTrue,
	)

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
				targetModelName,
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
