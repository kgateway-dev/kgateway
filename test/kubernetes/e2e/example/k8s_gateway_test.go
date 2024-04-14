package example_test

import (
	"context"

	"github.com/solo-io/gloo/test/kubernetes/e2e/features/deployer"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/test/kubernetes/e2e"
	"github.com/solo-io/gloo/test/kubernetes/testutils/gloogateway"
)

var _ = Describe("K8s Gateway Example Test", Ordered, func() {

	// An entire file is meant to capture the behaviors that we want to test for a given installation of Gloo Gateway

	var (
		ctx context.Context

		// testInstallation contains all the metadata/utilities necessary to execute a series of tests
		// against an installation of Gloo Gateway
		testInstallation *e2e.TestInstallation
	)

	BeforeAll(func() {
		ctx = context.Background()

		testInstallation = testSuite.RegisterTestInstallation(
			"k8s-gw-example-test",
			&gloogateway.Context{
				InstallNamespace:   "k8s-gw-example-test",
				ValuesManifestFile: e2e.ManifestPath("example", "manifests", "k8s-gateway-test-helm.yaml"),
			},
		)

		err := testInstallation.InstallGlooGateway(ctx, testInstallation.Actions.GlooCtl().NewTestHelperInstallAction())
		Expect(err).NotTo(HaveOccurred())
	})

	AfterAll(func() {
		err := testInstallation.UninstallGlooGateway(ctx, testInstallation.Actions.GlooCtl().NewTestHelperUninstallAction())
		Expect(err).NotTo(HaveOccurred())

		testSuite.UnregisterTestInstallation(testInstallation)
	})

	Context("K8s Gateway Integration - Deployer", func() {

		It("provisions resources appropriately", func() {
			testInstallation.RunTests(
				ctx,
				deployer.ProvisionDeploymentAndService,
				deployer.ConfigureProxiesFromGatewayParameters,
			)
		})

	})

})
