package glooctl

import (
	"context"

	"github.com/solo-io/gloo/test/kubernetes/e2e"
	"github.com/stretchr/testify/suite"
)

type crdSuite struct {
	suite.Suite

	ctx              context.Context
	testInstallation *e2e.TestInstallation
}

func NewCRDSuite(ctx context.Context, testInst *e2e.TestInstallation) suite.TestingSuite {
	return &crdSuite{
		ctx:              ctx,
		testInstallation: testInst,
	}
}

func (s *crdSuite) SetupSuite() {
	// This is code that will be executed before an entire suite is run
}

func (s *crdSuite) TearDownSuite() {
	// This is code that will be executed after an entire suite is run
}

func (s *crdSuite) TestCheckCRDsErrorsForMismatch() {
	err := s.testInstallation.Actions.Glooctl().RunCommand(s.ctx, "check-crds", "--version", "1.9.0")
	s.Error(err, "crds should be out of date")
	s.Contains(err.Error(), "One or more CRDs are out of date")
}
