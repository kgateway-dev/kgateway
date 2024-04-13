package example_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/test/kubernetes/e2e"
	"github.com/solo-io/gloo/test/kubernetes/e2e/features/deployer"
)

var _ = Describe("K8s Gateway Example Test", Ordered, func() {

	// An entire file is meant to capture the behaviors that we want to test for a given installation of Gloo Gateway

	var (
		ctx context.Context
	)

	BeforeAll(func() {
		ctx = context.Background()

		manifestPath := e2e.ManifestPath("example", "manifests", "k8s-gateway-test-helm.yaml")
		installOp, err := testSuite.OperationsProvider.GlooCtl().NewInstallOperation(manifestPath)
		Expect(err).NotTo(HaveOccurred())

		err = testSuite.Operator.ExecuteOperations(ctx, installOp)
		Expect(err).NotTo(HaveOccurred())

		// TODO: if there is anything in the Gloo Gateway install context that is useful for these
		// providers, we should update that here
	})

	AfterAll(func() {
		uninstallOp, err := testSuite.OperationsProvider.GlooCtl().NewUninstallOperation()
		Expect(err).NotTo(HaveOccurred())

		err = testSuite.Operator.ExecuteOperations(ctx, uninstallOp)
		Expect(err).NotTo(HaveOccurred())
	})

	Context("K8s Gateway Integration - Deployer", func() {

		It("provisions resources appropriately", func() {
			testSuite.RunTests(
				ctx,
				deployer.ProvisionDeploymentAndService,
				deployer.RouteIngressTraffic,
			)
		})

	})

})
