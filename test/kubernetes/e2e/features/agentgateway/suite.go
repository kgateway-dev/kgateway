package agentgateway

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/stretchr/testify/suite"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/kubeutils"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/requestutils/curl"
	"github.com/kgateway-dev/kgateway/v2/test/gomega/matchers"
	"github.com/kgateway-dev/kgateway/v2/test/testutils"

	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e"
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e/defaults"
)

var (
	// The self-managed Gateway and deployed Gateway should have the same name
	proxyObjMeta = metav1.ObjectMeta{
		Name:      "agent-gateway",
		Namespace: "default",
	}
	proxyDeployment = &appsv1.Deployment{ObjectMeta: proxyObjMeta}
	proxyService    = &corev1.Service{ObjectMeta: proxyObjMeta}
)

type testingSuite struct {
	suite.Suite

	ctx context.Context

	testInstallation *e2e.TestInstallation

	rootDir string

	installNamespace string

	manifests map[string][]string
}

func NewTestingSuite(ctx context.Context, testInst *e2e.TestInstallation) suite.TestingSuite {
	return &testingSuite{
		ctx:              ctx,
		testInstallation: testInst,
		rootDir:          testutils.GitRootDirectory(),
		installNamespace: os.Getenv(testutils.InstallNamespace),
	}
}

func (s *testingSuite) SetupSuite() {
	s.manifests = map[string][]string{
		"TestAgentGatewayDeployment": {
			deployAgentGatewayManifest,
			a2aAgentManifest,
			defaults.CurlPodManifest,
		},
	}
}

func (s *testingSuite) TearDownSuite() {
}

func (s *testingSuite) BeforeTest(suiteName, testName string) {
	manifests := s.manifests[testName]
	fmt.Printf("Applying manifests for test %s in suite %s", testName, suiteName)
	for _, manifest := range manifests {
		err := s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, manifest)
		s.Require().NoError(err)
	}
}

func (s *testingSuite) AfterTest(suiteName, testName string) {
	if s.T().Failed() {
		s.testInstallation.PreFailHandler(s.ctx)
	}
	manifests := s.manifests[testName]
	fmt.Printf("Deleting manifests for test %s in suite %s", testName, suiteName)
	for _, manifest := range manifests {
		err := s.testInstallation.Actions.Kubectl().DeleteFileSafe(s.ctx, manifest)
		s.Require().NoError(err)
	}
}

func (s *testingSuite) TestAgentGatewayDeployment() {
	// assert the expected resources are created and running before attempting to send traffic
	s.testInstallation.Assertions.EventuallyObjectsExist(s.ctx, proxyService, proxyDeployment)
	// check curl pod is running
	s.testInstallation.Assertions.EventuallyPodsRunning(s.ctx, defaults.CurlPod.GetNamespace(), metav1.ListOptions{
		LabelSelector: "app.kubernetes.io/name=curl",
	}, time.Minute*2)
	// match auto labels created by kgateway deployer
	s.testInstallation.Assertions.EventuallyPodsRunning(s.ctx, proxyObjMeta.GetNamespace(), metav1.ListOptions{
		LabelSelector: "app.kubernetes.io/name=agent-gateway",
	}, time.Minute*2)

	s.testA2ARouting()
}

func (s *testingSuite) testA2ARouting() {
	// Check agentgateway, a2a-agent and mcp-tool are running
	s.testInstallation.Assertions.EventuallyPodsRunning(s.ctx, "default", metav1.ListOptions{
		LabelSelector: "app=a2a-agent",
	}, time.Minute*2)

	// Check A2A Agent endpoint is reachable through the agentgateway
	/*
		curl -X POST http://localhost:9090/default-a2a-agent \
		  -H "Content-Type: application/json" \
		  -v \
		  -d '{
		    "jsonrpc": "2.0",
		    "id": "1",
		    "method": "tasks/send",
		    "params": {
		      "id": "1",
		      "message": {
		        "role": "user",
		        "parts": [
		          {
		            "type": "text",
		            "text": "hello gateway!"
		          }
		        ]
		      }
		    }
		  }'
	*/
	data := `{"jsonrpc":"2.0","id":"1","method":"tasks/send","params":{"id":"1","message":{"role":"user","parts":[{"type":"text","text":"hello gateway!"}]}}}`
	s.testInstallation.Assertions.AssertEventualCurlResponse(
		s.ctx,
		defaults.CurlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(proxyService.ObjectMeta)),
			curl.WithPort(a2aPort),
			curl.WithPath("/default-a2a-agent"),
			curl.WithContentType("application/json"),
			curl.WithMethod(http.MethodPost),
			curl.WithBody(data),
		},
		&matchers.HttpResponse{
			StatusCode: http.StatusOK,
		}, time.Minute*2)
}
