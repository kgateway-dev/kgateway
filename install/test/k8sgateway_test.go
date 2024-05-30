package test

import (
	"encoding/json"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/pkg/utils/kubeutils"
	"github.com/solo-io/gloo/projects/gateway2/pkg/api/gateway.gloo.solo.io/v1alpha1"
	"github.com/solo-io/gloo/projects/gateway2/pkg/api/gateway.gloo.solo.io/v1alpha1/kube"
	"github.com/solo-io/gloo/projects/gateway2/wellknown"
	"github.com/solo-io/gloo/projects/gloo/constants"
	"github.com/solo-io/gloo/test/gomega/matchers"
	. "github.com/solo-io/k8s-utils/manifesttestutils"
)

var _ = Describe("Kubernetes Gateway API integration", func() {
	var allTests = func(rendererTestCase renderTestCase) {
		var (
			testManifest TestManifest
			valuesArgs   []string
		)
		prepareMakefile := func(namespace string, values helmValues) {
			tm, err := rendererTestCase.renderer.RenderManifest(namespace, values)
			ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Failed to render manifest")
			testManifest = tm
		}

		BeforeEach(func() {
			valuesArgs = []string{}
		})

		JustBeforeEach(func() {
			prepareMakefile(namespace, helmValues{valuesArgs: valuesArgs})
		})
		When("kube gateway integration is enabled", func() {
			BeforeEach(func() {
				valuesArgs = append(valuesArgs, "kubeGateway.enabled=true")
			})

			It("relevant resources are rendered", func() {
				// make sure the env variable that enables the controller is set
				deployment := getDeployment(testManifest, namespace, kubeutils.GlooDeploymentName)
				Expect(deployment.Spec.Template.Spec.Containers).To(HaveLen(1), "should have exactly 1 container")
				expectEnvVarExists(deployment.Spec.Template.Spec.Containers[0], constants.GlooGatewayEnableK8sGwControllerEnv, "true")

				// make sure the GatewayClass and RBAC resources exist (note, since they are all cluster-scoped, they do not have a namespace)
				testManifest.ExpectUnstructured("GatewayClass", "", "gloo-gateway").NotTo(BeNil())

				testManifest.ExpectUnstructured("GatewayParameters", namespace, wellknown.DefaultGatewayParametersName).NotTo(BeNil())

				controlPlaneRbacName := fmt.Sprintf("glood-%s.%s", releaseName, namespace)
				testManifest.Expect("ClusterRole", "", controlPlaneRbacName).NotTo(BeNil())
				testManifest.Expect("ClusterRoleBinding", "", controlPlaneRbacName+"-binding").NotTo(BeNil())

				deployerRbacName := fmt.Sprintf("glood-%s-deploy.%s", releaseName, namespace)
				testManifest.Expect("ClusterRole", "", deployerRbacName).NotTo(BeNil())
				testManifest.Expect("ClusterRoleBinding", "", deployerRbacName+"-binding").NotTo(BeNil())
			})
			It("renders default GatewayParameters", func() {

				gwpUnstructured := testManifest.ExpectCustomResource("GatewayParameters", namespace, wellknown.DefaultGatewayParametersName)
				Expect(gwpUnstructured).NotTo(BeNil())

				var gwp v1alpha1.GatewayParameters
				b, err := gwpUnstructured.MarshalJSON()
				Expect(err).ToNot(HaveOccurred())
				err = json.Unmarshal(b, &gwp)
				Expect(err).ToNot(HaveOccurred())

				gwpKube := gwp.Spec.GetKube()
				Expect(gwpKube).ToNot(BeNil())

				Expect(gwpKube.GetDeployment().GetReplicas().GetValue()).To(Equal(uint32(1)))

				Expect(gwpKube.GetEnvoyContainer().GetImage().GetPullPolicy()).To(Equal(kube.Image_IfNotPresent))
				Expect(gwpKube.GetEnvoyContainer().GetImage().GetRegistry().GetValue()).To(Equal("quay.io/solo-io"))
				Expect(gwpKube.GetEnvoyContainer().GetImage().GetRepository().GetValue()).To(Equal("gloo-envoy-wrapper"))
				Expect(gwpKube.GetEnvoyContainer().GetImage().GetTag().GetValue()).To(Equal(version))

				Expect(gwpKube.GetIstio().GetEnabled().GetValue()).To(BeFalse())
				Expect(gwpKube.GetIstio().GetIstioContainer().GetImage().GetPullPolicy()).To(Equal(kube.Image_IfNotPresent))
				Expect(gwpKube.GetIstio().GetIstioContainer().GetImage().GetRegistry().GetValue()).To(Equal("docker.io/istio"))
				Expect(gwpKube.GetIstio().GetIstioContainer().GetImage().GetRepository().GetValue()).To(Equal("proxyv2"))
				Expect(gwpKube.GetIstio().GetIstioContainer().GetImage().GetTag().GetValue()).To(Equal("1.21.2"))
				Expect(gwpKube.GetIstio().GetIstioContainer().GetLogLevel().GetValue()).To(Equal("warning"))
				Expect(gwpKube.GetIstio().GetIstioDiscoveryAddress().GetValue()).To(Equal("istiod.istio-system.svc:15012"))
				Expect(gwpKube.GetIstio().GetIstioMetaMeshId().GetValue()).To(Equal("cluster.local"))
				Expect(gwpKube.GetIstio().GetIstioMetaClusterId().GetValue()).To(Equal("Kubernetes"))

				Expect(gwpKube.GetPodTemplate().GetExtraLabels()).To(matchers.ContainMapElements(map[string]string{"gloo": "kube-gateway"}))

				Expect(gwpKube.GetSdsContainer().GetImage().GetPullPolicy()).To(Equal(kube.Image_IfNotPresent))
				Expect(gwpKube.GetSdsContainer().GetImage().GetRegistry().GetValue()).To(Equal("quay.io/solo-io"))
				Expect(gwpKube.GetSdsContainer().GetImage().GetRepository().GetValue()).To(Equal("sds"))
				Expect(gwpKube.GetSdsContainer().GetImage().GetTag().GetValue()).To(Equal(version))
				Expect(gwpKube.GetSdsContainer().GetBootstrap().GetLogLevel().GetValue()).To(Equal("info"))

				Expect(gwpKube.GetService().GetType()).To(Equal(kube.Service_LoadBalancer))
			})

			When("overrides are set", func() {
				BeforeEach(func() {
					sdsVals := []string{"101Mi", "201m", "301Mi", "401m"}
					extraValuesArgs := []string{
						"global.image.variant=standard",
						"global.image.tag=global-override-tag",
						"global.image.registry=global-override-registry",
						"global.image.repository=global-override-repository",
						"global.image.pullPolicy=Never",
						"kubeGateway.gatewayParameters.glooGateway.image.tag=envoy-override-tag",
						"kubeGateway.gatewayParameters.glooGateway.image.registry=envoy-override-registry",
						"kubeGateway.gatewayParameters.glooGateway.image.repository=envoy-override-repository",
						"kubeGateway.gatewayParameters.glooGateway.image.pullPolicy=Always",
						"kubeGateway.gatewayParameters.glooGateway.proxyDeployment.replicas=5",
						"kubeGateway.gatewayParameters.glooGateway.service.type=ClusterIP",
						"global.glooMtls.enabled=false",
						"global.glooMtls.sds.image.tag=sds-override-tag",
						"global.glooMtls.sds.image.registry=sds-override-registry",
						"global.glooMtls.sds.image.repository=sds-override-repository",
						"global.glooMtls.sds.image.pullPolicy=Never",
						"global.glooMtls.sds.logLevel=debug",
						"global.glooMtls.sds.securityContext.runAsUser=999",
						fmt.Sprintf("global.glooMtls.sdsResources.requests.memory=%s", sdsVals[0]),
						fmt.Sprintf("global.glooMtls.sdsResources.requests.cpu=%s", sdsVals[1]),
						fmt.Sprintf("global.glooMtls.sdsResources.limits.memory=%s", sdsVals[2]),
						fmt.Sprintf("global.glooMtls.sdsResources.limits.cpu=%s", sdsVals[3]),
						"global.glooMtls.istioProxy.image.tag=istio-override-tag",
						"global.glooMtls.istioProxy.image.registry=istio-override-registry",
						"global.glooMtls.istioProxy.image.repository=istio-override-repository",
						"global.glooMtls.istioProxy.image.pullPolicy=Never",
						"global.glooMtls.istioProxy.logLevel=debug",
						"global.glooMtls.istioProxy.securityContext.runAsUser=888",
						"global.istioSDS.enabled=true",
					}
					valuesArgs = append(valuesArgs, extraValuesArgs...)
				})
				It("passes overrides to default GatewayParameters", func() {

					gwpUnstructured := testManifest.ExpectCustomResource("GatewayParameters", namespace, wellknown.DefaultGatewayParametersName)
					Expect(gwpUnstructured).NotTo(BeNil())

					var gwp v1alpha1.GatewayParameters
					b, err := gwpUnstructured.MarshalJSON()
					Expect(err).ToNot(HaveOccurred())
					err = json.Unmarshal(b, &gwp)
					Expect(err).ToNot(HaveOccurred())

					gwpKube := gwp.Spec.GetKube()
					Expect(gwpKube).ToNot(BeNil())

					Expect(gwpKube.GetDeployment().GetReplicas().GetValue()).To(Equal(uint32(5)))

					Expect(gwpKube.GetEnvoyContainer().GetImage().GetPullPolicy()).To(Equal(kube.Image_Always))
					Expect(gwpKube.GetEnvoyContainer().GetImage().GetRegistry().GetValue()).To(Equal("envoy-override-registry"))
					Expect(gwpKube.GetEnvoyContainer().GetImage().GetRepository().GetValue()).To(Equal("envoy-override-repository"))
					Expect(gwpKube.GetEnvoyContainer().GetImage().GetTag().GetValue()).To(Equal("envoy-override-tag"))

					Expect(gwpKube.GetIstio().GetEnabled().GetValue()).To(BeTrue())
					Expect(gwpKube.GetIstio().GetIstioContainer().GetImage().GetPullPolicy()).To(Equal(kube.Image_Never))
					Expect(gwpKube.GetIstio().GetIstioContainer().GetImage().GetRegistry().GetValue()).To(Equal("istio-override-registry"))
					Expect(gwpKube.GetIstio().GetIstioContainer().GetImage().GetRepository().GetValue()).To(Equal("istio-override-repository"))
					Expect(gwpKube.GetIstio().GetIstioContainer().GetImage().GetTag().GetValue()).To(Equal("istio-override-tag"))
					Expect(gwpKube.GetIstio().GetIstioContainer().GetLogLevel().GetValue()).To(Equal("debug"))
					Expect(gwpKube.GetIstio().GetIstioDiscoveryAddress().GetValue()).To(Equal("istiod.istio-system.svc:15012"))
					Expect(gwpKube.GetIstio().GetIstioMetaMeshId().GetValue()).To(Equal("cluster.local"))
					Expect(gwpKube.GetIstio().GetIstioMetaClusterId().GetValue()).To(Equal("Kubernetes"))

					Expect(gwpKube.GetPodTemplate().GetExtraLabels()).To(matchers.ContainMapElements(map[string]string{"gloo": "kube-gateway"}))

					Expect(gwpKube.GetSdsContainer().GetImage().GetPullPolicy()).To(Equal(kube.Image_Never))
					Expect(gwpKube.GetSdsContainer().GetImage().GetRegistry().GetValue()).To(Equal("sds-override-registry"))
					Expect(gwpKube.GetSdsContainer().GetImage().GetRepository().GetValue()).To(Equal("sds-override-repository"))
					Expect(gwpKube.GetSdsContainer().GetImage().GetTag().GetValue()).To(Equal("sds-override-tag"))
					Expect(gwpKube.GetSdsContainer().GetBootstrap().GetLogLevel().GetValue()).To(Equal("debug"))

					Expect(gwpKube.GetService().GetType()).To(Equal(kube.Service_ClusterIP))
				})
				// set overrides for all fields that get translated into gwp and make sure they make it in
				//
				// this should include image fields that are not valid in gwp (e.g. variant)
			})
		})

		When("kube gateway integration is disabled (default)", func() {
			BeforeEach(func() {
				valuesArgs = append(valuesArgs, "kubeGateway.enabled=false")
			})

			It("relevant resources are not rendered", func() {
				// the env variable that enables the controller should not be set
				deployment := getDeployment(testManifest, namespace, kubeutils.GlooDeploymentName)
				Expect(deployment.Spec.Template.Spec.Containers).To(HaveLen(1), "should have exactly 1 container")
				expectEnvVarDoesNotExist(deployment.Spec.Template.Spec.Containers[0], constants.GlooGatewayEnableK8sGwControllerEnv)

				// the RBAC resources should not be rendered
				testManifest.ExpectUnstructured("GatewayClass", "", "gloo-gateway").To(BeNil())

				testManifest.ExpectUnstructured("GatewayParameters", namespace, wellknown.DefaultGatewayParametersName).To(BeNil())

				controlPlaneRbacName := fmt.Sprintf("glood-%s.%s", releaseName, namespace)
				testManifest.Expect("ClusterRole", "", controlPlaneRbacName).To(BeNil())
				testManifest.Expect("ClusterRoleBinding", "", controlPlaneRbacName+"-binding").To(BeNil())

				deployerRbacName := fmt.Sprintf("glood-%s-deploy.%s", releaseName, namespace)
				testManifest.Expect("ClusterRole", "", deployerRbacName).To(BeNil())
				testManifest.Expect("ClusterRoleBinding", "", deployerRbacName+"-binding").To(BeNil())
			})
		})
	}
	runTests(allTests)
})
