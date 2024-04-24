package example

import (
	"context"
	"github.com/solo-io/gloo/test/kubernetes/e2e"
	"github.com/stretchr/testify/suite"
)

// Suite is the entire Suite of tests for the "example" feature
type Suite struct {
	suite.Suite

	ctx context.Context

	// testInstallation contains all the metadata/utilities necessary to execute a series of tests
	// against an installation of Gloo Gateway
	testInstallation *e2e.TestInstallation
}

func NewSuite(ctx context.Context, testInst *e2e.TestInstallation) *Suite {
	return &Suite{
		ctx:              ctx,
		testInstallation: testInst,
	}
}

func (s *Suite) SetupSuite() {
}

func (s *Suite) TearDownSuite() {
}

func (s *Suite) BeforeTest(suiteName, testName string) {
}

func (s *Suite) AfterTest(suiteName, testName string) {
}
