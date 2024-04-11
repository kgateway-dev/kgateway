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
	"path/filepath"
)

var _ = Describe("Example Test", Ordered, func() {

	var (
		ctx context.Context

		scenarioProvider *spec.ScenarioProvider

		assertionProvider *specassertions.Provider
	)

	BeforeAll(func() {
		ctx = context.Background()

		// Set the scenario provider to point to the running cluster
		scenarioProvider = spec.NewProvider().WithClusterContext(clusterContext)

		// Set the assertion provider to point to the running cluster
		assertionProvider = specassertions.NewProvider().WithClusterContext(clusterContext)

		// TODO: if there is anything in the Gloo Gateway install context that is useful for these
		// providers, we should update that here
	})

	Context("Spec Scenarios", func() {

		It("works", func() {
			// These are the resources that we expect to be dynamically provisioned when we run the test
			// The name and namespace of these objects is determined from the manifest file
			proxyDeployment := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "gloo-proxy-gw", Namespace: "default"}}
			proxyService := &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "gloo-proxy-gw", Namespace: "default"}}

			spec, err := scenarioProvider.NewScenario(
				spec.WithName("basic-test"),
				spec.WithManifestFile(filepath.Join(cwd, "manifests", "basic-test.yaml")),
				spec.WithInitializedAssertion(
					assertionProvider.ObjectsExist(proxyDeployment, proxyService),
				),
				spec.WithAssertion(func(ctx context.Context) {
					// This test is mainly an Integration-style test,
					// in that we just assert objects are created and destroyed

					// If we wanted to expand the usage, this is the function where we would assert traffic behaviors
				}),
				spec.WithFinalizedAssertion(
					assertionProvider.ObjectsNotExist(proxyDeployment, proxyService),
				),
			)
			Expect(err).NotTo(HaveOccurred())

			err = scenarioRunner.RunScenario(ctx, spec)
			Expect(err).NotTo(HaveOccurred())
		})

	})

})
