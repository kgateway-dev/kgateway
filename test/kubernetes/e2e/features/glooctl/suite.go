package glooctl

import (
	"context"
	"fmt"
	"os"

	"github.com/solo-io/gloo/test/kubernetes/e2e"
	"github.com/stretchr/testify/suite"
)

type testingSuite struct {
	suite.Suite

	tmpDir string

	ctx              context.Context
	testInstallation *e2e.TestInstallation
}

func NewTestingSuite(ctx context.Context, testInst *e2e.TestInstallation) suite.TestingSuite {
	return &testingSuite{
		ctx:              ctx,
		testInstallation: testInst,
	}
}

func (s *testingSuite) SetupSuite() {
	var err error
	s.tmpDir, err = os.MkdirTemp("", "glooctl-test")
	s.NoError(err)
}

func (s *testingSuite) TearDownSuite() {
	_ = os.RemoveAll(s.tmpDir)
}

func (s *testingSuite) TestCheckCRDsErrorsForMismatch() {
	err := s.testInstallation.Actions.Glooctl().CheckCrds(s.ctx, "--version", "1.9.0")
	s.Error(err, "crds should be out of date")
	s.Contains(err.Error(), "One or more CRDs are out of date")
}

var (
	// checkOutputSuccessfulMessages is a map of messages that `glooctl check` will emit
	// The key is the values of the `-x` flag that will exclude a given sub-check
	checkOutputSuccessfulMessages = map[string]string{
		"deployments":          "Checking deployments... OK",
		"pods":                 "Checking pods... OK",
		"upstreams":            "Checking upstreams... OK",
		"upstreamgroup":        "Checking upstream groups... OK",
		"auth-configs":         "Checking auth configs... OK",
		"rate-limit-configs":   "Checking rate limit configs... OK",
		"virtual-host-options": "Checking VirtualHostOptions... OK",
		"route-options":        "Checking RouteOptions... OK",
		"secrets":              "Checking secrets... OK",
		"virtual-services":     "Checking virtual services... OK",
		"gateways":             "Checking gateways... OK",
		"proxies":              "Checking proxies... OK",
	}

	// readOnlyWarningMessages is a map of messages that `glooctl check` will emit when --read-only mode is set
	readOnlyWarningMessages = map[string]string{
		"proxies": "Warning: checking proxies with port forwarding is disabled",
		"xds":     "Warning: checking xds with port forwarding is disabled",
	}
)

func (s *testingSuite) TestCheck() {
	output, err := s.testInstallation.Actions.Glooctl().Check(s.ctx,
		"-n", s.testInstallation.Metadata.InstallNamespace, "-x", "xds-metrics")
	s.NoError(err)

	for _, expectedSubstring := range checkOutputSuccessfulMessages {
		s.Contains(output, expectedSubstring)
	}
}

func (s *testingSuite) TestCheckExclude() {
	for excludeKey, keyOutput := range checkOutputSuccessfulMessages {
		checkOutput, err := s.testInstallation.Actions.Glooctl().Check(s.ctx,
			"-n", s.testInstallation.Metadata.InstallNamespace, "-x", fmt.Sprintf("xds-metrics,%s", excludeKey))
		s.NoError(err)

		for key := range checkOutputSuccessfulMessages {
			if key == excludeKey {
				// If we have excluded a key, it should not be present in the output
				s.NotContains(checkOutput, keyOutput)
			} else {
				// If we have not excluded a key, it _should_ be present in the output
				s.Contains(checkOutput, keyOutput)
			}
		}
	}
}

func (s *testingSuite) TestCheckReadOnly() {
	output, err := s.testInstallation.Actions.Glooctl().Check(s.ctx,
		"-n", s.testInstallation.Metadata.InstallNamespace, "--read-only")
	s.NoError(err)

	for _, expectedSubstring := range checkOutputSuccessfulMessages {
		s.Contains(output, expectedSubstring)
	}
	for _, expectedWarning := range readOnlyWarningMessages {
		s.Contains(output, expectedWarning)
	}
}

func (s *testingSuite) TestCheckKubeContext() {
	// When passing an invalid kube-context, `glooctl check` should succeed
	_, err := s.testInstallation.Actions.Glooctl().Check(s.ctx,
		"-n", s.testInstallation.Metadata.InstallNamespace, "--kube-context", "invalid-context")
	s.Error(err)
	s.Contains(err.Error(), "Could not get kubernetes client: Error retrieving Kubernetes configuration: context \"invalid-context\" does not exist")

	// When passing the kube-context of the running cluster, `glooctl check` should succeed
	_, err = s.testInstallation.Actions.Glooctl().Check(s.ctx, "--kube-context", s.testInstallation.TestCluster.ClusterContext.KubeContext)
	s.NoError(err)
}

func (s *testingSuite) TestDebugLogsNoPanic() {
	err := s.testInstallation.Actions.Glooctl().DebugLogs(s.ctx,
		"-n", s.testInstallation.Metadata.InstallNamespace)
	s.NoError(err)
}
