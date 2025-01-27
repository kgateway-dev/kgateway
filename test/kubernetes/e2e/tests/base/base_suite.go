package base

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/onsi/gomega"
	"github.com/solo-io/gloo/test/kubernetes/e2e"
	"github.com/solo-io/gloo/test/kubernetes/testutils/helper"
	"github.com/stretchr/testify/suite"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
	// values file passed during an upgrade
	UpgradeValues string
	// Rollback method to be called during cleanup.
	// Do not provide this. Calling an upgrade returns this method which we save
	Rollback func() error
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

// NewBaseTestingSuite returns a BaseTestingSuite that performs all the pre-requisites of upgrading helm installations,
// applying manifests and verifying resources exist before a suite and tests and the corresponding post-run cleanup.
// The pre-requisites for the suite are defined in the setup parameter and for each test in the individual testCase.
// Currently, tests that require upgrades (eg: to change settings) can not be run in Enterprise. To do so,
// the test must be written without upgrades and call the `NewBaseTestingSuiteWithoutUpgrades` constructor.
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

// NewBaseTestingSuiteWithoutUpgrades returns a BaseTestingSuite without allowing upgrades and reverts before the suite and tests.
// This is useful when creating tests that need to run in Enterprise since the helm values change between OSS and Enterprise installations.
func NewBaseTestingSuiteWithoutUpgrades(ctx context.Context, testInst *e2e.TestInstallation, setup SimpleTestCase, testCase map[string]*TestCase) *BaseTestingSuite {
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

		for _, resource := range s.Setup.Resources {
			if pod, ok := resource.(*corev1.Pod); ok {
				s.TestInstallation.Assertions.EventuallyPodsRunning(s.Ctx, pod.Namespace, metav1.ListOptions{
					LabelSelector: fmt.Sprintf("app.kubernetes.io/name=%s", pod.Name),
				})
			}
		}
	}

	if s.Setup.UpgradeValues != "" {
		if s.TestHelper == nil {
			panic("The base suite was configured to disable upgrades")
		}

		// Perform an upgrade to change settings, deployments, etc.
		var err error
		s.Setup.Rollback, err = s.TestHelper.UpgradeGloo(s.Ctx, 600*time.Second, helper.WithExtraArgs([]string{
			// Reuse values so there's no need to know the prior values used
			"--reuse-values",
			"--values", s.Setup.UpgradeValues,
		}...))
		s.TestInstallation.Assertions.Require.NoError(err)
	}
}

func (s *BaseTestingSuite) TearDownSuite() {
	if s.Setup.UpgradeValues != "" {
		if s.TestHelper == nil {
			panic("The base suite was configured to disable upgrades")
		}

		// Revet the upgrade applied before this test. This way we are sure that any changes
		// made are undone and we go back to a clean state
		err := s.Setup.Rollback()
		s.TestInstallation.Assertions.Require.NoError(err)
	}

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

	if testCase.UpgradeValues != "" {
		if s.TestHelper == nil {
			panic("The base suite was configured to disable upgrades")
		}

		// Perform an upgrade to change settings, deployments, etc.
		var err error
		testCase.Rollback, err = s.TestHelper.UpgradeGloo(s.Ctx, 600*time.Second, helper.WithExtraArgs([]string{
			// Reuse values so there's no need to know the prior values used
			"--reuse-values",
			"--values", testCase.UpgradeValues,
		}...))
		s.TestInstallation.Assertions.Require.NoError(err)
	}

	for _, manifest := range testCase.Manifests {
		gomega.Eventually(func() error {
			err := s.TestInstallation.Actions.Kubectl().ApplyFile(s.Ctx, manifest)
			return err
		}, 10*time.Second, 1*time.Second).Should(gomega.Succeed(), "can apply "+manifest)
	}
	s.TestInstallation.Assertions.EventuallyObjectsExist(s.Ctx, testCase.Resources...)

	for _, resource := range testCase.Resources {
		if pod, ok := resource.(*corev1.Pod); ok {
			s.TestInstallation.Assertions.EventuallyPodsRunning(s.Ctx, pod.Namespace, metav1.ListOptions{
				LabelSelector: fmt.Sprintf("app.kubernetes.io/name=%s", pod.Name),
			})
		}
	}

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

	if testCase.UpgradeValues != "" {
		if s.TestHelper == nil {
			panic("The base suite was configured to disable upgrades")
		}

		// Revet the upgrade applied before this test. This way we are sure that any changes
		// made are undone and we go back to a clean state
		err := testCase.Rollback()
		s.TestInstallation.Assertions.Require.NoError(err)
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

func (s *BaseTestingSuite) GetKubectlOutput(command ...string) string {
	out, _, err := s.TestInstallation.Actions.Kubectl().Execute(s.Ctx, command...)
	s.TestInstallation.Assertions.Require.NoError(err)

	return out
}

func (s *BaseTestingSuite) UpgradeWithCustomValuesFile(valuesFile string) {
	_, err := s.TestHelper.UpgradeGloo(s.Ctx, 600*time.Second, helper.WithExtraArgs([]string{
		// Do not reuse the existing values as we need to install the new chart with the new version of the images
		"--values", valuesFile,
	}...))
	s.TestInstallation.Assertions.Require.NoError(err)
}
