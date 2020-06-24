package test

import (
	"encoding/json"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/solo-io/go-utils/manifesttestutils"
	v1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

var _ = Describe("Deployment Privileges Test", func() {
	var allTests = func(testCase renderTestCase) {
		Context("Gloo", func() {
			Context("all cluster-scoped deployments", func() {
				It("is running all deployments with non root user permissions by default", func() {
					testManifest, err := testCase.renderer.RenderManifest(namespace, helmValues{})
					Expect(err).NotTo(HaveOccurred(), "Should be able to render the manifest in the priviliges unit test")

					expectNonRoot(testManifest)
				})

			})
		})
	}
	runTests(allTests)
})

func expectNonRoot(testManifest manifesttestutils.TestManifest) {
	deployments := testManifest.SelectResources(func(resource *unstructured.Unstructured) bool {
		return resource.GetKind() == "Deployment"
	})

	Expect(deployments.NumResources()).NotTo(BeZero())

	deployments.ExpectAll(func(resource *unstructured.Unstructured) {
		rawDeploy, err := resource.MarshalJSON()
		Expect(err).NotTo(HaveOccurred())

		deploy := v1.Deployment{}
		err = json.Unmarshal(rawDeploy, &deploy)
		Expect(err).NotTo(HaveOccurred())

		Expect(deploy.Spec.Template).NotTo(BeNil())

		podLevelSecurity := false
		// Check for root at the pod level
		if deploy.Spec.Template.Spec.SecurityContext != nil {
			Expect(deploy.Spec.Template.Spec.SecurityContext.RunAsUser).NotTo(Equal(0))
			podLevelSecurity = true
		}

		// Check for root at the container level
		for _, container := range deploy.Spec.Template.Spec.Containers {
			if !podLevelSecurity {
				// If pod level security is not set, containers need to explicitly not be run as root
				Expect(container.SecurityContext).NotTo(BeNil())
				Expect(container.SecurityContext.RunAsUser).NotTo(Equal(0))
			} else if container.SecurityContext != nil {
				// If podLevel security is set to non-root, make sure containers don't override it:
				Expect(container.SecurityContext.RunAsUser).NotTo(Equal(0))
			}
		}
	})
}
