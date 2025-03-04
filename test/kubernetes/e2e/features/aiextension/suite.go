package aiextension

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/rotisserie/eris"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/kgateway-dev/kgateway/v2/test/testutils"

	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e"
)

var pythonBin = func() string {
	v, ok := os.LookupEnv("PYTHON")
	if !ok {
		return "python3"
	}
	return v
}()

type tsuite struct {
	suite.Suite

	ctx context.Context

	testInst *e2e.TestInstallation

	rootDir string

	installNamespace string

	manifests map[string][]string
}

func NewSuite(
	ctx context.Context,
	testInst *e2e.TestInstallation,
) suite.TestingSuite {
	return &tsuite{
		ctx:              ctx,
		testInst:         testInst,
		rootDir:          testutils.GitRootDirectory(),
		installNamespace: os.Getenv(testutils.InstallNamespace),
	}
}

func (s *tsuite) SetupSuite() {
	s.manifests = map[string][]string{
		"TestRouting":            {commonManifest, backendManifest, routesBasicManifest},
		"TestRoutingPassthrough": {commonManifest, backendPassthroughManifest, routesWithExtensionManifest},
		"TestStreaming":          {commonManifest, backendManifest, routesWithExtensionManifest, routeOptionStreamingManifest},
		// "TestPromptGuardWebhook": {commonManifest, backendManifest, routesWithExtensionManifest, promptGuardWebhookManifest},
		//"TestPromptGuardWebhookStreaming":         {commonManifest, backendManifest, routesWithExtensionManifest, promptGuardWebhookStreamingManifest},
		"TestPromptGuard": {commonManifest, backendManifest, routesWithExtensionManifest, promptGuardManifest},
		// "TestPromptGuardStreaming":                {commonManifest, backendManifest, routesWithExtensionManifest, promptGuardStreamingManifest},
		//"TestUserInvokedFunctionCalling":          {commonManifest, backendManifest, routesBasicManifest},
		//"TestUserInvokedFunctionCallingStreaming": {commonManifest, backendManifest, routesWithExtensionManifest, routeOptionStreamingManifest},
		//"TestLangchain":                           {commonManifest, backendManifest, routesBasicManifest},
	}
}

func (s *tsuite) TearDownSuite() {
}

func (s *tsuite) waitForEnvoyReady() {
	gwURL := s.getGatewayURL()
	fmt.Printf("Waiting for envoy up.")
	s.Require().EventuallyWithT(func(c *assert.CollectT) {
		statusChar := "."
		resp, err := http.Get(gwURL + "/not_there")
		if assert.NoErrorf(c, err, "failed to wait for envoy up") {
			defer resp.Body.Close()
			statusChar = "*"
			assert.Equalf(c, resp.StatusCode, 404, "envoy up check failed")
		}
		fmt.Printf(statusChar)
	}, 30*time.Second, 1*time.Second)
	fmt.Printf("\n")
}

func (s *tsuite) BeforeTest(suiteName, testName string) {
	manifests := s.manifests[testName]
	fmt.Printf("Applying manifests for test %s in suite %s", testName, suiteName)
	for _, manifest := range manifests {
		err := s.testInst.Actions.Kubectl().ApplyFile(s.ctx, manifest)
		s.Require().NoError(err)
	}

	s.waitForEnvoyReady()
}

func (s *tsuite) AfterTest(suiteName, testName string) {
	if s.T().Failed() {
		s.testInst.PreFailHandler(s.ctx)
	}
	manifests := s.manifests[testName]
	for _, manifest := range manifests {
		err := s.testInst.Actions.Kubectl().DeleteFileSafe(s.ctx, manifest)
		s.Require().NoError(err)
	}
}

func (s *tsuite) TestRouting() {
	s.invokePytest("routing.py")
}

func (s *tsuite) TestRoutingPassthrough() {
	vertexAIToken, err := GetVertexAIToken()
	if err != nil {
		s.T().Fatal(err)
	}
	s.invokePytest(
		"routing.py",
		"TEST_TOKEN_PASSTHROUGH=true",
		fmt.Sprintf("OPENAI_API_KEY=%s", os.Getenv("OPENAI_API_KEY")),
		fmt.Sprintf("AZURE_OPENAI_API_KEY=%s", os.Getenv("AZURE_OPENAI_API_KEY")),
		fmt.Sprintf("GEMINI_API_KEY=%s", os.Getenv("GEMINI_API_KEY")),
		fmt.Sprintf("VERTEX_AI_API_KEY=%s", vertexAIToken),
	)
}

func (s *tsuite) TestStreaming() {
	s.invokePytest("streaming.py")
}

func (s *tsuite) TestPromptGuard() {
	s.invokePytest("prompt_guard.py")
}

//func (s *tsuite) TestPromptGuardStreaming() {
//	s.invokePytest("prompt_guard_streaming.py")
//}
//
//func (s *tsuite) TestRateLimit() {
//	s.invokePytest("rate_limit.py")
//
//	// Restart rate-limiter Redis so that the next test can start with a clean state
//	err := s.testInst.Actions.Kubectl().DeploymentRolloutStatus(s.ctx, "redis", "-n", s.installNamespace)
//	s.Require().NoError(err)
//}
//
//func (s *tsuite) TestPromptGuardWebhook() {
//	spacy_model := "en_core_web_lg"
//	spacyInfo, err := exec.Command(pythonBin, "-m", "spacy", "info").CombinedOutput()
//	if !strings.Contains(string(spacyInfo), spacy_model) {
//		byt, err := exec.Command(pythonBin, "-m", "spacy", "download", spacy_model).CombinedOutput()
//		s.Require().NoError(err)
//		s.T().Logf("spacy download output: %s", string(byt))
//	}
//
//	cmd := exec.Command(pythonBin, "-m", "fastapi", "run", "--host", "0.0.0.0", "--port", "7891", "samples/app.py")
//	cmd.Dir = filepath.Join(s.rootDir, "projects/ai-extension")
//
//	var b bytes.Buffer
//	cmd.Stdout = &b
//	cmd.Stderr = &b
//	err = cmd.Start()
//	s.Require().NoError(err)
//	defer func() {
//		cmd.Process.Kill()
//		err := cmd.Wait()
//		if err != nil && err.Error() != "signal: killed" {
//			s.T().Logf("error: %s", err)
//		}
//		s.T().Logf("combined_output: %s", b.String())
//		http.Get("http://localhost:7891/shutdown")
//	}()
//
//	s.Require().EventuallyWithT(func(c *assert.CollectT) {
//		resp, err := http.Get("http://localhost:7891/health")
//		if assert.NoErrorf(c, err, "failed to get health check") {
//			defer resp.Body.Close()
//			assert.Equalf(c, resp.StatusCode, 200, "health check failed")
//		}
//	}, 20*time.Second, 1*time.Second)
//
//	s.invokePytest("prompt_guard_webhook.py")
//}
//
//func (s *tsuite) TestPromptGuardWebhookStreaming() {
//	spacy_model := "en_core_web_lg"
//	spacyInfo, err := exec.Command(pythonBin, "-m", "spacy", "info").CombinedOutput()
//	if !strings.Contains(string(spacyInfo), spacy_model) {
//		byt, err := exec.Command(pythonBin, "-m", "spacy", "download", spacy_model).CombinedOutput()
//		s.Require().NoError(err)
//		s.T().Logf("spacy download output: %s", string(byt))
//	}
//
//	cmd := exec.Command(pythonBin, "-m", "fastapi", "run", "--host", "0.0.0.0", "--port", "7891", "samples/app.py")
//	cmd.Dir = filepath.Join(s.rootDir, "projects/ai-extension")
//
//	var b bytes.Buffer
//	cmd.Stdout = &b
//	cmd.Stderr = &b
//	err = cmd.Start()
//	s.Require().NoError(err)
//	defer func() {
//		cmd.Process.Kill()
//		err := cmd.Wait()
//		if err != nil && err.Error() != "signal: killed" {
//			s.T().Logf("error: %s", err)
//		}
//		s.T().Logf("combined_output: %s", b.String())
//		http.Get("http://localhost:7891/shutdown")
//	}()
//
//	s.Require().EventuallyWithT(func(c *assert.CollectT) {
//		resp, err := http.Get("http://localhost:7891/health")
//		if assert.NoErrorf(c, err, "failed to get health check") {
// 			defer resp.Body.Close()
//			assert.Equalf(c, resp.StatusCode, 200, "health check failed")
//		}
//	}, 20*time.Second, 1*time.Second)
//
//	s.invokePytest("prompt_guard_webhook_streaming.py")
//}
//
//func (s *tsuite) TestUserInvokedFunctionCalling() {
//	s.invokePytest("user_function_calling.py")
//}
//
//func (s *tsuite) TestUserInvokedFunctionCallingStreaming() {
//	s.invokePytest("user_function_calling_stream.py")
//}
//
//func (s *tsuite) TestLangchain() {
//	s.invokePytest("langchain_function_calling.py")
//}

func (s *tsuite) invokePytest(test string, extraEnv ...string) {
	fmt.Printf("Using Python binary: %s\n", pythonBin)

	gwURL := s.getGatewayURL()
	logLevel := os.Getenv("TEST_PYTHON_LOG_LEVEL")
	if logLevel == "" {
		logLevel = "INFO"
	}

	args := []string{"-m", "pytest", test, "-vvv", "--log-cli-level=" + logLevel}
	if pyMatch := os.Getenv("TEST_PYTHON_STRING_MATCH"); pyMatch != "" {
		args = append(args, "-k="+pyMatch)
	}

	cmd := exec.Command(pythonBin, args...)
	cmd.Dir = filepath.Join(s.rootDir, "test/kubernetes/e2e/features/aiextension/tests")
	cmd.Env = []string{
		fmt.Sprintf("TEST_OPENAI_BASE_URL=%s/openai", gwURL),
		fmt.Sprintf("TEST_AZURE_OPENAI_BASE_URL=%s/azure", gwURL),
		fmt.Sprintf("TEST_MISTRAL_BASE_URL=%s/mistralai", gwURL),
		fmt.Sprintf("TEST_ANTHROPIC_BASE_URL=%s/anthropic", gwURL),
		fmt.Sprintf("TEST_GEMINI_BASE_URL=%s/gemini", gwURL), // need to specify HTTP as part of the endpoint
		fmt.Sprintf("TEST_VERTEX_AI_BASE_URL=%s/vertex-ai", gwURL),
		fmt.Sprintf("TEST_GATEWAY_ADDRESS=%s", gwURL),
	}
	cmd.Env = append(cmd.Env, extraEnv...)

	fmt.Printf("Running Test Command: %s\n", cmd.String())
	// TODO: remove
	fmt.Printf("Using Environment Values: %v\n", cmd.Env)

	out, err := cmd.CombinedOutput()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			// Check the exit code
			if exitErr.ExitCode() == 5 {
				// When all tests are filtered (by TEST_PYTHON_STRING_MATCH), pytest returns exit code 5
				// ignore it
			} else {
				s.Require().NoError(err, string(out))
			}
		}
	}
	s.T().Logf("Test output: %s", string(out))
}

func (s *tsuite) getGatewayURL() string {
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ai-gateway",
			Namespace: s.testInst.Metadata.InstallNamespace,
		},
	}
	s.testInst.Assertions.EventuallyObjectsExist(s.ctx, svc)

	s.Require().Greater(len(svc.Spec.Ports), 0)

	s.Require().EventuallyWithT(func(c *assert.CollectT) {
		err := s.testInst.ClusterContext.Client.Get(
			s.ctx,
			types.NamespacedName{Name: svc.Name, Namespace: svc.Namespace},
			svc,
		)
		assert.NoErrorf(c, err, "failed to get service %s/%s", svc.Namespace, svc.Name)
		assert.Greaterf(c, len(svc.Status.LoadBalancer.Ingress), 0, "LB IP not found on service %s/%s", svc.Namespace, svc.Name)
	}, 10*time.Second, 1*time.Second)

	return fmt.Sprintf("http://%s:%d", svc.Status.LoadBalancer.Ingress[0].IP, svc.Spec.Ports[0].Port)
}

func GetVertexAIToken() (string, error) {
	cmd := exec.Command("gcloud", "auth", "print-access-token",
		"ci-cloud-run@gloo-ee.iam.gserviceaccount.com", "--project", "gloo-ee")
	vertexAIToken, err := cmd.Output()
	if err != nil {
		return "", eris.Wrap(err, "Failed to get access token")
	}
	return string(bytes.TrimSpace(vertexAIToken)), nil
}
