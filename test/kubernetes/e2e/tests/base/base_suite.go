package base

import (
	"context"
	"fmt"
	"time"

	"github.com/onsi/gomega"
	"github.com/stretchr/testify/suite"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e"
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e/defaults"
)

// TestCase defines the manifests and resources used by a test or test suite.
type TestCase struct {
	// Manifests contains a list of manifest filenames.
	Manifests []string
	// Resources contains a list of objects that are expected to be created by the manifest files.
	Resources []client.Object
	// values file passed during an upgrade
	// UpgradeValues string
	// Rollback method to be called during cleanup.
	// Do not provide this. Calling an upgrade returns this method which we save
	//Rollback func() error
}

type BaseTestingSuite struct {
	suite.Suite
	Ctx              context.Context
	TestInstallation *e2e.TestInstallation
	TestCases        map[string]TestCase
	Setup            TestCase
}

// NewBaseTestingSuite returns a BaseTestingSuite that performs all the pre-requisites of upgrading helm installations,
// applying manifests and verifying resources exist before a suite and tests and the corresponding post-run cleanup.
// The pre-requisites for the suite are defined in the setup parameter and for each test in the individual testCase.
// Currently, tests that require upgrades (eg: to change settings) can not be run in Enterprise. To do so,
// the test must be written without upgrades and call the `NewBaseTestingSuiteWithoutUpgrades` constructor.
func NewBaseTestingSuite(ctx context.Context, testInst *e2e.TestInstallation, setupTestCase TestCase, testCases map[string]TestCase) *BaseTestingSuite {
	return &BaseTestingSuite{
		Ctx:              ctx,
		TestInstallation: testInst,
		TestCases:        testCases,
		Setup:            setupTestCase,
	}
}

// NewBaseTestingSuiteWithoutUpgrades returns a BaseTestingSuite without allowing upgrades and reverts before the suite and tests.
// This is useful when creating tests that need to run in Enterprise since the helm values change between OSS and Enterprise installations.
func NewBaseTestingSuiteWithoutUpgrades(ctx context.Context, testInst *e2e.TestInstallation, setupTestCase TestCase, testCases map[string]TestCase) *BaseTestingSuite {
	return &BaseTestingSuite{
		Ctx:              ctx,
		TestInstallation: testInst,
		TestCases:        testCases,
		Setup:            setupTestCase,
	}
}

func (s *BaseTestingSuite) SetupSuite() {
	s.ApplyManifests(s.Setup)

	// TODO handle upgrades https://github.com/kgateway-dev/kgateway/issues/10609
	// if s.Setup.UpgradeValues != "" {
	// 	// Perform an upgrade to change settings, deployments, etc.
	// 	var err error
	// 	s.Setup.Rollback, err = s.TestHelper.UpgradeGloo(s.Ctx, 600*time.Second, helper.WithExtraArgs([]string{
	// 		// Reuse values so there's no need to know the prior values used
	// 		"--reuse-values",
	// 		"--values", s.Setup.UpgradeValues,
	// 	}...))
	// 	s.TestInstallation.Assertions.Require.NoError(err)
	// }
}

func (s *BaseTestingSuite) TearDownSuite() {
	// TODO handle upgrades https://github.com/kgateway-dev/kgateway/issues/10609
	// if s.Setup.UpgradeValues != "" {
	// 	// Revet the upgrade applied before this test. This way we are sure that any changes
	// 	// made are undone and we go back to a clean state
	// 	err := s.Setup.Rollback()
	// 	s.TestInstallation.Assertions.Require.NoError(err)
	// }

	s.DeleteManifests(s.Setup)
}

func (s *BaseTestingSuite) BeforeTest(suiteName, testName string) {
	// apply test-specific manifests
	testCase, ok := s.TestCases[testName]
	if !ok {
		return
	}

	// TODO handle upgrades https://github.com/kgateway-dev/kgateway/issues/10609
	// if testCase.UpgradeValues != "" {
	// 	// Perform an upgrade to change settings, deployments, etc.
	// 	var err error
	// 	testCase.Rollback, err = s.TestHelper.UpgradeGloo(s.Ctx, 600*time.Second, helper.WithExtraArgs([]string{
	// 		// Reuse values so there's no need to know the prior values used
	// 		"--reuse-values",
	// 		"--values", testCase.UpgradeValues,
	// 	}...))
	// 	s.TestInstallation.Assertions.Require.NoError(err)
	// }

	s.ApplyManifests(testCase)
}

func (s *BaseTestingSuite) AfterTest(suiteName, testName string) {
	// Delete test-specific manifests
	testCase, ok := s.TestCases[testName]
	if !ok {
		return
	}

	// TODO handle upgrades https://github.com/kgateway-dev/kgateway/issues/10609
	// if testCase.UpgradeValues != "" {
	// 	// Revet the upgrade applied before this test. This way we are sure that any changes
	// 	// made are undone and we go back to a clean state
	// 	err := testCase.Rollback()
	// 	s.TestInstallation.Assertions.Require.NoError(err)
	// }

	s.DeleteManifests(testCase)
}

func (s *BaseTestingSuite) GetKubectlOutput(command ...string) string {
	out, _, err := s.TestInstallation.Actions.Kubectl().Execute(s.Ctx, command...)
	s.TestInstallation.Assertions.Require.NoError(err)

	return out
}

// TODO handle upgrades https://github.com/kgateway-dev/kgateway/issues/10609
// func (s *BaseTestingSuite) UpgradeWithCustomValuesFile(valuesFile string) {
// 	_, err := s.TestHelper.UpgradeGloo(s.Ctx, 600*time.Second, helper.WithExtraArgs([]string{
// 		// Do not reuse the existing values as we need to install the new chart with the new version of the images
// 		"--values", valuesFile,
// 	}...))
// 	s.TestInstallation.Assertions.Require.NoError(err)
// }

// ApplyManifests applies the manifests and waits until the resources are created and ready.
func (s *BaseTestingSuite) ApplyManifests(testCase TestCase) {
	// apply the manifests
	for _, manifest := range testCase.Manifests {
		gomega.Eventually(func() error {
			err := s.TestInstallation.Actions.Kubectl().ApplyFile(s.Ctx, manifest)
			return err
		}, 10*time.Second, 1*time.Second).Should(gomega.Succeed(), "can apply "+manifest)
	}

	// wait until the resources are created
	s.TestInstallation.Assertions.EventuallyObjectsExist(s.Ctx, testCase.Resources...)

	// wait until pods are ready; this assumes that pods use a well-known label
	// app.kubernetes.io/name=<name>
	for _, resource := range testCase.Resources {
		var ns, name string
		if pod, ok := resource.(*corev1.Pod); ok {
			ns = pod.Namespace
			name = pod.Name
		} else if deployment, ok := resource.(*appsv1.Deployment); ok {
			ns = deployment.Namespace
			name = deployment.Name
		} else {
			continue
		}
		s.TestInstallation.Assertions.EventuallyPodsRunning(s.Ctx, ns, metav1.ListOptions{
			LabelSelector: fmt.Sprintf("%s=%s", defaults.WellKnownAppLabel, name),
			// Provide a longer timeout as the pod needs to be pulled and pass HCs
		}, time.Second*60, time.Second*2)
	}
}

// DeleteManifests deletes the manifests and waits until the resources are deleted.
func (s *BaseTestingSuite) DeleteManifests(testCase TestCase) {
	for _, manifest := range testCase.Manifests {
		gomega.Eventually(func() error {
			err := s.TestInstallation.Actions.Kubectl().DeleteFileSafe(s.Ctx, manifest)
			return err
		}, 10*time.Second, 1*time.Second).Should(gomega.Succeed(), "can delete "+manifest)
	}

	s.TestInstallation.Assertions.EventuallyObjectsNotExist(s.Ctx, testCase.Resources...)

	// wait until pods created by deployments are deleted; this assumes that pods use a well-known label
	// app.kubernetes.io/name=<name>
	for _, resource := range testCase.Resources {
		if deployment, ok := resource.(*appsv1.Deployment); ok {
			s.TestInstallation.Assertions.EventuallyPodsNotExist(s.Ctx, deployment.Namespace, metav1.ListOptions{
				LabelSelector: fmt.Sprintf("%s=%s", defaults.WellKnownAppLabel, deployment.Name),
			}, time.Second*120, time.Second*2)
		}
	}
}
