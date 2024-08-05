package base

import (
	"context"
	"slices"
	"time"

	"github.com/onsi/gomega"
	"github.com/solo-io/gloo/test/kubernetes/e2e"
	"github.com/solo-io/gloo/test/kubernetes/testutils/helper"
	"github.com/stretchr/testify/suite"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type TestCase struct {
	SimpleTestCase
	subTestCases map[string]*TestCase
}

type SimpleTestCase struct {
	// manifest files
	manifests []string
	// resources expected to be created by manifest
	resources []client.Object
	// values file passed during an upgrade
	upgradeValues string
	// rollback method to be called during cleanup.
	// Do not provide this. Calling an upgrade returns this method which we save
	rollback func() error
}

var namespace string

type BaseTestingSuite struct {
	suite.Suite
	Ctx              context.Context
	TestInstallation *e2e.TestInstallation
	TestHelper       *helper.SoloTestHelper
	TestCase         map[string]*TestCase
	Setup            SimpleTestCase
}

func NewBaseTestingSuite(ctx context.Context, testInst *e2e.TestInstallation, testHelper *helper.SoloTestHelper, setup SimpleTestCase, testCase map[string]*TestCase) *BaseTestingSuite {
	namespace = testInst.Metadata.InstallNamespace
	return &BaseTestingSuite{
		Ctx:              ctx,
		TestInstallation: testInst,
		TestHelper:       testHelper,
		TestCase:         testCase,
		Setup:            setup,
	}
}

func (s *BaseTestingSuite) SetupSuite() {
	if s.Setup.manifests != nil {
		for _, manifest := range s.Setup.manifests {
			gomega.Eventually(func() error {
				err := s.TestInstallation.Actions.Kubectl().ApplyFile(s.Ctx, manifest)
				return err
			}, 10*time.Second, 1*time.Second).Should(gomega.Succeed(), "can apply "+manifest)
		}
	}

	// Ensure the resources exist
	if s.Setup.resources != nil {
		s.TestInstallation.Assertions.EventuallyObjectsExist(s.Ctx, s.Setup.resources...)
		// TODO special case of pods running
	}

	if s.Setup.upgradeValues != "" {
		// Perform an upgrade to change settings, deployments, etc.
		var err error
		s.Setup.rollback, err = s.TestHelper.UpgradeGloo(s.Ctx, 600*time.Second, helper.WithExtraArgs([]string{
			// Reuse values so there's no need to know the prior values used
			"--reuse-values",
			"--values", s.Setup.upgradeValues,
		}...))
		s.TestInstallation.Assertions.Require.NoError(err)
	}
}

func (s *BaseTestingSuite) TearDownSuite() {
	if s.Setup.upgradeValues != "" {
		// Revet the upgrade applied before this test. This way we are sure that any changes
		// made are undone and we go back to a clean state
		err := s.Setup.rollback()
		s.TestInstallation.Assertions.Require.NoError(err)
	}

	// Delete the setup manifest
	if s.Setup.manifests != nil {
		manifests := slices.Clone(s.Setup.manifests)
		slices.Reverse(manifests)
		for _, manifest := range manifests {
			gomega.Eventually(func() error {
				err := s.TestInstallation.Actions.Kubectl().DeleteFile(s.Ctx, manifest)
				return err
			}, 10*time.Second, 1*time.Second).Should(gomega.Succeed(), "can delete "+manifest)
		}

		if s.Setup.resources != nil {
			s.TestInstallation.Assertions.EventuallyObjectsNotExist(s.Ctx, s.Setup.resources...)
			// TODO special case of pods running
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

	if testCase.upgradeValues != "" {
		// Perform an upgrade to change settings, deployments, etc.
		var err error
		testCase.rollback, err = s.TestHelper.UpgradeGloo(s.Ctx, 600*time.Second, helper.WithExtraArgs([]string{
			// Reuse values so there's no need to know the prior values used
			"--reuse-values",
			"--values", testCase.upgradeValues,
		}...))
		s.TestInstallation.Assertions.Require.NoError(err)
	}

	for _, manifest := range testCase.manifests {
		// TODO: Instead of sleeping, wrap this in an eventually
		time.Sleep(10 * time.Second)
		err := s.TestInstallation.Actions.Kubectl().ApplyFile(s.Ctx, manifest)
		s.NoError(err, "can apply "+manifest)
	}
	s.TestInstallation.Assertions.EventuallyObjectsExist(s.Ctx, testCase.resources...)
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

	if testCase.upgradeValues != "" {
		// Revet the upgrade applied before this test. This way we are sure that any changes
		// made are undone and we go back to a clean state
		err := testCase.rollback()
		s.TestInstallation.Assertions.Require.NoError(err)
	}

	// Delete them in reverse to avoid validation issues
	manifests := slices.Clone(testCase.manifests)
	slices.Reverse(manifests)
	for _, manifest := range manifests {
		time.Sleep(10 * time.Second)
		err := s.TestInstallation.Actions.Kubectl().DeleteFile(s.Ctx, manifest)
		s.NoError(err, "can delete "+manifest)
	}
	s.TestInstallation.Assertions.EventuallyObjectsNotExist(s.Ctx, testCase.resources...)
}
