package lambda

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/onsi/gomega"
	"github.com/stretchr/testify/suite"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/kubeutils"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/requestutils/curl"
	testmatchers "github.com/kgateway-dev/kgateway/v2/test/gomega/matchers"
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e"
	testdefaults "github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e/defaults"
)

// testingSuite is a suite of Lambda backend routing tests
type testingSuite struct {
	suite.Suite
	ctx         context.Context
	ti          *e2e.TestInstallation
	manifests   map[string][]string
	endpointURL string
}

var _ e2e.NewSuiteFunc = NewTestingSuite

func NewTestingSuite(ctx context.Context, testInst *e2e.TestInstallation) suite.TestingSuite {
	return &testingSuite{
		ctx: ctx,
		ti:  testInst,
	}
}

func (s *testingSuite) SetupSuite() {
	err := s.ti.Actions.Kubectl().ApplyFile(s.ctx, setupManifest)
	s.NoError(err, "can apply "+setupManifest)
	err = s.ti.Actions.Kubectl().ApplyFile(s.ctx, testdefaults.CurlPodManifest)
	s.NoError(err, "can apply curl pod manifest")
	err = s.ti.Actions.Kubectl().ApplyFile(s.ctx, awsCliPodManifest)
	s.NoError(err, "can apply aws-cli pod manifest")

	s.ti.Assertions.EventuallyObjectsExist(s.ctx, testdefaults.CurlPod)
	s.ti.Assertions.EventuallyPodsRunning(s.ctx, testdefaults.CurlPod.GetNamespace(), metav1.ListOptions{
		LabelSelector: "app.kubernetes.io/name=curl",
	})
	s.ti.Assertions.EventuallyPodReady(s.ctx, "lambda-test", "aws-cli", 30*time.Second)

	s.manifests = map[string][]string{
		"TestLambdaBackendRouting":      {lambdaBackendManifest},
		"TestLambdaBackendAsyncRouting": {lambdaAsyncManifest},
		"TestLambdaBackendQualifier":    {lambdaQualifierManifest},
	}

	s.extractLocalstackEndpoint()
	s.createLambdaFunctions()
}

func (s *testingSuite) TearDownSuite() {
	err := s.ti.Actions.Kubectl().DeleteFileSafe(s.ctx, setupManifest)
	s.NoError(err, "can delete setup manifest")
	err = s.ti.Actions.Kubectl().DeleteFileSafe(s.ctx, testdefaults.CurlPodManifest)
	s.NoError(err, "can delete curl pod manifest")
}

func (s *testingSuite) BeforeTest(suiteName, testName string) {
	manifests, ok := s.manifests[testName]
	if !ok {
		s.FailNow("no manifests found for %s, manifest map contents: %v", testName, s.manifests)
	}

	for _, manifest := range manifests {
		// Read the manifest content
		content, err := os.ReadFile(manifest)
		s.Assert().NoError(err, "can read manifest "+manifest)

		// Replace the endpointURL placeholder with actual URL
		newContent := strings.Replace(string(content), "http://172.18.0.2:31566", s.endpointURL, -1)
		tmpFile, err := os.CreateTemp("", "lambda-manifest-*.yaml")
		s.Assert().NoError(err, "can create temp file")
		defer os.Remove(tmpFile.Name())

		_, err = tmpFile.WriteString(newContent)
		s.Assert().NoError(err, "can write to temp file")
		err = tmpFile.Close()
		s.Assert().NoError(err, "can close temp file")

		err = s.ti.Actions.Kubectl().WithReceiver(os.Stdout).ApplyFile(s.ctx, tmpFile.Name())
		s.Require().NoError(err, "can apply manifest "+manifest)
	}

	s.ti.Assertions.EventuallyObjectsExist(s.ctx, testdefaults.CurlPod)
	s.ti.Assertions.EventuallyPodsRunning(s.ctx, testdefaults.CurlPod.GetNamespace(), metav1.ListOptions{
		LabelSelector: "app.kubernetes.io/name=curl",
	})

	s.ti.Assertions.EventuallyObjectsExist(s.ctx, proxyServiceMeta)
	s.ti.Assertions.EventuallyObjectsExist(s.ctx, proxyDeploymentMeta)
	s.ti.Assertions.EventuallyPodsRunning(s.ctx, proxyDeploymentMeta.GetNamespace(), metav1.ListOptions{
		LabelSelector: fmt.Sprintf("app.kubernetes.io/name=%s", gatewayName),
	})
}

func (s *testingSuite) AfterTest(suiteName, testName string) {
	manifests, ok := s.manifests[testName]
	if !ok {
		s.FailNow("no manifests found for %s, manifest map contents: %v", testName, s.manifests)
	}
	for _, manifest := range manifests {
		err := s.ti.Actions.Kubectl().DeleteFile(s.ctx, manifest, "--grace-period", "0")
		s.NoError(err, "can delete manifest "+manifest)
	}
}

func (s *testingSuite) TestLambdaBackendRouting() {
	// Test Lambda backend with custom endpoint
	s.ti.Assertions.AssertEventualCurlResponse(
		s.ctx,
		testdefaults.CurlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(gatewayObjectMeta)),
			curl.WithHostHeader("www.example.com"),
			curl.WithPort(8080),
			curl.WithPath("/lambda"),
		},
		&testmatchers.HttpResponse{
			StatusCode: http.StatusOK,
			Body:       gomega.ContainSubstring(`Hello from Lambda`),
		},
	)

	// Test Lambda backend with Envoy payload transformation disabled
	s.ti.Assertions.AssertEventualCurlResponse(
		s.ctx,
		testdefaults.CurlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(gatewayObjectMeta)),
			curl.WithHostHeader("www.example.com"),
			curl.WithPort(8080),
			curl.WithPath("/lambda/no-payload-transform"),
			curl.WithBody("{}"), // JSON payload is a requirement when Envoy payload transformation is disabled
		},
		// Ensure the JSON transformation Envoy applies are not a part of the lambda's response body:
		// https://www.envoyproxy.io/docs/envoy/latest/configuration/http/http_filters/aws_lambda_filter#configuration-as-a-listener-filter
		&testmatchers.HttpResponse{
			StatusCode: http.StatusOK,
			Body: gomega.And(
				gomega.ContainSubstring(`Hello from Lambda`),
				gomega.Not(gomega.ContainSubstring(`raw_path`)),
				gomega.Not(gomega.ContainSubstring(`method`)),
				gomega.Not(gomega.ContainSubstring(`headers`)),
				gomega.Not(gomega.ContainSubstring(`query_string_parameters`)),
				gomega.Not(gomega.ContainSubstring(`is_base64_encoded`)),
			),
		},
	)
}

func (s *testingSuite) TestLambdaBackendAsyncRouting() {
	// Test Lambda backend with custom endpoint
	s.ti.Assertions.AssertEventualCurlResponse(
		s.ctx,
		testdefaults.CurlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(gatewayObjectMeta)),
			curl.WithHostHeader("www.example.com"),
			curl.WithPort(8080),
			curl.WithPath("/lambda"),
		},
		&testmatchers.HttpResponse{
			StatusCode: http.StatusAccepted,
			Body:       gomega.BeEmpty(),
		},
	)
}

func (s *testingSuite) TestLambdaBackendQualifier() {
	// Test Lambda backend with the prod qualifier
	s.ti.Assertions.AssertEventualCurlResponse(
		s.ctx,
		testdefaults.CurlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(gatewayObjectMeta)),
			curl.WithHostHeader("www.example.com"),
			curl.WithPort(8080),
			curl.WithPath("/lambda/prod"),
		},
		&testmatchers.HttpResponse{
			StatusCode: http.StatusOK,
			Body:       gomega.ContainSubstring(`"message":"Hello from Lambda prod"`),
		},
	)

	// Test Lambda backend with the dev qualifier
	s.ti.Assertions.AssertEventualCurlResponse(
		s.ctx,
		testdefaults.CurlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(gatewayObjectMeta)),
			curl.WithHostHeader("www.example.com"),
			curl.WithPort(8080),
			curl.WithPath("/lambda/dev"),
		},
		&testmatchers.HttpResponse{
			StatusCode: http.StatusOK,
			Body:       gomega.ContainSubstring(`"message":"Hello from Lambda dev"`),
		},
	)

	// Test Lambda backend with the latest qualifier
	s.ti.Assertions.AssertEventualCurlResponse(
		s.ctx,
		testdefaults.CurlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(gatewayObjectMeta)),
			curl.WithHostHeader("www.example.com"),
			curl.WithPort(8080),
			curl.WithPath("/lambda/latest"),
		},
		&testmatchers.HttpResponse{
			StatusCode: http.StatusOK,
			Body:       gomega.ContainSubstring(`"message":"Hello from Lambda $LATEST"`),
		},
	)

	// Test non-existent qualifier returns 404
	s.ti.Assertions.AssertEventualCurlResponse(
		s.ctx,
		testdefaults.CurlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(gatewayObjectMeta)),
			curl.WithHostHeader("www.example.com"),
			curl.WithPort(8080),
			curl.WithPath("/lambda/nonexistent"),
		},
		&testmatchers.HttpResponse{
			StatusCode: http.StatusNotFound,
		},
	)
}

func (s *testingSuite) extractLocalstackEndpoint() {
	s.T().Log("extracting localstack endpoint URL from cluster")
	c := s.ti.ClusterContext.Client

	var nodes corev1.NodeList
	err := c.List(s.ctx, &nodes)
	s.Require().NoError(err, "can list nodes")
	s.Require().NotEmpty(nodes.Items, "cluster has at least one node")

	var nodeIP string
	for _, node := range nodes.Items {
		for _, addr := range node.Status.Addresses {
			if addr.Type == "InternalIP" {
				nodeIP = addr.Address
				break
			}
		}
	}
	s.Require().NotEmpty(nodeIP, "node has internal IP")

	var svc corev1.Service
	err = c.Get(s.ctx, client.ObjectKeyFromObject(&localstackService), &svc)
	s.Require().NoError(err, "can get localstack service")
	s.Require().Greater(len(svc.Spec.Ports), 0, "localstack service has ports")
	port := svc.Spec.Ports[0].NodePort
	s.Require().NotZero(port, "localstack service has node port")

	s.endpointURL = fmt.Sprintf("http://%s:%d", nodeIP, port)
	u, err := url.Parse(s.endpointURL)
	s.Require().NoError(err, "can parse localstack endpoint URL")
	s.endpointURL = u.String()
	s.T().Logf("localstack endpoint URL: %s", s.endpointURL)
}

func (s *testingSuite) createLambdaFunctions() {
	k := s.ti.Actions.Kubectl().WithReceiver(io.Discard)

	// Check if function exists and delete it if it does
	err := k.RunCommand(s.ctx, "exec", "-n", "lambda-test", "aws-cli", "--",
		"aws", "lambda", "get-function",
		"--endpoint-url", s.endpointURL,
		"--function-name", "hello-function")
	if err == nil {
		// Function exists, delete it
		err = k.RunCommand(s.ctx, "exec", "-n", "lambda-test", "aws-cli", "--",
			"aws", "lambda", "delete-function",
			"--endpoint-url", s.endpointURL,
			"--function-name", "hello-function")
		s.Require().NoError(err, "can delete existing function")

		// Wait a bit to ensure the function is fully deleted
		err = k.RunCommand(s.ctx, "exec", "-n", "lambda-test", "aws-cli", "--", "sleep", "5")
		s.Require().NoError(err, "can wait for function deletion")
	}

	functionCode, err := os.ReadFile(lambdaFunctionPath)
	s.Require().NoError(err, "can read function code")

	// Create the function code directly in the pod using base64 to preserve formatting
	encodedCode := base64.StdEncoding.EncodeToString(functionCode)
	err = k.RunCommand(s.ctx, "exec", "-n", "lambda-test", "aws-cli", "--",
		"sh", "-c", fmt.Sprintf("echo %s | base64 -d > /tmp/hello-function.js", encodedCode))
	s.Require().NoError(err, "can create function code in pod")

	// Create the zip file in the pod
	err = k.RunCommand(s.ctx, "exec", "-n", "lambda-test", "aws-cli", "--", "zip", "-j", "/tmp/function.zip", "/tmp/hello-function.js")
	s.Require().NoError(err, "can create zip file")

	// Create the Lambda functions with different qualifiers
	err = k.RunCommand(s.ctx, "exec", "-n", "lambda-test", "aws-cli", "--",
		"aws", "lambda", "create-function",
		"--endpoint-url", s.endpointURL,
		"--function-name", "hello-function",
		"--runtime", "nodejs18.x",
		"--handler", "hello-function.handler",
		"--role", "arn:aws:iam::000000000000:role/lambda-role",
		"--zip-file", "fileb:///tmp/function.zip")
	s.Require().NoError(err, "can create base function")

	// Create function versions
	err = k.RunCommand(s.ctx, "exec", "-n", "lambda-test", "aws-cli", "--",
		"aws", "lambda", "publish-version",
		"--endpoint-url", s.endpointURL,
		"--function-name", "hello-function")
	s.Require().NoError(err, "can publish version")

	// Create aliases (qualifiers)
	err = k.RunCommand(s.ctx, "exec", "-n", "lambda-test", "aws-cli", "--",
		"aws", "lambda", "create-alias",
		"--endpoint-url", s.endpointURL,
		"--function-name", "hello-function",
		"--name", "prod",
		"--function-version", "1")
	s.Require().NoError(err, "can create prod alias")

	err = k.RunCommand(s.ctx, "exec", "-n", "lambda-test", "aws-cli", "--",
		"aws", "lambda", "create-alias",
		"--endpoint-url", s.endpointURL,
		"--function-name", "hello-function",
		"--name", "dev",
		"--function-version", "1")
	s.Require().NoError(err, "can create dev alias")
}
