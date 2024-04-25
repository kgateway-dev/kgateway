package example

import (
	"context"

	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/test/kubernetes/e2e"
	"github.com/stretchr/testify/suite"
)

// ExampleSuite is the entire Suite of tests for the "example" feature
// Typically, we would include a link to the feature code here
// We intentionally name this ExampleSuite even though the package is example, and thus the name stutters a bit
// This is because we can run individual tests by specifying test suites:
// go test -run TestBasicInstallation/ExampleSuite/TestExampleAssertion
// So this naming strategy makes the format consistent
type ExampleSuite struct {
	suite.Suite

	ctx context.Context

	// testInstallation contains all the metadata/utilities necessary to execute a series of tests
	// against an installation of Gloo Gateway
	testInstallation *e2e.TestInstallation
}

func NewTestingSuite(ctx context.Context, testInst *e2e.TestInstallation) suite.TestingSuite {
	return &ExampleSuite{
		ctx:              ctx,
		testInstallation: testInst,
	}
}

func (s *ExampleSuite) SetupSuite() {
}

func (s *ExampleSuite) TearDownSuite() {
}

func (s *ExampleSuite) BeforeTest(suiteName, testName string) {
}

func (s *ExampleSuite) AfterTest(suiteName, testName string) {
}

func (s *ExampleSuite) TestExampleAssertion() {
	// Testify assertion
	s.Assert().NotEqual(1, 2, "1 does not equal 2")

	// Testify assertion, using the TestInstallation to provide it
	s.testInstallation.Assertions.NotEqual(1, 2, "1 does not equal 2")

	// Gomega assertion, using the TestInstallation to provide it
	s.testInstallation.Assertions.Expect(1).NotTo(Equal(2), "1 does not equal 2")
}
