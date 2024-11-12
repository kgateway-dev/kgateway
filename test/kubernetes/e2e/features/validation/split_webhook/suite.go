package split_webhook

import (
	"context"
	"fmt"

	"github.com/onsi/gomega"
	gloo_defaults "github.com/solo-io/gloo/projects/gloo/pkg/defaults"
	"github.com/solo-io/gloo/test/kubernetes/e2e"
	"github.com/solo-io/gloo/test/kubernetes/e2e/features/validation"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"

	"github.com/stretchr/testify/suite"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ e2e.NewSuiteFunc = NewKubeFailTestingSuite
var _ e2e.NewSuiteFunc = NewGlooFailTestingSuite

// testingSuite is the entire Suite of tests for DO_NOT_SUBMIT
type testingSuite struct {
	suite.Suite

	ctx context.Context

	// testInstallation contains all the metadata/utilities necessary to execute a series of tests
	// against an installation of Gloo Gateway
	testInstallation *e2e.TestInstallation

	// glooFailurePolicyFail determines whether the gloo webhook failure policy is set to Fail
	glooFailurePolicyFail bool
	// kubeFailurePolicyFail determines whether the kube webhook failure policy is set to Fail
	kubeFailurePolicyFail bool
}

func NewKubeFailTestingSuite(ctx context.Context, testInst *e2e.TestInstallation) suite.TestingSuite {
	return &testingSuite{
		ctx:                   ctx,
		testInstallation:      testInst,
		glooFailurePolicyFail: false,
		kubeFailurePolicyFail: true,
	}
}

func NewGlooFailTestingSuite(ctx context.Context, testInst *e2e.TestInstallation) suite.TestingSuite {
	return &testingSuite{
		ctx:                   ctx,
		testInstallation:      testInst,
		glooFailurePolicyFail: true,
		kubeFailurePolicyFail: false,
	}
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

func (s *testingSuite) setup() {
	glooReplicas := 1
	s.T().Cleanup(func() {
		// Scale the gloo deployment back up
		err := s.testInstallation.Actions.Kubectl().Scale(s.ctx, s.testInstallation.Metadata.InstallNamespace, "deployment/gloo", uint(glooReplicas))
		s.Assert().NoError(err, "can scale gloo deployment back to %d", glooReplicas)
		s.testInstallation.Assertions.EventuallyRunningReplicas(s.ctx, s.glooDeployment().ObjectMeta, gomega.Equal(glooReplicas))

		// The upstream should have been deleted either directly or via the namespace
		err = s.testInstallation.Actions.Kubectl().DeleteFile(s.ctx, validation.BasicUpstream, "-n", s.testInstallation.Metadata.InstallNamespace)
		if s.glooFailurePolicyFail {
			// If the gloo failure policy is "Fail", the upstream should be deleted
			s.Assert().NoError(err, "can delete "+validation.BasicUpstream)

		} else {
			// If the gloo failure policy is "Ignore", the upstream was already deleted
			s.Assert().Error(err, "cannot delete "+validation.BasicUpstream+" - expected failure as it was already deleted")
		}

		err = s.testInstallation.Actions.Kubectl().DeleteFile(s.ctx, validation.Secret, "-n", s.testInstallation.Metadata.InstallNamespace)
		if s.kubeFailurePolicyFail {
			// If the kube failure policy is "Fail", the secret should be deleted
			s.Assert().NoError(err, "can delete "+validation.Secret)
		} else {
			// If the kube failure policy is "Ignore", the secret was already deleted
			s.Assert().Error(err, "cannot delete "+validation.Secret+" - expected failure as it was already deleted")
		}

	})

	// Upstream should be accepted
	err := s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, validation.BasicUpstream, "-n", s.testInstallation.Metadata.InstallNamespace)
	s.Assert().NoError(err)

	s.testInstallation.Assertions.EventuallyResourceStatusMatchesState(
		func() (resources.InputResource, error) {
			return s.testInstallation.ResourceClients.UpstreamClient().Read(s.testInstallation.Metadata.InstallNamespace, "json-upstream", clients.ReadOpts{Ctx: s.ctx})
		},
		core.Status_Accepted,
		gloo_defaults.GlooReporter,
	)

	// Create secret
	err = s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, validation.Secret, "-n", s.testInstallation.Metadata.InstallNamespace)
	s.Assert().NoError(err)

	err = s.testInstallation.Actions.Kubectl().Scale(s.ctx, s.testInstallation.Metadata.InstallNamespace, "deployment/gloo", 0)
	s.Assert().NoError(err, "can scale gloo deployment back to %d", glooReplicas)
	s.testInstallation.Assertions.EventuallyRunningReplicas(s.ctx, s.glooDeployment().ObjectMeta, gomega.Equal(0))

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

// TestSplitWebhook tests the split webhook functionality
// The test will apply a basic upstream and a secret, then attempt to delete the upstream and secret
func (s *testingSuite) TestSplitWebhook() {
	s.setup()

	s.validateCaBundles()

	// Validate that the clientConfig.caBundle is set
	stdout, _, err := s.testInstallation.Actions.Kubectl().Execute(
		s.ctx, "get",
		"ValidatingWebhookConfiguration", fmt.Sprintf("gloo-gateway-validation-webhook-%s", s.testInstallation.Metadata.InstallNamespace),
		"-n", s.testInstallation.Metadata.InstallNamespace,
		"-o", "jsonpath={.webhooks[1].clientConfig.caBundle}",
	)

	s.Assert().NoError(err)
	// The value is set as "" in the template, so if it is not empty we know it was set
	s.Assert().NotEmpty(stdout)

	output, err := s.testInstallation.Actions.Kubectl().DeleteFileWithOutput(s.ctx, validation.BasicUpstream, "-n", s.testInstallation.Metadata.InstallNamespace)
	if s.glooFailurePolicyFail {
		// If the gloo failure policy is "Fail", the upstream should not be deleted
		s.Assert().Error(err)
		s.Assert().Contains(output, "Internal error occurred: failed calling webhook")
	} else {
		// If the gloo failure policy is "Ignore", the upstream should be deleted
		s.Assert().NoError(err)
	}

	output, err = s.testInstallation.Actions.Kubectl().DeleteFileWithOutput(s.ctx, validation.Secret, "-n", s.testInstallation.Metadata.InstallNamespace)
	if s.kubeFailurePolicyFail {
		// If the kube failure policy is "Fail", the secret should not be deleted
		s.Assert().Error(err)
		s.Assert().Contains(output, "Internal error occurred: failed calling webhook")
	} else {
		// If the gloo failure policy is "Ignore", the secret should be deleted
		s.Assert().NoError(err)
	}

}
