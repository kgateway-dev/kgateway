package validation_strict

import (
	"context"

	"github.com/solo-io/gloo/test/kubernetes/e2e"
	"github.com/stretchr/testify/suite"
)

var _ e2e.NewSuiteFunc = NewTestingSuite

// testingSuite is the entire Suite of tests for the webhook validation alwaysAccept=false feature
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

// TestDoesNotReject checks webhook does not reject invalid transformation when server_enabled=false
func (s *testingSuite) TestDoesNotReject() {
	// accepts invalid inja template in transformation

	// accepts invalid subgroup in transformation

	// accepts invalid subgroup in transformation
}
