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

func (s *crdSuite) BeforeTest(suiteName, testName string) {
	// This is code that will be executed before each test is run
}

func (s *crdSuite) AfterTest(suiteName, testName string) {
	// This is code that will be executed after each test is run
}
