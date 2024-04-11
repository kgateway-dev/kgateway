package example

import (
	"context"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/test/kube2e/testutils/spec"
	"github.com/solo-io/gloo/test/kube2e/testutils/specassertions"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("Example Test", Ordered, func() {

	var (
		ctx context.Context

		scenarioRunner   *spec.ScenarioRunner
		scenarioProvider *spec.ScenarioProvider

		assertionProvider *specassertions.Provider
	)

	BeforeAll(func() {
		//
		scenarioRunner = spec.NewGinkgoScenarioRunner()

		// Set the scenario provider to point to the running cluster
		scenarioProvider = spec.NewProvider().WithClusterContext(clusterContext)

		// Set the assertion provider to point to the running cluster
		assertionProvider = specassertions.NewProvider().WithClusterContext(clusterContext)

		// Install with values

	})

	AfterAll(func() {

		// Uninstall with values

	})

	Context("Spec Scenarios", func() {

		It("works", func() {
			// These are the resources that we expect to be dynamically provisioned when we run the test
			// The name and namespace of these objects is determined from the manifest file
			proxyDeployment := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "proxyName", Namespace: "httpbin"}}
			proxyService := &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "proxyName", Namespace: "httpbin"}}

			spec := scenarioProvider.NewScenario(
				spec.WithName("basic-test"),
				spec.WithManifestFile("manifests/basic-test.txt"),
				spec.WithInitializedAssertion(assertionProvider.ObjectsExist(proxyDeployment, proxyService)),
				spec.WithAssertion(func(ctx context.Context) {
					// This test is mainly an Integration-style test, in that we just assert objects are
					// created and destroyed

				}),
				spec.WithFinalizedAssertion(assertionProvider.ObjectsNotExist(proxyDeployment, proxyService)),
			)

			err := scenarioRunner.RunScenario(ctx, spec)
			Expect(err).NotTo(HaveOccurred())
		})

	})

})
