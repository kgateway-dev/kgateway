package aiextension

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e"
	"github.com/kgateway-dev/kgateway/v2/test/testutils"
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
	}
}

func (s *tsuite) TearDownSuite() {
}

func (s *tsuite) BeforeTest(suiteName, testName string) {
	manifests := s.manifests[testName]
	fmt.Printf("Applying manifests for test %s in suite %s", testName, suiteName)
	for _, manifest := range manifests {
		err := s.testInst.Actions.Kubectl().ApplyFile(s.ctx, manifest)
		s.Require().NoError(err)
	}
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
	s.invokePytest(
		"routing.py",
		"TEST_TOKEN_PASSTHROUGH=true",
	)
}

func (s *tsuite) TestStreaming() {
	s.invokePytest("streaming.py")
}

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
		fmt.Sprintf("TEST_GEMINI_BASE_URL=%s/gemini", gwURL), // need to specify HTTP as part of the endpoint
		fmt.Sprintf("TEST_VERTEX_AI_BASE_URL=%s/vertex-ai", gwURL),
		fmt.Sprintf("TEST_GATEWAY_ADDRESS=%s", gwURL),
	}
	cmd.Env = append(cmd.Env, extraEnv...)

	fmt.Printf("Running Test Command: %s\n", cmd.String())

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
