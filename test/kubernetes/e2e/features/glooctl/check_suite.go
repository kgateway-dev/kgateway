package glooctl

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/onsi/gomega"
	"github.com/solo-io/gloo/test/kubernetes/e2e"
	"github.com/stretchr/testify/suite"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ e2e.NewSuiteFunc = NewCheckSuite

// checkSuite contains the set of tests to validate the behavior of `glooctl check`
// These tests attempt to mirror: https://github.com/solo-io/gloo/blob/v1.16.x/test/kube2e/glooctl/check_test.go
type checkSuite struct {
	suite.Suite

	ctx              context.Context
	testInstallation *e2e.TestInstallation
}

// NewChecksuite for glooctl check validation
// TODO(nfuden): Fix clusterloadassignment issues that forced xds-metrics to be excluded.
// Consider https://github.com/envoyproxy/envoy/issues/7529#issuecomment-1227724217
func NewCheckSuite(ctx context.Context, testInst *e2e.TestInstallation) suite.TestingSuite {
	return &checkSuite{
		ctx:              ctx,
		testInstallation: testInst,
	}
}

func (s *checkSuite) TestCheck() {
	output, err := s.testInstallation.Actions.Glooctl().Check(s.ctx,
		"-n", s.testInstallation.Metadata.InstallNamespace, "-x", "xds-metrics")
	s.NoError(err)

	for _, expectedOutput := range checkCommonGlooGatewayOutputByKey {
		gomega.Expect(output).To(expectedOutput.include)
	}

	if s.testInstallation.Metadata.K8sGatewayEnabled {
		for _, expectedOutput := range checkK8sGatewayOutputByKey {
			gomega.Expect(output).To(expectedOutput.include)
		}
	}
}

func (s *checkSuite) TestCheckExclude() {
	for excludeKey, expectedOutput := range checkCommonGlooGatewayOutputByKey {
		output, err := s.testInstallation.Actions.Glooctl().Check(s.ctx,
			"-n", s.testInstallation.Metadata.InstallNamespace, "-x", fmt.Sprintf("xds-metrics,%s", excludeKey))
		s.NoError(err)
		gomega.Expect(output).To(expectedOutput.exclude)
	}

	if s.testInstallation.Metadata.K8sGatewayEnabled {
		for excludeKey, expectedOutput := range checkK8sGatewayOutputByKey {
			output, err := s.testInstallation.Actions.Glooctl().Check(s.ctx,
				"-n", s.testInstallation.Metadata.InstallNamespace, "-x", fmt.Sprintf("xds-metrics,%s", excludeKey))
			s.NoError(err)
			gomega.Expect(output).To(expectedOutput.exclude)
		}
	}
}

func (s *checkSuite) TestCheckReadOnly() {
	output, err := s.testInstallation.Actions.Glooctl().Check(s.ctx,
		"-n", s.testInstallation.Metadata.InstallNamespace, "--read-only", "-x", "xds-metrics")
	s.NoError(err)

	for _, expectedOutput := range checkCommonGlooGatewayOutputByKey {
		gomega.Expect(output).To(gomega.And(
			expectedOutput.include,
			expectedOutput.readOnly,
		))
	}

	if s.testInstallation.Metadata.K8sGatewayEnabled {
		for _, expectedOutput := range checkK8sGatewayOutputByKey {
			gomega.Expect(output).To(gomega.And(
				expectedOutput.include,
				expectedOutput.readOnly,
			))
		}
	}
}

func (s *checkSuite) TestCheckKubeContext() {
	// When passing an invalid kube-context, `glooctl check` should succeed
	_, err := s.testInstallation.Actions.Glooctl().Check(s.ctx,
		"-n", s.testInstallation.Metadata.InstallNamespace, "--kube-context", "invalid-context")
	s.Error(err)
	s.Contains(err.Error(), "Could not get kubernetes client: Error retrieving Kubernetes configuration: context \"invalid-context\" does not exist")

	// When passing the kube-context of the running cluster, `glooctl check` should succeed
	_, err = s.testInstallation.Actions.Glooctl().Check(s.ctx,
		"-n", s.testInstallation.Metadata.InstallNamespace, "--kube-context", s.testInstallation.ClusterContext.KubeContext, "-x", "xds-metrics")
	s.NoError(err)
}

func (s *checkSuite) TestCheckTimeout() {
	// When passing short timeout, check will fail
	shortTimeoutValues, err := os.CreateTemp("", "*.yaml")
	s.NoError(err)
	_, err = shortTimeoutValues.Write([]byte(`checkTimeoutSeconds: 1ns`))
	s.NoError(err)

	_, err = s.testInstallation.Actions.Glooctl().Check(s.ctx,
		"-n", s.testInstallation.Metadata.InstallNamespace,
		"-c", shortTimeoutValues.Name())
	s.Error(err)
	s.Contains(err.Error(), "context deadline exceeded")

	// When passing valid timeout, check should succeed
	normalTimeoutValues, err := os.CreateTemp("", "*.yaml")
	s.NoError(err)
	_, err = normalTimeoutValues.Write([]byte(`checkTimeoutSeconds: 300s`))
	s.NoError(err)

	_, err = s.testInstallation.Actions.Glooctl().Check(s.ctx,
		"-n", s.testInstallation.Metadata.InstallNamespace,
		"-c", normalTimeoutValues.Name())
	s.NoError(err)
}

func (s *checkSuite) TestCheckNamespace() {
	// namespace does not exist
	output, err := s.testInstallation.Actions.Glooctl().Check(s.ctx, "-n", "not-gloo-system")
	s.Error(err)
	s.Contains(output, "Could not communicate with kubernetes cluster: namespaces \"not-gloo-system\" not found")

	// gloo not in namespace
	output, err = s.testInstallation.Actions.Glooctl().Check(s.ctx, "-n", "default")
	s.Error(err)
	s.Contains(output, "Warning: The provided label selector (gloo) applies to no pods")

	// pod does not exist
	output, err = s.testInstallation.Actions.Glooctl().Check(s.ctx,
		"-n", s.testInstallation.Metadata.InstallNamespace,
		"-p", "not-gloo")
	s.NoError(err)
	s.Contains(output, "Warning: The provided label selector (not-gloo) applies to no pods")
	s.Contains(output, "No problems detected.")

	// resource namespace does not exist
	output, err = s.testInstallation.Actions.Glooctl().Check(s.ctx,
		"-n", s.testInstallation.Metadata.InstallNamespace,
		"-r", "not-gloo-system")
	s.Error(err)
	s.Contains(output, fmt.Sprintf("No namespaces specified are currently being watched (defaulting to '%s' namespace)", s.testInstallation.Metadata.InstallNamespace))
}

func (s *checkSuite) TestNoGateways() {
	s.T().Cleanup(func() {
		// Scale gateways back
		err := s.testInstallation.ClusterContext.Cli.Scale(s.ctx, s.testInstallation.Metadata.InstallNamespace, "deploy/gateway-proxy", 1)
		s.NoError(err)
		s.testInstallation.Assertions.EventuallyRunningReplicas(s.ctx, metav1.ObjectMeta{
			Name:      "gateway-proxy",
			Namespace: s.testInstallation.Metadata.InstallNamespace,
		}, gomega.Equal(1))

		err = s.testInstallation.ClusterContext.Cli.Scale(s.ctx, s.testInstallation.Metadata.InstallNamespace, "deploy/public-gw", 2)
		s.NoError(err)
		s.testInstallation.Assertions.EventuallyRunningReplicas(s.ctx, metav1.ObjectMeta{
			Name:      "public-gw",
			Namespace: s.testInstallation.Metadata.InstallNamespace,
		}, gomega.Equal(2)) // helm has replicas=2 defined
	})

	// Scale gateways down
	err := s.testInstallation.ClusterContext.Cli.Scale(s.ctx, s.testInstallation.Metadata.InstallNamespace, "deploy/gateway-proxy", 0)
	s.NoError(err)
	s.testInstallation.Assertions.EventuallyRunningReplicas(s.ctx, metav1.ObjectMeta{
		Name:      "gateway-proxy",
		Namespace: s.testInstallation.Metadata.InstallNamespace,
	}, gomega.Equal(0))

	// public-gw is defined in the helm chart
	err = s.testInstallation.ClusterContext.Cli.Scale(s.ctx, s.testInstallation.Metadata.InstallNamespace, "deploy/public-gw", 0)
	s.NoError(err)
	s.testInstallation.Assertions.EventuallyRunningReplicas(s.ctx, metav1.ObjectMeta{
		Name:      "public-gw",
		Namespace: s.testInstallation.Metadata.InstallNamespace,
	}, gomega.Equal(0))

	_, err = s.testInstallation.Actions.Glooctl().Check(s.ctx,
		"-n", s.testInstallation.Metadata.InstallNamespace, "--kube-context", s.testInstallation.ClusterContext.KubeContext, "-x", "xds-metrics")
	s.Error(err)
	s.Contains(err.Error(), "Gloo installation is incomplete: no active gateway-proxy pods exist in cluster")
}

func (s *checkSuite) TestEdgeGatewayScaled() {
	s.T().Cleanup(func() {
		// Scale gateway back
		err := s.testInstallation.ClusterContext.Cli.Scale(s.ctx, s.testInstallation.Metadata.InstallNamespace, "deploy/gateway-proxy", 1)
		s.NoError(err)
		s.testInstallation.Assertions.EventuallyRunningReplicas(s.ctx, metav1.ObjectMeta{
			Name:      "gateway-proxy",
			Namespace: s.testInstallation.Metadata.InstallNamespace,
		}, gomega.Equal(1))
	})

	// Scale gateway down
	err := s.testInstallation.ClusterContext.Cli.Scale(s.ctx, s.testInstallation.Metadata.InstallNamespace, "deploy/gateway-proxy", 0)
	s.NoError(err)

	// Wait for gateway to be gone
	s.testInstallation.Assertions.EventuallyRunningReplicas(s.ctx, metav1.ObjectMeta{
		Name:      "gateway-proxy",
		Namespace: s.testInstallation.Metadata.InstallNamespace,
	}, gomega.Equal(0))

	output, err := s.testInstallation.Actions.Glooctl().Check(s.ctx,
		"-n", s.testInstallation.Metadata.InstallNamespace, "--kube-context", s.testInstallation.ClusterContext.KubeContext, "-x", "xds-metrics")
	s.NoError(err)
	s.Contains(output, fmt.Sprintf("Warning: %s:gateway-proxy has zero replicas", s.testInstallation.Metadata.InstallNamespace))
	s.Contains(output, "No problems detected.")
	// Check healthy output
	for _, expectedOutput := range checkCommonGlooGatewayOutputByKey {
		gomega.Expect(output).To(expectedOutput.include)
	}

	if s.testInstallation.Metadata.K8sGatewayEnabled {
		for _, expectedOutput := range checkK8sGatewayOutputByKey {
			gomega.Expect(output).To(expectedOutput.include)
		}
	}
}

func (s *checkSuite) TestEdgeResourceError() {
	s.T().Cleanup(func() {
		// Delete invalid config
		err := s.testInstallation.ClusterContext.Cli.DeleteFileSafe(s.ctx, invalidVSKubeDest)
		s.NoError(err)
		err = s.testInstallation.ClusterContext.Cli.DeleteFileSafe(s.ctx, invalidVSUpstreamDest, "-n", s.testInstallation.Metadata.InstallNamespace)
		s.NoError(err)
	})

	// Apply invalid config
	err := s.testInstallation.ClusterContext.Cli.ApplyFile(s.ctx, invalidVSKubeDest)
	s.NoError(err)
	err = s.testInstallation.ClusterContext.Cli.ApplyFile(s.ctx, invalidVSUpstreamDest, "-n", s.testInstallation.Metadata.InstallNamespace)
	s.NoError(err)

	// Run check. This needs to run in eventually to get all errors reported
	gomega.Eventually(func() error {
		_, err = s.testInstallation.Actions.Glooctl().Check(s.ctx,
			"-n", s.testInstallation.Metadata.InstallNamespace, "--kube-context", s.testInstallation.ClusterContext.KubeContext)
		return err
	}, time.Minute*2, time.Second*10).Should(gomega.SatisfyAll(
		gomega.MatchError(gomega.ContainSubstring(fmt.Sprintf("* Found rejected virtual service by '%s': default reject-me-too (Reason: 2 errors occurred:", s.testInstallation.Metadata.InstallNamespace))),
		gomega.MatchError(gomega.ContainSubstring(fmt.Sprintf("* domain conflict: other virtual services that belong to the same Gateway as this one don't specify a domain (and thus default to '*'): [%s.reject-me]", s.testInstallation.Metadata.InstallNamespace))),
		gomega.MatchError(gomega.ContainSubstring(fmt.Sprintf("* VirtualHost Error: DomainsNotUniqueError. Reason: domain * is shared by the following virtual hosts: [default.reject-me-too %s.reject-me]", s.testInstallation.Metadata.InstallNamespace))),
		gomega.MatchError(gomega.ContainSubstring(fmt.Sprintf("* Found rejected virtual service by '%s': %s reject-me (Reason: 2 errors occurred:", s.testInstallation.Metadata.InstallNamespace, s.testInstallation.Metadata.InstallNamespace))),
		gomega.MatchError(gomega.ContainSubstring(fmt.Sprintf("* domain conflict: other virtual services that belong to the same Gateway as this one don't specify a domain (and thus default to '*'): [default.reject-me-too]"))),
		gomega.MatchError(gomega.ContainSubstring(fmt.Sprintf("* VirtualHost Error: DomainsNotUniqueError. Reason: domain * is shared by the following virtual hosts: [default.reject-me-too %s.reject-me]", s.testInstallation.Metadata.InstallNamespace))),
	))
}
