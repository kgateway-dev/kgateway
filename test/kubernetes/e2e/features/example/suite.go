//go:build ignore

package example

import (
	"context"

	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/suite"

	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e"
)

var _ e2e.NewSuiteFunc = NewTestingSuite

// testingSuite is the entire Suite of tests for the "example" feature
// Typically, we would include a link to the feature code here
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

func (s *testingSuite) SetupSuite() {
	// This is code that will be executed before an entire suite is run
}

func (s *testingSuite) TearDownSuite() {
	// This is code that will be executed after an entire suite is run
}

func (s *testingSuite) BeforeTest(suiteName, testName string) {
	// This is code that will be executed before each test is run
}

func (s *testingSuite) AfterTest(suiteName, testName string) {
	// This is code that will be executed after each test is run
}

func (s *testingSuite) TestExampleAssertion() {
	// Testify assertion
	s.Assert().NotEqual(1, 2, "1 does not equal 2")

	// Testify assertion, using the TestInstallation to provide it
	s.testInstallation.Assertions.Require.NotEqual(1, 2, "1 does not equal 2")

	// Gomega assertion, using the TestInstallation to provide it
	s.testInstallation.Assertions.Gomega.Expect(1).NotTo(Equal(2), "1 does not equal 2")
}
