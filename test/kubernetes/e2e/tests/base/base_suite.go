package base

import (
	"context"
	"slices"
	"time"

	"github.com/onsi/gomega"
	"github.com/solo-io/gloo/test/kubernetes/e2e"
	"github.com/stretchr/testify/suite"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// This defines a test case used by the BaseTestingSuite
type TestCase struct {
	// SimpleTestCase defines the resources used by a specific test
	SimpleTestCase
	// SubTestCases contains a map for hierarchial tests within the current test
	// Eg: TestRateLimit
	//      |- OnVhost
	//      |- OnRoute
	SubTestCases map[string]*TestCase
}

// SimpleTestCase defines the resources used by a specific test
type SimpleTestCase struct {
	// manifest files
	Manifests []string
	// Resources expected to be created by manifest
	Resources []client.Object
}

var namespace string

type BaseTestingSuite struct {
	suite.Suite
	Ctx              context.Context
	TestInstallation *e2e.TestInstallation
	TestCase         map[string]*TestCase
	Setup            SimpleTestCase
}

func NewBaseTestingSuite(ctx context.Context, testInst *e2e.TestInstallation, setup SimpleTestCase, testCase map[string]*TestCase) *BaseTestingSuite {
	namespace = testInst.Metadata.InstallNamespace

	return &BaseTestingSuite{
		Ctx:              ctx,
		TestInstallation: testInst,
		TestCase:         testCase,
		Setup:            setup,
	}
}

func (s *BaseTestingSuite) SetupSuite() {
	for _, manifest := range s.Setup.Manifests {
		gomega.Eventually(func() error {
			err := s.TestInstallation.Actions.Kubectl().ApplyFile(s.Ctx, manifest)
			return err
		}, 10*time.Second, 1*time.Second).Should(gomega.Succeed(), "can apply "+manifest)
	}

	// Ensure the resources exist
	if s.Setup.Resources != nil {
		s.TestInstallation.Assertions.EventuallyObjectsExist(s.Ctx, s.Setup.Resources...)
	}
}

func (s *BaseTestingSuite) TearDownSuite() {
	// Delete the setup manifest
	if s.Setup.Manifests != nil {
		manifests := slices.Clone(s.Setup.Manifests)
		slices.Reverse(manifests)
		for _, manifest := range manifests {
			gomega.Eventually(func() error {
				err := s.TestInstallation.Actions.Kubectl().DeleteFile(s.Ctx, manifest)
				return err
			}, 10*time.Second, 1*time.Second).Should(gomega.Succeed(), "can delete "+manifest)
		}

		if s.Setup.Resources != nil {
			s.TestInstallation.Assertions.EventuallyObjectsNotExist(s.Ctx, s.Setup.Resources...)
		}
	}
}

func (s *BaseTestingSuite) BeforeTest(suiteName, testName string) {
	// apply test-specific manifests
	if s.TestCase == nil {
		return
	}

	testCase, ok := s.TestCase[testName]
	if !ok {
		return
	}

	for _, manifest := range testCase.Manifests {
		gomega.Eventually(func() error {
			err := s.TestInstallation.Actions.Kubectl().ApplyFile(s.Ctx, manifest)
			return err
		}, 10*time.Second, 1*time.Second).Should(gomega.Succeed(), "can apply "+manifest)
	}
	s.TestInstallation.Assertions.EventuallyObjectsExist(s.Ctx, testCase.Resources...)
}

func (s *BaseTestingSuite) AfterTest(suiteName, testName string) {
	if s.TestCase == nil {
		return
	}

	// Delete test-specific manifests
	testCase, ok := s.TestCase[testName]
	if !ok {
		return
	}

	// Delete them in reverse to avoid validation issues
	if testCase.Manifests != nil {
		manifests := slices.Clone(testCase.Manifests)
		slices.Reverse(manifests)
		for _, manifest := range manifests {
			gomega.Eventually(func() error {
				err := s.TestInstallation.Actions.Kubectl().DeleteFile(s.Ctx, manifest)
				return err
			}, 10*time.Second, 1*time.Second).Should(gomega.Succeed(), "can delete "+manifest)
		}
	}

	s.TestInstallation.Assertions.EventuallyObjectsNotExist(s.Ctx, testCase.Resources...)
}
