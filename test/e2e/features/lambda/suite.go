//go:build e2e

package lambda

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/onsi/gomega"
	"github.com/stretchr/testify/suite"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/kubeutils"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/requestutils/curl"
	"github.com/kgateway-dev/kgateway/v2/test/e2e"
	testdefaults "github.com/kgateway-dev/kgateway/v2/test/e2e/defaults"
	"github.com/kgateway-dev/kgateway/v2/test/e2e/testutils/localstack"
	"github.com/kgateway-dev/kgateway/v2/test/envoyutils/admincli"
	testmatchers "github.com/kgateway-dev/kgateway/v2/test/gomega/matchers"
	"github.com/kgateway-dev/kgateway/v2/test/testutils"
)

// stsSuccessStatRegex matches the 2xx request counter of the internal cluster
// Envoy creates for its assume-role credential provider's STS calls.
var stsSuccessStatRegex = regexp.MustCompile(`cluster\.sts_token_service_internal-us-east-1\.upstream_rq_2xx: (\d+)`)

// testingSuite is a suite of Lambda backend routing tests
type testingSuite struct {
	suite.Suite
	ctx          context.Context
	ti           *e2e.TestInstallation
	manifests    map[string][]string
	endpointURL  string
	stsServiceIP string
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

	s.ti.AssertionsT(s.T()).EventuallyObjectsExist(s.ctx, testdefaults.CurlPod)
	s.ti.AssertionsT(s.T()).EventuallyPodsRunning(s.ctx, testdefaults.CurlPod.GetNamespace(), metav1.ListOptions{
		LabelSelector: testdefaults.WellKnownAppLabel + "=curl",
	})
	s.ti.AssertionsT(s.T()).EventuallyPodReady(s.ctx, "lambda-test", "aws-cli", 30*time.Second)

	s.manifests = map[string][]string{
		"TestLambdaBackendRouting":      {lambdaBackendManifest},
		"TestLambdaBackendAsyncRouting": {lambdaAsyncManifest},
		"TestLambdaBackendQualifier":    {lambdaQualifierManifest},
		"TestLambdaBackendAssumeRole":   {lambdaAssumeRoleManifest},
	}

	err = s.ti.Actions.Kubectl().ApplyFile(s.ctx, stsServiceManifest)
	s.NoError(err, "can apply "+stsServiceManifest)

	s.extractLocalstackEndpoint()
	s.extractSTSServiceIP()
	s.createLambdaFunctions()
}

func (s *testingSuite) TearDownSuite() {
	if testutils.ShouldSkipCleanup(s.T()) {
		return
	}
	err := s.ti.Actions.Kubectl().DeleteFileSafe(s.ctx, setupManifest)
	s.NoError(err, "can delete setup manifest")
	err = s.ti.Actions.Kubectl().DeleteFileSafe(s.ctx, testdefaults.CurlPodManifest)
	s.NoError(err, "can delete curl pod manifest")
	err = s.ti.Actions.Kubectl().DeleteFileSafe(s.ctx, stsServiceManifest)
	s.NoError(err, "can delete sts service manifest")
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

		// Replace the endpointURL and STS service IP placeholders with the actual values
		newContent := strings.Replace(string(content), "http://172.18.0.2:31566", s.endpointURL, -1)
		newContent = strings.Replace(newContent, stsServiceIPPlaceholder, s.stsServiceIP, -1)
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

	s.ti.AssertionsT(s.T()).EventuallyObjectsExist(s.ctx, testdefaults.CurlPod)
	s.ti.AssertionsT(s.T()).EventuallyPodsRunning(s.ctx, testdefaults.CurlPod.GetNamespace(), metav1.ListOptions{
		LabelSelector: testdefaults.WellKnownAppLabel + "=curl",
	})

	s.ti.AssertionsT(s.T()).EventuallyObjectsExist(s.ctx, proxyServiceMeta)
	s.ti.AssertionsT(s.T()).EventuallyObjectsExist(s.ctx, proxyDeploymentMeta)
	s.ti.AssertionsT(s.T()).EventuallyPodsRunning(s.ctx, proxyDeploymentMeta.GetNamespace(), metav1.ListOptions{
		LabelSelector: fmt.Sprintf("%s=%s", testdefaults.WellKnownAppLabel, gatewayName),
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
	s.ti.AssertionsT(s.T()).AssertEventualCurlResponse(
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
	s.ti.AssertionsT(s.T()).AssertEventualCurlResponse(
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
	s.ti.AssertionsT(s.T()).AssertEventualCurlResponse(
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
	s.ti.AssertionsT(s.T()).AssertEventualCurlResponse(
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
	s.ti.AssertionsT(s.T()).AssertEventualCurlResponse(
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
	s.ti.AssertionsT(s.T()).AssertEventualCurlResponse(
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
	s.ti.AssertionsT(s.T()).AssertEventualCurlResponse(
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

// TestLambdaBackendAssumeRole verifies STS role chaining: the proxy must use its
// base credentials (env vars on the dedicated gateway's Envoy container) to call
// sts:AssumeRole on the Backend's roleArn, then sign the Lambda invocation with
// the temporary credentials STS returns.
func (s *testingSuite) TestLambdaBackendAssumeRole() {
	// BeforeTest waits on the shared gateway; this test brings its own.
	s.ti.AssertionsT(s.T()).EventuallyObjectsExist(s.ctx, assumeRoleProxyServiceMeta, assumeRoleProxyDeploymentMeta)
	s.ti.AssertionsT(s.T()).EventuallyPodsRunning(s.ctx, lambdaNamespace, metav1.ListOptions{
		LabelSelector: fmt.Sprintf("%s=%s", testdefaults.WellKnownAppLabel, assumeRoleGatewayName),
	})

	// The route must work end to end through the assumed role's credentials.
	s.ti.AssertionsT(s.T()).AssertEventualCurlResponse(
		s.ctx,
		testdefaults.CurlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(assumeRoleGatewayObjectMeta)),
			curl.WithHostHeader("www.example.com"),
			curl.WithPort(8080),
			curl.WithPath("/lambda"),
		},
		&testmatchers.HttpResponse{
			StatusCode: http.StatusOK,
			Body:       gomega.ContainSubstring(`Hello from Lambda`),
		},
	)

	// A 200 alone doesn't prove role chaining: localstack doesn't enforce IAM, so
	// a proxy that (incorrectly) signs with its base credentials also gets a 200.
	// Envoy only creates and uses the internal STS cluster when the assume-role
	// credential provider is actually active, so require a successful AssumeRole
	// call to have gone through it.
	s.ti.AssertionsT(s.T()).AssertEnvoyAdminApi(s.ctx, assumeRoleProxyDeploymentMeta.ObjectMeta,
		func(ctx context.Context, adminClient *admincli.Client) {
			stats, err := adminClient.GetStats(ctx, map[string]string{
				"filter": `cluster\.sts_token_service_internal-us-east-1\.upstream_rq_2xx`,
			})
			s.Assert().NoError(err, "can fetch envoy stats")
			matches := stsSuccessStatRegex.FindStringSubmatch(stats)
			s.Require().Len(matches, 2, "expected an upstream_rq_2xx stat for the STS cluster; "+
				"its absence means Envoy never called sts:AssumeRole and signed with its base credentials instead. stats: %q", stats)
			count, err := strconv.Atoi(matches[1])
			s.Assert().NoError(err, "can parse STS 2xx stat value")
			s.Assert().GreaterOrEqual(count, 1, "expected at least one successful sts:AssumeRole call")
		},
	)
}

func (s *testingSuite) extractLocalstackEndpoint() {
	s.T().Log("extracting localstack endpoint URL from cluster")

	endpoint, found, err := localstack.EndpointURL(s.ctx, s.ti.ClusterContext.Client)
	s.Require().NoError(err, "can resolve localstack endpoint")
	s.Require().True(found, "localstack service must exist")

	s.endpointURL = endpoint
	s.T().Logf("localstack endpoint URL: %s", s.endpointURL)
}

// extractSTSServiceIP resolves the cluster IP of the localstack-sts service so
// the assume-role test can alias sts.us-east-1.amazonaws.com (which Envoy's
// assume-role credential provider hardcodes) to localstack via pod hostAliases.
func (s *testingSuite) extractSTSServiceIP() {
	svc := &corev1.Service{}
	err := s.ti.ClusterContext.Client.Get(s.ctx,
		client.ObjectKey{Namespace: "localstack", Name: "localstack-sts"}, svc)
	s.Require().NoError(err, "can get localstack-sts service")
	s.Require().NotEmpty(svc.Spec.ClusterIP, "localstack-sts service must have a cluster IP")

	s.stsServiceIP = svc.Spec.ClusterIP
	s.T().Logf("localstack STS service cluster IP: %s", s.stsServiceIP)
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
