package base

import (
	"context"
	"fmt"
	"path/filepath"
	"slices"
	"time"

	"github.com/onsi/gomega"
	"github.com/stretchr/testify/suite"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apiserverschema "k8s.io/apiextensions-apiserver/pkg/apiserver/schema"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	deployerinternal "github.com/kgateway-dev/kgateway/v2/internal/kgateway/deployer"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/deployer"
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e"
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e/defaults"
	"github.com/kgateway-dev/kgateway/v2/test/testutils"
)

// TestCase defines the manifests and resources used by a test or test suite.
type TestCase struct {
	// Manifests contains a list of manifest filenames.
	Manifests []string

	// manifestResources contains the resources automatically loaded from the manifest files for
	// this test case.
	manifestResources []client.Object

	// dynamicResources contains the expected dynamically provisioned resources for any Gateways
	// contained in this test case's manifests.
	dynamicResources []client.Object
}

type BaseTestingSuite struct {
	suite.Suite
	Ctx              context.Context
	TestInstallation *e2e.TestInstallation
	Setup            TestCase
	TestCases        map[string]*TestCase

	// (Optional) Path of directory (relative to git root) containing the CRDs that will be used to read
	// the objects from the manifests. If empty then defaults to "install/helm/kgateway-crds/templates"
	CrdPath string

	// (Optional) Helper to determine if a Gateway is self-managed. If not provided, a default implementation
	// is used.
	GatewayHelper GatewayHelper

	// used internally to parse the manifest files
	gvkToStructuralSchema map[schema.GroupVersionKind]*apiserverschema.Structural
}

// NewBaseTestingSuite returns a BaseTestingSuite that performs all the pre-requisites of upgrading helm installations,
// applying manifests and verifying resources exist before a suite and tests and the corresponding post-run cleanup.
// The pre-requisites for the suite are defined in the setup parameter and for each test in the individual testCase.
func NewBaseTestingSuite(ctx context.Context, testInst *e2e.TestInstallation, setupTestCase TestCase, testCases map[string]*TestCase) *BaseTestingSuite {
	return &BaseTestingSuite{
		Ctx:              ctx,
		TestInstallation: testInst,
		Setup:            setupTestCase,
		TestCases:        testCases,
	}
}

func (s *BaseTestingSuite) SetupSuite() {
	// set up the helpers once and store them on the suite
	s.setupHelpers()

	s.ApplyManifests(&s.Setup)
}

func (s *BaseTestingSuite) TearDownSuite() {
	s.DeleteManifests(&s.Setup)
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

	if s.T().Failed() {
		s.TestInstallation.PreFailHandler(s.Ctx)
	}

	s.DeleteManifests(testCase)
}

func (s *BaseTestingSuite) GetKubectlOutput(command ...string) string {
	out, _, err := s.TestInstallation.Actions.Kubectl().Execute(s.Ctx, command...)
	s.TestInstallation.Assertions.Require.NoError(err)

	return out
}

// ApplyManifests applies the manifests and waits until the resources are created and ready.
func (s *BaseTestingSuite) ApplyManifests(testCase *TestCase) {
	// apply the manifests
	for _, manifest := range testCase.Manifests {
		gomega.Eventually(func() error {
			err := s.TestInstallation.Actions.Kubectl().ApplyFile(s.Ctx, manifest)
			return err
		}, 10*time.Second, 1*time.Second).Should(gomega.Succeed(), "can apply "+manifest)
	}

	// parse the expected resources and dynamic resources from the manifests, and wait until the resources are created.
	// we must wait until the resources from the manifest exist on the cluster before calling loadDynamicResources,
	// because in order to determine what dynamic resources are expected, certain resources (e.g. Gateways and
	// GatewayParameters) must already exist on the cluster.
	s.loadManifestResources(testCase)
	s.TestInstallation.Assertions.EventuallyObjectsExist(s.Ctx, testCase.manifestResources...)
	s.loadDynamicResources(testCase)
	s.TestInstallation.Assertions.EventuallyObjectsExist(s.Ctx, testCase.dynamicResources...)

	// wait until pods are ready; this assumes that pods use a well-known label
	// app.kubernetes.io/name=<name>
	allResources := slices.Concat(testCase.manifestResources, testCase.dynamicResources)
	for _, resource := range allResources {
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
func (s *BaseTestingSuite) DeleteManifests(testCase *TestCase) {
	// parse the expected resources and dynamic resources from the manifests (this normally would already
	// have been done via ApplyManifests, but we check again here just in case ApplyManifests was not called).
	// we need to do this before calling delete on the manifests, so we can accurately determine which dynamic
	// resources need to be deleted.
	s.loadManifestResources(testCase)
	s.loadDynamicResources(testCase)

	for _, manifest := range testCase.Manifests {
		gomega.Eventually(func() error {
			err := s.TestInstallation.Actions.Kubectl().DeleteFileSafe(s.Ctx, manifest)
			return err
		}, 10*time.Second, 1*time.Second).Should(gomega.Succeed(), "can delete "+manifest)
	}

	// wait until the resources are deleted
	allResources := slices.Concat(testCase.manifestResources, testCase.dynamicResources)
	s.TestInstallation.Assertions.EventuallyObjectsNotExist(s.Ctx, allResources...)

	// wait until pods created by deployments are deleted; this assumes that pods use a well-known label
	// app.kubernetes.io/name=<name>
	for _, resource := range allResources {
		if deployment, ok := resource.(*appsv1.Deployment); ok {
			s.TestInstallation.Assertions.EventuallyPodsNotExist(s.Ctx, deployment.Namespace, metav1.ListOptions{
				LabelSelector: fmt.Sprintf("%s=%s", defaults.WellKnownAppLabel, deployment.Name),
			}, time.Second*120, time.Second*2)
		}
	}
}

func (s *BaseTestingSuite) setupHelpers() {
	if s.GatewayHelper == nil {
		s.GatewayHelper = newGatewayHelper(s.TestInstallation)
	}
	if s.CrdPath == "" {
		s.CrdPath = testutils.CRDPath
	}
	var err error
	s.gvkToStructuralSchema, err = testutils.GetStructuralSchemas(filepath.Join(testutils.GitRootDirectory(), s.CrdPath))
	s.Require().NoError(err)
}

// loadManifestResources populates the `manifestResources` for the given test case, by parsing each
// manifest file into a list of resources
func (s *BaseTestingSuite) loadManifestResources(testCase *TestCase) {
	if len(testCase.manifestResources) > 0 {
		// resources have already been loaded
		return
	}

	var resources []client.Object
	for _, manifest := range testCase.Manifests {
		objs, err := testutils.LoadFromFiles(manifest, s.TestInstallation.ClusterContext.Client.Scheme(), s.gvkToStructuralSchema)
		s.Require().NoError(err)
		resources = append(resources, objs...)
	}
	testCase.manifestResources = resources
}

// loadDynamicResources populates the `dynamicResources` for the given test case. For each Gateway
// in the test case, if it is not self-managed, then the expected dynamically provisioned resources
// are added to dynamicResources.
//
// This should only be called *after* loadManifestResources has been called and we have waited
// for all the manifest objects to be created. This is because the "is self-managed" check requires
// any dependent Gateways and GatewayParameters to exist on the cluster already.
func (s *BaseTestingSuite) loadDynamicResources(testCase *TestCase) {
	if len(testCase.dynamicResources) > 0 {
		// resources have already been loaded
		return
	}

	var dynamicResources []client.Object
	for _, obj := range testCase.manifestResources {
		if gw, ok := obj.(*gwv1.Gateway); ok {
			selfManaged, err := s.GatewayHelper.IsSelfManaged(s.Ctx, gw)
			s.Require().NoError(err)

			// if the gateway is not self-managed, then we expect a proxy deployment and service
			// to be created, so add them to the dynamic resource list
			if !selfManaged {
				proxyObjectMeta := metav1.ObjectMeta{
					Name:      gw.GetName(),
					Namespace: gw.GetNamespace(),
				}
				proxyResources := []client.Object{
					&appsv1.Deployment{ObjectMeta: proxyObjectMeta},
					&corev1.Service{ObjectMeta: proxyObjectMeta},
				}
				dynamicResources = append(dynamicResources, proxyResources...)
			}
		}
	}
	testCase.dynamicResources = dynamicResources
}

// GatewayHelper is an interface that can be implemented to provide a custom way to determine if a
// Gateway is self-managed.
type GatewayHelper interface {
	IsSelfManaged(ctx context.Context, gw *gwv1.Gateway) (bool, error)
}

type defaultGatewayHelper struct {
	gwpClient *deployerinternal.GatewayParameters
}

func newGatewayHelper(testInst *e2e.TestInstallation) *defaultGatewayHelper {
	gwpClient := deployerinternal.NewGatewayParameters(
		testInst.ClusterContext.Client,

		// empty is ok as we only care whether it's self-managed or not
		&deployer.Inputs{
			ImageInfo:                &deployer.ImageInfo{},
			GatewayClassName:         wellknown.DefaultGatewayClassName,
			WaypointGatewayClassName: wellknown.DefaultWaypointClassName,
			AgentgatewayClassName:    wellknown.DefaultAgwClassName,
		},
	)
	return &defaultGatewayHelper{gwpClient: gwpClient}
}

func (h *defaultGatewayHelper) IsSelfManaged(ctx context.Context, gw *gwv1.Gateway) (bool, error) {
	return h.gwpClient.IsSelfManaged(ctx, gw)
}
