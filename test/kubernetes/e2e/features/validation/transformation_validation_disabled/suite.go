package transformation_validation_disabled

import (
	"context"

	"github.com/solo-io/gloo/test/kubernetes/e2e"
	"github.com/solo-io/gloo/test/kubernetes/e2e/features/validation"
	"github.com/stretchr/testify/suite"
)

var _ e2e.NewSuiteFunc = NewTestingSuite

// testingSuite is the entire Suite of tests for the webhook validation disableTransformationValidation=true feature
type testingSuite struct {
	suite.Suite

	ctx context.Context

	// testInstallation contains all the metadata/utilities necessary to execute a series of tests
	// against an installation of Gloo Gateway
	testInstallation *e2e.TestInstallation
}

func NewTestingSuite(ctx context.Context, testInst *e2e.TestInstallation) suite.TestingSuite {
	return &testingSuite{
		ctx:              ctx,
		testInstallation: testInst,
	}
}

// TestDoesNotReject checks webhook does not reject invalid transformation when disable_transformation_validation=false
func (s *testingSuite) TestDoesNotReject() {
	// accepts invalid inja template in transformation
	err := s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, validation.VSTransformationExtractors, "-n", s.testInstallation.Metadata.InstallNamespace)
	s.Assert().NoError(err)

	// Extract mode -- accepts invalid subgroup in transformation
	// note that the regex has no subgroups, but we are trying to extract the first subgroup
	// this should be rejected
	err = s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, validation.VSTransformationHeaderText, "-n", s.testInstallation.Metadata.InstallNamespace)
	s.Assert().NoError(err)

	// Single replace mode -- accepts invalid subgroup in transformation
	// note that the regex has no subgroups, but we are trying to extract the first subgroup
	// this should be rejected
	err = s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, validation.VSTransformationSingleReplace, "-n", s.testInstallation.Metadata.InstallNamespace)
	s.Assert().NoError(err)
}
