package example_test

import (
	"context"
	"github.com/solo-io/gloo/test/kubernetes/e2e"
	"github.com/solo-io/gloo/test/kubernetes/testutils/assertions"
	"github.com/solo-io/gloo/test/kubernetes/testutils/operations/provider"
	"os"
	"path/filepath"

	"github.com/solo-io/gloo/test/kubernetes/testutils/operations"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = e2e.SuiteDescribe("Example Test", func(suiteCtx *e2e.SuiteContext) {

	// This is just a way to extract some of the variables that are defined at the Suite level,
	// and make them available to the given test
	var (
		operator           *operations.Operator
		operationsProvider *provider.OperationProvider
		assertionProvider  *assertions.Provider
	)

	BeforeAll(func() {
		assertionProvider = suiteCtx.AssertionProvider
		operationsProvider = suiteCtx.OperationsProvider
		operator = suiteCtx.Operator
	})

	var (
		ctx context.Context

		cwd string
	)

	BeforeAll(func() {
		ctx = context.Background()

		var err error
		cwd, err = os.Getwd()
		Expect(err).NotTo(HaveOccurred(), "working dir could not be retrieved while installing gloo")

		installOp, err := operationsProvider.Installs().NewGlooctlInstallOperation(filepath.Join(cwd, "manifests", "helm.yaml"))
		Expect(err).NotTo(HaveOccurred())

		err = operator.ExecuteOperations(ctx, installOp)
		Expect(err).NotTo(HaveOccurred())

		// TODO: if there is anything in the Gloo Gateway install context that is useful for these
		// providers, we should update that here
	})

	AfterAll(func() {
		uninstallOp, err := operationsProvider.Installs().NewGlooctlUninstallOperation()
		Expect(err).NotTo(HaveOccurred())

		err = operator.ExecuteOperations(ctx, uninstallOp)
		Expect(err).NotTo(HaveOccurred())
	})

	Context("Spec Scenarios", func() {

		It("works", func() {
			// These are the resources that we expect to be dynamically provisioned when we run the test
			// The name and namespace of these objects is determined from the manifest file
			proxyDeployment := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "gloo-proxy-gw", Namespace: "default"}}
			proxyService := &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "gloo-proxy-gw", Namespace: "default"}}

			manifestFile := filepath.Join(cwd, "manifests", "basic-test.yaml")
			createResourcesOp := operationsProvider.Manifests().NewApplyManifestOperation(manifestFile,
				assertionProvider.ObjectsExist(proxyService, proxyDeployment))
			deleteResourcesOp := operationsProvider.Manifests().NewDeleteManifestOperation(manifestFile,
				assertionProvider.ObjectsNotExist(proxyService, proxyDeployment))

			err := operator.ExecuteReversibleOperations(ctx, operations.ReversibleOperation{
				Do:   createResourcesOp,
				Undo: deleteResourcesOp,
			})
			Expect(err).NotTo(HaveOccurred())
		})

		It("fails to produce running replicas", func() {
			// These are the resources that we expect to be dynamically provisioned when we run the test
			// The name and namespace of these objects is determined from the manifest file
			proxyDeployment := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "gloo-proxy-gw", Namespace: "default"}}
			proxyService := &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "gloo-proxy-gw", Namespace: "default"}}

			// UX of this is wonky, as a dev what am I actually adding to the cluster?
			manifestFile := filepath.Join(cwd, "manifests", "basic-test.yaml")
			createResourcesOp := operationsProvider.Manifests().NewApplyManifestOperation(manifestFile,
				assertionProvider.ObjectsExist(proxyService, proxyDeployment),
				assertionProvider.RunningReplicas(&core.ResourceRef{
					Name:      "gloo-proxy-gw",
					Namespace: "default",
				}, 1))
			deleteResourcesOp := operationsProvider.Manifests().NewDeleteManifestOperation(manifestFile,
				assertionProvider.ObjectsNotExist(proxyService, proxyDeployment))

			err := operator.ExecuteReversibleOperations(ctx, operations.ReversibleOperation{
				Do:   createResourcesOp,
				Undo: deleteResourcesOp,
			})
			Expect(err).To(HaveOccurred(), "The gloo-proxy-gw that is deployed has the tag 1.0.0-ci which will fail to start")
		})

		It("example", func() {

			// create resources
			// check they are there
			// check behavior for rate limiting
			// 1 operation

			// then i want to change the defintion
			// assert new behavior
			// 2 operation

			// operation.ExecuteOperation(1,2)
			// do-1, do-2, undo-2, undo-1

			// TODOs:
			// 1. PR that intorduces the framework with
			// include more complex tests to demonstrate the framework
			// get PR reviewed for framework
			// 1 suite completely re-written, as a separate PR off this
			// get PR for framework merged
			// get suite re-write PR merged
			//

			//
			// create an httproute and httpgateway
			// send traffic

			// then you create a gw parameters
			// expect resources to change
			// test traffic

			// cleanup both
			// side-effect of creating the gw, the deployer creates some resources on your behalf,
			// those should get deleted?
			//
		})

	})

})
