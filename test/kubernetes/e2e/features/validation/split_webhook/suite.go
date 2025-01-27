package split_webhook

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/onsi/gomega"
	"github.com/solo-io/gloo/test/kubernetes/e2e"
	"github.com/solo-io/gloo/test/kubernetes/e2e/features/validation"
	"github.com/solo-io/gloo/test/kubernetes/testutils/helper"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/stretchr/testify/suite"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ e2e.NewSuiteFunc = NewTestingSuite

type testingSuite struct {
	suite.Suite

	ctx context.Context

	// testInstallation contains all the metadata/utilities necessary to execute a series of tests
	// against an installation of Gloo Gateway
	testInstallation *e2e.TestInstallation

	testHelper      *helper.SoloTestHelper
	glooReplicas    int
	manifestObjects map[string][]client.Object
	rollback        func() error
	resourceDeleted bool
}

// This suite is mean to be run in an environment where validation is enabled
func NewTestingSuite(ctx context.Context, testInst *e2e.TestInstallation) suite.TestingSuite {
	return &testingSuite{
		ctx:              ctx,
		testInstallation: testInst,
		testHelper:       e2e.MustTestHelper(ctx, testInst),
	}
}

type webhookFailurePolicyTest struct {
	// glooFailurePolicyFail determines whether the gloo webhook failure policy is set to Fail
	glooFailurePolicyFail bool
	// kubeFailurePolicyFail determines whether the kube webhook failure policy is set to Fail
	kubeFailurePolicyFail bool
}

func (s *testingSuite) TearDownSuite() {
	// nothing at the moment
}
func (s *testingSuite) SetupSuite() {
	// nothing at the moment
}

func (s *testingSuite) BeforeTest(suiteName, testName string) {
	// Apply the upgrade values file
	var err error
	s.rollback, err = s.testHelper.UpgradeGloo(s.ctx, 600*time.Second, helper.WithExtraArgs([]string{
		// Reuse values so there's no need to know the prior values used
		"--reuse-values",
		"--values", upgradeValues[testName],
	}...))
	s.testInstallation.Assertions.Require.NoError(err)

	// Create resource we will be trying to delete
	manifest := manifests[testName]
	err = s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, manifest.filename, "-n", s.testInstallation.Metadata.InstallNamespace)
	s.Assert().NoError(err, "can apply %s", manifest.filename)

	s.Assert().NotNil(manifest.validateCreated, "validateCreated function must be set for %s", testName)
	manifest.validateCreated(s)

	// Get current replica count of gloo deployment
	stdout, _, err := s.testInstallation.Actions.Kubectl().Execute(s.ctx, "get", "deployment", "gloo", "-n", s.testInstallation.Metadata.InstallNamespace, "-o=jsonpath='{.status.replicas}'")
	s.Assert().NoError(err)

	if stdout == "" {
		s.glooReplicas = 0
	} else {
		s.glooReplicas, err = strconv.Atoi(strings.Trim(stdout, "'"))
		s.Assert().NoError(err)
	}
	// Scale gloo deployment to 0
	err = s.testInstallation.Actions.Kubectl().Scale(s.ctx, s.testInstallation.Metadata.InstallNamespace, "deployment/gloo", 0)
	s.Assert().NoError(err, "can scale gloo deployment to 0")
	s.testInstallation.Assertions.EventuallyRunningReplicas(s.ctx, s.glooDeployment().ObjectMeta, gomega.Equal(0))

	s.validateCaBundles()
	s.resourceDeleted = false
}

func (s *testingSuite) AfterTest(suiteName, testName string) {
	// Scale gloo deployment back to original replica count
	err := s.testInstallation.Actions.Kubectl().Scale(s.ctx, s.testInstallation.Metadata.InstallNamespace, "deployment/gloo", uint(s.glooReplicas))
	s.Assert().NoError(err, "can scale gloo deployment back to %d", s.glooReplicas)
	s.testInstallation.Assertions.EventuallyRunningReplicas(s.ctx, s.glooDeployment().ObjectMeta, gomega.Equal(s.glooReplicas))
	s.testInstallation.Assertions.EventuallyPodsRunning(s.ctx, s.testInstallation.Metadata.InstallNamespace, metav1.ListOptions{LabelSelector: "gloo=gloo"}, time.Minute*2)

	// Delete the resource created if it hasn't been already
	if !s.resourceDeleted {
		manifest := manifests[testName]
		output, err := s.testInstallation.Actions.Kubectl().DeleteFileWithOutput(s.ctx, manifest.filename, "-n", s.testInstallation.Metadata.InstallNamespace)
		s.testInstallation.Assertions.ExpectObjectDeleted(manifest.filename, err, output)
	}

	// Rollback the helm upgrades
	err = s.rollback()
	s.testInstallation.Assertions.Require.NoError(err)
}

func (s *testingSuite) glooDeployment() *appsv1.Deployment {
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: s.testInstallation.Metadata.InstallNamespace,
			Name:      "gloo",
			Labels:    map[string]string{"gloo": "gloo"},
		},
	}
}

// Test the caBundle is set for both webhooks
func (s *testingSuite) validateCaBundles() {

	for i := 0; i < 2; i++ {
		stdout, _, err := s.testInstallation.Actions.Kubectl().Execute(
			s.ctx, "get",
			"ValidatingWebhookConfiguration", fmt.Sprintf("gloo-gateway-validation-webhook-%s", s.testInstallation.Metadata.InstallNamespace),
			"-n", s.testInstallation.Metadata.InstallNamespace,
			"-o", fmt.Sprintf("jsonpath={.webhooks[%d].clientConfig.caBundle}", i),
		)

		s.Assert().NoError(err)
		// The value is set as "" in the template, so if it is not empty we know it was set
		s.Assert().NotEmpty(stdout)
	}

}

func (s *testingSuite) TestGlooFailurePolicyFail() {
	s.testDeleteResource(validation.BasicUpstream, false)
}

func (s *testingSuite) TestKubeFailurePolicyFail() {
	s.testDeleteResource(validation.Secret, false)
}

func (s *testingSuite) TestGlooFailurePolicyIgnore() {
	s.testDeleteResource(validation.BasicUpstream, true)
}

func (s *testingSuite) TestKubeFailurePolicyIgnore() {
	s.testDeleteResource(validation.Secret, true)
}

func (s *testingSuite) testDeleteResource(fileName string, shouldDelete bool) {
	output, err := s.testInstallation.Actions.Kubectl().DeleteFileWithOutput(s.ctx, fileName, "-n", s.testInstallation.Metadata.InstallNamespace)

	if shouldDelete {
		s.Assert().NoError(err)
		s.testInstallation.Assertions.ExpectObjectDeleted(fileName, err, output)
		s.resourceDeleted = true
	} else {
		s.Assert().Error(err)
		s.Assert().Contains(output, "Internal error occurred: failed calling webhook")
	}

}

var upgradeValues = map[string]string{
	"TestGlooFailurePolicyFail":   validation.GlooFailurePolicyFailValues,
	"TestKubeFailurePolicyFail":   validation.KubeFailurePolicyFailValues,
	"TestGlooFailurePolicyIgnore": validation.GlooFailurePolicyIgnoreValues,
	"TestKubeFailurePolicyIgnore": validation.KubeFailurePolicyIgnoreValues,
}

// These tests create one resource and try to delete it, so don't need lists of resources
type testManifest struct {
	filename        string
	validateCreated func(*testingSuite)
}

var (
	validateSecretCreated = func(s *testingSuite) {
		// No error is enough to validate the secret was created
	}

	validateUpstreamCreated = func(s *testingSuite) {
		// Upstreams no longer report status if they have not been translated at all to avoid conflicting with
		// other syncers that have translated them, so we can only detect that the objects exist here
		s.testInstallation.Assertions.EventuallyResourceExists(
			func() (resources.Resource, error) {
				uc := s.testInstallation.ResourceClients.UpstreamClient()
				return uc.Read(s.testInstallation.Metadata.InstallNamespace, validation.SplitWebhookBasicUpstreamName, clients.ReadOpts{Ctx: s.ctx})
			},
		)
		// we need to make sure Gloo has had a chance to process it
		s.testInstallation.Assertions.ConsistentlyResourceExists(
			s.ctx,
			func() (resources.Resource, error) {
				uc := s.testInstallation.ResourceClients.UpstreamClient()
				return uc.Read(s.testInstallation.Metadata.InstallNamespace, validation.SplitWebhookBasicUpstreamName, clients.ReadOpts{Ctx: s.ctx})
			},
		)
	}

	secretManifest = &testManifest{
		filename:        validation.Secret,
		validateCreated: validateSecretCreated,
	}

	upstreamManifest = &testManifest{
		filename:        validation.BasicUpstream,
		validateCreated: validateUpstreamCreated,
	}

	manifests = map[string]*testManifest{
		"TestGlooFailurePolicyFail":   upstreamManifest,
		"TestGlooFailurePolicyIgnore": upstreamManifest,
		"TestKubeFailurePolicyFail":   secretManifest,
		"TestKubeFailurePolicyIgnore": secretManifest,
	}
)
