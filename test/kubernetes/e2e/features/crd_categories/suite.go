//go:build ignore

package crd_categories

import (
	"bytes"
	"context"
	"io"
	"strings"

	"github.com/stretchr/testify/suite"

	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e"
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e/features/helm"
)

var _ e2e.NewSuiteFunc = NewTestingSuite

type testingSuite struct {
	suite.Suite
	ctx context.Context
	ti  *e2e.TestInstallation
}

func NewTestingSuite(
	ctx context.Context,
	testInst *e2e.TestInstallation,
) suite.TestingSuite {
	return &testingSuite{
		ctx: ctx,
		ti:  testInst,
	}
}

func (s *testingSuite) SetupSuite() {
	err := s.ti.Actions.Kubectl().ApplyFile(s.ctx, emptyVsManifest)
	s.NoError(err, "can apply manifest "+emptyVsManifest)
}

func (s *testingSuite) TearDownSuite() {
	output, err := s.ti.Actions.Kubectl().DeleteFileWithOutput(s.ctx, emptyVsManifest)
	s.ti.Assertions.ExpectObjectDeleted(emptyVsManifest, err, output)
}

// See TestApplyCRDs() helm test for a future-proofed common category test,
// which ensures all CRDs in our helm chart include the gloo-gateway category.
// This test uses one of those CRs to assert that the resulting end user experience is as desired.
func (s *testingSuite) TestCommonCategory() {
	cmd := s.ti.Actions.Kubectl().Command(s.ctx, "get", helm.CommonCRDCategory, "-o", "name")

	var out bytes.Buffer
	err := cmd.WithStdout(io.Writer(&out)).Run().Cause()
	s.NoError(err)

	// output should match the installed VS
	s.Equal(strings.TrimSpace(out.String()), installedVs)
}
