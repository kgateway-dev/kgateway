package example_test

import (
	"context"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/test/kubernetes/testutils/operations/manifest"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("Example Test", Ordered, func() {

	var (
		ctx context.Context
	)

	BeforeAll(func() {
		ctx = context.Background()

		// TODO: if there is anything in the Gloo Gateway install context that is useful for these
		// providers, we should update that here
	})

	Context("Spec Scenarios", func() {

		It("works", func() {
			// These are the resources that we expect to be dynamically provisioned when we run the test
			// The name and namespace of these objects is determined from the manifest file
			proxyDeployment := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "gloo-proxy-gw", Namespace: "default"}}
			proxyService := &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "gloo-proxy-gw", Namespace: "default"}}

			op, err := operationsProvider.NewReversibleOperation(
				manifest.WithName("basic-test"),
				manifest.WithManifestFile(filepath.Join(cwd, "manifests", "basic-test.yaml")),
				manifest.WithInitializedObjectsAssertion(assertionProvider.ObjectsExist(proxyService, proxyDeployment)),
				manifest.WithInitializedObjectsAssertion(assertionProvider.ObjectsNotExist(proxyService, proxyDeployment)),
			)
			Expect(err).NotTo(HaveOccurred())

			err = operator.ExecuteReversibleOperations(ctx, op)
			Expect(err).NotTo(HaveOccurred())
		})

		It("fails to produce running replicas", func() {
			// These are the resources that we expect to be dynamically provisioned when we run the test
			// The name and namespace of these objects is determined from the manifest file
			proxyDeployment := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "gloo-proxy-gw", Namespace: "default"}}
			proxyService := &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "gloo-proxy-gw", Namespace: "default"}}

			op, err := operationsProvider.NewReversibleOperation(
				manifest.WithName("basic-test"),
				manifest.WithManifestFile(filepath.Join(cwd, "manifests", "basic-test.yaml")),
				manifest.WithInitializedObjectsAssertion(
					assertionProvider.RunningReplicas(&core.ResourceRef{
						Name:      "gloo-proxy-gw",
						Namespace: "default",
					}, 1),
				),
				manifest.WithInitializedObjectsAssertion(assertionProvider.ObjectsNotExist(proxyService, proxyDeployment)),
			)
			Expect(err).NotTo(HaveOccurred())

			err = operator.ExecuteReversibleOperations(ctx, op)
			Expect(err).To(HaveOccurred(), "The gloo-proxy-gw that is deployed has the tag 1.0.0-ci which will fail to start")
		})

	})

})
