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
		"deployments":        "Checking deployments... OK",
		"pods":               "Checking pods... OK",
		"upstreams":          "Checking upstreams... OK",
		"upstreamgroup":      "Checking upstream groups... OK",
		"auth-configs":       "Checking auth configs... OK",
		"rate-limit-configs": "Checking rate limit configs... OK",
		"secrets":            "Checking secrets... OK",
		"virtual-services":   "Checking virtual services... OK",
		"gateways":           "Checking gateways... OK",
		"proxies":            "Checking proxies... OK",
	}
)

func (s *testingSuite) TestCheckOk() {
	output, err := s.testInstallation.Actions.Glooctl().Check(s.ctx, "-x", "xds-metrics")
	s.NoError(err)

	for _, expectedSubstring := range checkOutputSuccessfulMessages {
		s.Contains(output, expectedSubstring)
	}
}

func (s *testingSuite) TestCheckExcludeOk() {
	for excludeKey, keyOutput := range checkOutputSuccessfulMessages {
		checkOutput, err := s.testInstallation.Actions.Glooctl().Check(s.ctx, "-x", fmt.Sprintf("xds-metrics,%s", excludeKey))
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

func (s *testingSuite) TestDebugLogsNoPanic() {
	err := s.testInstallation.Actions.Glooctl().DebugLogs(s.ctx, "-n", s.testInstallation.Metadata.InstallNamespace)
	s.NoError(err)
}
