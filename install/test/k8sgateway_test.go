package test

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/pkg/utils/kubeutils"
	"github.com/solo-io/gloo/projects/gloo/constants"
	. "github.com/solo-io/k8s-utils/manifesttestutils"
)

var _ = Describe("Kubernetes Gateway API integration", func() {
	var allTests = func(rendererTestCase renderTestCase) {
		var (
			testManifest TestManifest
		)
		prepareMakefile := func(namespace string, values helmValues) {
			tm, err := rendererTestCase.renderer.RenderManifest(namespace, values)
			ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Failed to render manifest")
			testManifest = tm
		}

		It("when k8sgateway is enabled, env var and resources should be rendered", func() {
			prepareMakefile(namespace, helmValues{
				valuesArgs: []string{
					"kubeGateway.enabled=true",
				},
			})

			// make sure the env variable that enables the controller is set
			deployment := getDeployment(testManifest, namespace, kubeutils.GlooDeploymentName)
			Expect(deployment.Spec.Template.Spec.Containers).To(HaveLen(1), "should have exactly 1 container")
			expectEnvVarExists(deployment.Spec.Template.Spec.Containers[0], constants.GlooGatewayEnableK8sGwControllerEnv, "true")

			// make sure the GatewayClass and RBAC resources exist (note, since they are all cluster-scoped, they do not have a namespace)
			testManifest.ExpectUnstructured("GatewayClass", "", "gloo-gateway").NotTo(BeNil())

			controlPlaneRbacName := fmt.Sprintf("glood-%s.%s", releaseName, namespace)
			testManifest.Expect("ClusterRole", "", controlPlaneRbacName).NotTo(BeNil())
			testManifest.Expect("ClusterRoleBinding", "", controlPlaneRbacName+"-binding").NotTo(BeNil())

			deployerRbacName := fmt.Sprintf("glood-%s-deploy.%s", releaseName, namespace)
			testManifest.Expect("ClusterRole", "", deployerRbacName).NotTo(BeNil())
			testManifest.Expect("ClusterRoleBinding", "", deployerRbacName+"-binding").NotTo(BeNil())
		})

		It("when k8sgateway is disabled, env var and resources should not be rendered", func() {
			prepareMakefile(namespace, helmValues{
				valuesArgs: []string{
					"kubeGateway.enabled=false",
				},
			})

			// the env variable that enables the controller should not be set
			deployment := getDeployment(testManifest, namespace, kubeutils.GlooDeploymentName)
			Expect(deployment.Spec.Template.Spec.Containers).To(HaveLen(1), "should have exactly 1 container")
			expectEnvVarDoesNotExist(deployment.Spec.Template.Spec.Containers[0], constants.GlooGatewayEnableK8sGwControllerEnv)

			// the RBAC resources should not be rendered
			testManifest.ExpectUnstructured("GatewayClass", "", "gloo-gateway").To(BeNil())

			controlPlaneRbacName := fmt.Sprintf("glood-%s.%s", releaseName, namespace)
			testManifest.Expect("ClusterRole", "", controlPlaneRbacName).To(BeNil())
			testManifest.Expect("ClusterRoleBinding", "", controlPlaneRbacName+"-binding").To(BeNil())

			deployerRbacName := fmt.Sprintf("glood-%s-deploy.%s", releaseName, namespace)
			testManifest.Expect("ClusterRole", "", deployerRbacName).To(BeNil())
			testManifest.Expect("ClusterRoleBinding", "", deployerRbacName+"-binding").To(BeNil())
		})
	}
	runTests(allTests)
})
