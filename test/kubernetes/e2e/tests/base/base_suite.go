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
func NewBaseTestingSuite(ctx context.Context, testInst *e2e.TestInstallation, setupTestCase TestCase, testCases map[string]TestCase) *BaseTestingSuite {
	return &BaseTestingSuite{
		Ctx:              ctx,
		TestInstallation: testInst,
		TestCases:        testCases,
		Setup:            setupTestCase,
	}
}

func (s *BaseTestingSuite) SetupSuite() {
	s.ApplyManifests(s.Setup)
}

func (s *BaseTestingSuite) TearDownSuite() {
	s.DeleteManifests(s.Setup)
}

func (s *BaseTestingSuite) BeforeTest(suiteName, testName string) {
	// apply test-specific manifests
	testCase, ok := s.TestCases[testName]
	if !ok {
		return
	}

	s.ApplyManifests(testCase)
}

func (s *BaseTestingSuite) AfterTest(suiteName, testName string) {
	// Delete test-specific manifests
	testCase, ok := s.TestCases[testName]
	if !ok {
		return
	}

	s.DeleteManifests(testCase)
}

func (s *BaseTestingSuite) GetKubectlOutput(command ...string) string {
	out, _, err := s.TestInstallation.Actions.Kubectl().Execute(s.Ctx, command...)
	s.TestInstallation.Assertions.Require.NoError(err)

	return out
}

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
