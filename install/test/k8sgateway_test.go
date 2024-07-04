package test

import (
	"encoding/json"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/pkg/utils/kubeutils"
	"github.com/solo-io/gloo/projects/gateway2/api/v1alpha1"
	"github.com/solo-io/gloo/projects/gateway2/wellknown"
	"github.com/solo-io/gloo/projects/gloo/constants"
	"github.com/solo-io/gloo/test/gomega/matchers"
	. "github.com/solo-io/k8s-utils/manifesttestutils"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

var _ = Describe("Kubernetes Gateway API integration", func() {
	allTests := func(rendererTestCase renderTestCase) {
		var (
			testManifest TestManifest
			valuesArgs   []string
		)
		prepareHelmManifest := func(namespace string, values helmValues) {
			tm, err := rendererTestCase.renderer.RenderManifest(namespace, values)
			ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Failed to render manifest")
			testManifest = tm
		}

		BeforeEach(func() {
			valuesArgs = []string{}
		})

		JustBeforeEach(func() {
			prepareHelmManifest(namespace, helmValues{valuesArgs: valuesArgs})
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

				gwpKube := gwp.Spec.Kube
				Expect(gwpKube).ToNot(BeNil())

				Expect(*gwpKube.Deployment.Replicas).To(Equal(uint32(1)))

				Expect(*gwpKube.GetEnvoyContainer().GetImage().GetPullPolicy()).To(Equal(corev1.PullIfNotPresent))
				Expect(*gwpKube.GetEnvoyContainer().GetImage().GetRegistry()).To(Equal("quay.io/solo-io"))
				Expect(*gwpKube.GetEnvoyContainer().GetImage().GetRepository()).To(Equal("gloo-envoy-wrapper"))
				Expect(*gwpKube.GetEnvoyContainer().GetImage().GetTag()).To(Equal(version))
				Expect(*gwpKube.GetEnvoyContainer().GetSecurityContext().AllowPrivilegeEscalation).To(BeFalse())
				Expect(*gwpKube.GetEnvoyContainer().GetSecurityContext().ReadOnlyRootFilesystem).To(BeTrue())
				Expect(*gwpKube.GetEnvoyContainer().GetSecurityContext().RunAsNonRoot).To(BeTrue())
				Expect(*gwpKube.GetEnvoyContainer().GetSecurityContext().RunAsUser).To(Equal(int64(10101)))
				Expect(gwpKube.GetEnvoyContainer().GetSecurityContext().Capabilities.Drop).To(ContainElement(corev1.Capability("ALL")))
				Expect(gwpKube.GetEnvoyContainer().GetSecurityContext().Capabilities.Add).To(ContainElement(corev1.Capability("NET_BIND_SERVICE")))
				Expect(gwpKube.GetEnvoyContainer().GetResources()).To(BeNil())

				Expect(*gwpKube.GetIstio().GetIstioProxyContainer().GetImage().GetPullPolicy()).To(Equal(corev1.PullIfNotPresent))
				Expect(*gwpKube.GetIstio().GetIstioProxyContainer().GetImage().GetRegistry()).To(Equal("docker.io/istio"))
				Expect(*gwpKube.GetIstio().GetIstioProxyContainer().GetImage().GetRepository()).To(Equal("proxyv2"))
				Expect(*gwpKube.GetIstio().GetIstioProxyContainer().GetImage().GetTag()).To(Equal("1.22.0"))
				Expect(gwpKube.GetIstio().GetIstioProxyContainer().GetSecurityContext()).To(BeNil())
				Expect(*gwpKube.GetIstio().GetIstioProxyContainer().GetLogLevel()).To(Equal("warning"))
				Expect(*gwpKube.GetIstio().GetIstioProxyContainer().GetIstioDiscoveryAddress()).To(Equal("istiod.istio-system.svc:15012"))
				Expect(*gwpKube.GetIstio().GetIstioProxyContainer().GetIstioMetaMeshId()).To(Equal("cluster.local"))
				Expect(*gwpKube.GetIstio().GetIstioProxyContainer().GetIstioMetaClusterId()).To(Equal("Kubernetes"))

				Expect(gwpKube.GetPodTemplate().GetExtraLabels()).To(matchers.ContainMapElements(map[string]string{"gloo": "kube-gateway"}))

				Expect(*gwpKube.GetSdsContainer().GetImage().GetPullPolicy()).To(Equal(corev1.PullIfNotPresent))
				Expect(*gwpKube.GetSdsContainer().GetImage().GetRegistry()).To(Equal("quay.io/solo-io"))
				Expect(*gwpKube.GetSdsContainer().GetImage().GetRepository()).To(Equal("sds"))
				Expect(*gwpKube.GetSdsContainer().GetImage().GetTag()).To(Equal(version))
				Expect(gwpKube.GetSdsContainer().GetSecurityContext()).To(BeNil())
				Expect(*gwpKube.GetSdsContainer().GetBootstrap().GetLogLevel()).To(Equal("info"))
				Expect(gwpKube.GetSdsContainer().GetResources()).To(BeNil())

				Expect(*gwpKube.GetService().GetType()).To(Equal(corev1.ServiceTypeLoadBalancer))

				Expect(*gwpKube.GetStats().GetEnabled()).To(BeTrue())
				Expect(*gwpKube.GetStats().GetRoutePrefixRewrite()).To(Equal("/stats/prometheus"))
				Expect(*gwpKube.GetStats().GetEnableStatsRoute()).To(BeTrue())
				Expect(*gwpKube.GetStats().GetStatsRoutePrefixRewrite()).To(Equal("/stats"))

				Expect(*gwpKube.GetAiExtension().GetEnabled()).To(BeFalse())
				Expect(*gwpKube.GetAiExtension().GetImage().GetPullPolicy()).To(Equal(corev1.PullIfNotPresent))
				Expect(*gwpKube.GetAiExtension().GetImage().GetRegistry()).To(Equal("quay.io/solo-io"))
				Expect(*gwpKube.GetAiExtension().GetImage().GetRepository()).To(Equal("gloo-ai-extension"))
				Expect(*gwpKube.GetAiExtension().GetImage().GetTag()).To(Equal(version))
				Expect(gwpKube.GetAiExtension().GetSecurityContext()).To(BeNil())
				Expect(gwpKube.GetAiExtension().GetResources()).To(BeNil())
				Expect(gwpKube.GetAiExtension().GetPorts()).To(BeEmpty())
			})

			When("overrides are set", func() {
				var (
					sdsRequests   = corev1.ResourceList{"memory": resource.MustParse("101Mi"), "cpu": resource.MustParse("201m")}
					sdsLimits     = corev1.ResourceList{"memory": resource.MustParse("301Mi"), "cpu": resource.MustParse("401m")}
					envoyRequests = corev1.ResourceList{"memory": resource.MustParse("102Mi"), "cpu": resource.MustParse("202m")}
					envoyLimits   = corev1.ResourceList{"memory": resource.MustParse("302Mi"), "cpu": resource.MustParse("402m")}
				)
				BeforeEach(func() {
					extraValuesArgs := []string{
						"global.image.variant=standard",
						"global.image.tag=global-override-tag",
						"global.image.registry=global-override-registry",
						"global.image.repository=global-override-repository",
						"global.image.pullPolicy=Never",
						"kubeGateway.gatewayParameters.glooGateway.envoyContainer.image.tag=envoy-override-tag",
						"kubeGateway.gatewayParameters.glooGateway.envoyContainer.image.registry=envoy-override-registry",
						"kubeGateway.gatewayParameters.glooGateway.envoyContainer.image.repository=envoy-override-repository",
						"kubeGateway.gatewayParameters.glooGateway.envoyContainer.image.pullPolicy=Always",
						"kubeGateway.gatewayParameters.glooGateway.envoyContainer.securityContext.runAsNonRoot=null",
						"kubeGateway.gatewayParameters.glooGateway.envoyContainer.securityContext.runAsUser=777",
						fmt.Sprintf("kubeGateway.gatewayParameters.glooGateway.envoyContainer.resources.requests.memory=%s", envoyRequests["memory"].ToUnstructured()),
						fmt.Sprintf("kubeGateway.gatewayParameters.glooGateway.envoyContainer.resources.requests.cpu=%s", envoyRequests["cpu"].ToUnstructured()),
						fmt.Sprintf("kubeGateway.gatewayParameters.glooGateway.envoyContainer.resources.limits.memory=%s", envoyLimits["memory"].ToUnstructured()),
						fmt.Sprintf("kubeGateway.gatewayParameters.glooGateway.envoyContainer.resources.limits.cpu=%s", envoyLimits["cpu"].ToUnstructured()),
						"kubeGateway.gatewayParameters.glooGateway.proxyDeployment.replicas=5",
						"kubeGateway.gatewayParameters.glooGateway.service.type=ClusterIP",
						"kubeGateway.gatewayParameters.glooGateway.sdsContainer.image.tag=sds-override-tag",
						"kubeGateway.gatewayParameters.glooGateway.sdsContainer.image.registry=sds-override-registry",
						"kubeGateway.gatewayParameters.glooGateway.sdsContainer.image.repository=sds-override-repository",
						"kubeGateway.gatewayParameters.glooGateway.sdsContainer.image.pullPolicy=Never",
						"kubeGateway.gatewayParameters.glooGateway.sdsContainer.logLevel=debug",
						"kubeGateway.gatewayParameters.glooGateway.sdsContainer.securityContext.runAsNonRoot=null",
						"kubeGateway.gatewayParameters.glooGateway.sdsContainer.securityContext.runAsUser=999",
						fmt.Sprintf("kubeGateway.gatewayParameters.glooGateway.sdsContainer.sdsResources.requests.memory=%s", sdsRequests["memory"].ToUnstructured()),
						fmt.Sprintf("kubeGateway.gatewayParameters.glooGateway.sdsContainer.sdsResources.requests.cpu=%s", sdsRequests["cpu"].ToUnstructured()),
						fmt.Sprintf("kubeGateway.gatewayParameters.glooGateway.sdsContainer.sdsResources.limits.memory=%s", sdsLimits["memory"].ToUnstructured()),
						fmt.Sprintf("kubeGateway.gatewayParameters.glooGateway.sdsContainer.sdsResources.limits.cpu=%s", sdsLimits["cpu"].ToUnstructured()),
						"kubeGateway.gatewayParameters.glooGateway.istio.istioProxyContainer.image.tag=istio-override-tag",
						"kubeGateway.gatewayParameters.glooGateway.istio.istioProxyContainer.image.registry=istio-override-registry",
						"kubeGateway.gatewayParameters.glooGateway.istio.istioProxyContainer.image.repository=istio-override-repository",
						"kubeGateway.gatewayParameters.glooGateway.istio.istioProxyContainer.image.pullPolicy=Never",
						"kubeGateway.gatewayParameters.glooGateway.istio.istioProxyContainer.logLevel=debug",
						"kubeGateway.gatewayParameters.glooGateway.istio.istioProxyContainer.securityContext.runAsNonRoot=null",
						"kubeGateway.gatewayParameters.glooGateway.istio.istioProxyContainer.securityContext.runAsUser=888",
						"kubeGateway.gatewayParameters.glooGateway.stats.enabled=false",
						"kubeGateway.gatewayParameters.glooGateway.stats.routePrefixRewrite=/foo/bar",
						"kubeGateway.gatewayParameters.glooGateway.stats.enableStatsRoute=false",
						"kubeGateway.gatewayParameters.glooGateway.stats.statsRoutePrefixRewrite=/scooby/doo",
						"kubeGateway.gatewayParameters.glooGateway.aiExtension.image.tag=sds-override-tag",
						"kubeGateway.gatewayParameters.glooGateway.aiExtension.image.registry=sds-override-registry",
						"kubeGateway.gatewayParameters.glooGateway.aiExtension.image.repository=sds-override-repository",
						"kubeGateway.gatewayParameters.glooGateway.aiExtension.image.pullPolicy=Never",
						"kubeGateway.gatewayParameters.glooGateway.aiExtension.ports[0].name=port1",
						"kubeGateway.gatewayParameters.glooGateway.aiExtension.ports[0].containerPort=80",
						"global.istioIntegration.enabled=true",
					}
					valuesArgs = append(valuesArgs, extraValuesArgs...)
				})
				It("passes overrides to default GatewayParameters with Istio container", func() {
					gwpUnstructured := testManifest.ExpectCustomResource("GatewayParameters", namespace, wellknown.DefaultGatewayParametersName)
					Expect(gwpUnstructured).NotTo(BeNil())

					var gwp v1alpha1.GatewayParameters
					b, err := gwpUnstructured.MarshalJSON()
					Expect(err).ToNot(HaveOccurred())
					err = json.Unmarshal(b, &gwp)
					Expect(err).ToNot(HaveOccurred())

					gwpKube := gwp.Spec.Kube
					Expect(gwpKube).ToNot(BeNil())

					Expect(*gwpKube.GetDeployment().GetReplicas()).To(Equal(uint32(5)))

					Expect(*gwpKube.GetEnvoyContainer().GetImage().GetPullPolicy()).To(Equal(corev1.PullAlways))
					Expect(*gwpKube.GetEnvoyContainer().GetImage().GetRegistry()).To(Equal("envoy-override-registry"))
					Expect(*gwpKube.GetEnvoyContainer().GetImage().GetRepository()).To(Equal("envoy-override-repository"))
					Expect(*gwpKube.GetEnvoyContainer().GetImage().GetTag()).To(Equal("envoy-override-tag"))
					// We specified non-null override for runAsUser and null override for runAsNonRoot. We expect runAsUser to be overridden,
					// runAsNonRoot to be missing (nil) and the rest to be rendered from defaults.
					Expect(*gwpKube.GetEnvoyContainer().GetSecurityContext().AllowPrivilegeEscalation).To(BeFalse())
					Expect(*gwpKube.GetEnvoyContainer().GetSecurityContext().ReadOnlyRootFilesystem).To(BeTrue())
					Expect(gwpKube.GetEnvoyContainer().GetSecurityContext().RunAsNonRoot).To(BeNil()) // Not using getter here as it masks nil as false
					Expect(*gwpKube.GetEnvoyContainer().GetSecurityContext().RunAsUser).To(Equal(int64(777)))
					Expect(gwpKube.GetEnvoyContainer().GetSecurityContext().Capabilities.Drop).To(ContainElement(corev1.Capability("ALL")))
					Expect(gwpKube.GetEnvoyContainer().GetSecurityContext().Capabilities.Add).To(ContainElement(corev1.Capability("NET_BIND_SERVICE")))
					Expect(gwpKube.GetEnvoyContainer().GetResources().Requests).To(matchers.ContainMapElements(envoyRequests))
					Expect(gwpKube.GetEnvoyContainer().GetResources().Limits).To(matchers.ContainMapElements(envoyLimits))

					Expect(*gwpKube.GetIstio().GetIstioProxyContainer().GetImage().GetPullPolicy()).To(Equal(corev1.PullNever))
					Expect(*gwpKube.GetIstio().GetIstioProxyContainer().GetImage().GetRegistry()).To(Equal("istio-override-registry"))
					Expect(*gwpKube.GetIstio().GetIstioProxyContainer().GetImage().GetRepository()).To(Equal("istio-override-repository"))
					Expect(*gwpKube.GetIstio().GetIstioProxyContainer().GetImage().GetTag()).To(Equal("istio-override-tag"))
					// We specified non-null override for runAsUser and null override for runAsNonRoot. We expect runAsUser to be overridden,
					// runAsNonRoot to be missing (nil) and the rest to be nil since there are no defaults.
					Expect(gwpKube.GetIstio().GetIstioProxyContainer().GetSecurityContext().AllowPrivilegeEscalation).To(BeNil()) // Not using getter here as it masks nil as false
					Expect(gwpKube.GetIstio().GetIstioProxyContainer().GetSecurityContext().ReadOnlyRootFilesystem).To(BeNil())   // Not using getter here as it masks nil as false
					Expect(gwpKube.GetIstio().GetIstioProxyContainer().GetSecurityContext().RunAsNonRoot).To(BeNil())             // Not using getter here as it masks nil as false
					Expect(*gwpKube.GetIstio().GetIstioProxyContainer().GetSecurityContext().RunAsUser).To(Equal(int64(888)))
					Expect(gwpKube.GetIstio().GetIstioProxyContainer().GetSecurityContext().Capabilities).To(BeNil())
					Expect(*gwpKube.GetIstio().GetIstioProxyContainer().GetLogLevel()).To(Equal("debug"))
					Expect(*gwpKube.GetIstio().GetIstioProxyContainer().GetIstioDiscoveryAddress()).To(Equal("istiod.istio-system.svc:15012"))
					Expect(*gwpKube.GetIstio().GetIstioProxyContainer().GetIstioMetaMeshId()).To(Equal("cluster.local"))
					Expect(*gwpKube.GetIstio().GetIstioProxyContainer().GetIstioMetaClusterId()).To(Equal("Kubernetes"))

					Expect(gwpKube.GetPodTemplate().GetExtraLabels()).To(matchers.ContainMapElements(map[string]string{"gloo": "kube-gateway"}))

					Expect(*gwpKube.GetSdsContainer().GetImage().GetPullPolicy()).To(Equal(corev1.PullNever))
					Expect(*gwpKube.GetSdsContainer().GetImage().GetRegistry()).To(Equal("sds-override-registry"))
					Expect(*gwpKube.GetSdsContainer().GetImage().GetRepository()).To(Equal("sds-override-repository"))
					Expect(*gwpKube.GetSdsContainer().GetImage().GetTag()).To(Equal("sds-override-tag"))
					// We specified non-null override for runAsUser and null override for runAsNonRoot. We expect runAsUser to be overridden,
					// runAsNonRoot to be missing (nil) and the rest to be nil since there are no defaults.
					Expect(gwpKube.GetSdsContainer().GetSecurityContext().AllowPrivilegeEscalation).To(BeNil()) // Not using getter here as it masks nil as false
					Expect(gwpKube.GetSdsContainer().GetSecurityContext().ReadOnlyRootFilesystem).To(BeNil())   // Not using getter here as it masks nil as false
					Expect(gwpKube.GetSdsContainer().GetSecurityContext().RunAsNonRoot).To(BeNil())             // Not using getter here as it masks nil as false
					Expect(*gwpKube.GetSdsContainer().GetSecurityContext().RunAsUser).To(Equal(int64(999)))
					Expect(gwpKube.GetSdsContainer().GetSecurityContext().Capabilities).To(BeNil())
					Expect(*gwpKube.GetSdsContainer().GetBootstrap().GetLogLevel()).To(Equal("debug"))
					Expect(gwpKube.GetSdsContainer().GetResources().Requests).To(matchers.ContainMapElements(sdsRequests))
					Expect(gwpKube.GetSdsContainer().GetResources().Limits).To(matchers.ContainMapElements(sdsLimits))

					Expect(*gwpKube.GetService().GetType()).To(Equal(corev1.ServiceTypeClusterIP))

					Expect(*gwpKube.GetStats().GetEnabled()).To(BeFalse())
					Expect(*gwpKube.GetStats().GetRoutePrefixRewrite()).To(Equal("/foo/bar"))
					Expect(*gwpKube.GetStats().GetEnableStatsRoute()).To(BeFalse())
					Expect(*gwpKube.GetStats().GetStatsRoutePrefixRewrite()).To(Equal("/scooby/doo"))

					Expect(*gwpKube.GetAiExtension().GetImage().GetPullPolicy()).To(Equal(corev1.PullNever))
					Expect(*gwpKube.GetAiExtension().GetImage().GetRegistry()).To(Equal("sds-override-registry"))
					Expect(*gwpKube.GetAiExtension().GetImage().GetRepository()).To(Equal("sds-override-repository"))
					Expect(*gwpKube.GetAiExtension().GetImage().GetTag()).To(Equal("sds-override-tag"))
					Expect(gwpKube.GetAiExtension().GetPorts()).To(HaveLen(1))
					Expect(gwpKube.GetAiExtension().GetPorts()[0].Name).To(Equal("port1"))
					Expect(gwpKube.GetAiExtension().GetPorts()[0].ContainerPort).To(BeEquivalentTo(80))
				})
			})

			When("custom sidecars overrides are set", func() {
				BeforeEach(func() {
					sdsVals := []string{"101Mi", "201m", "301Mi", "401m"}
					extraValuesArgs := []string{
						"global.image.variant=standard",
						"global.image.tag=global-override-tag",
						"global.image.registry=global-override-registry",
						"global.image.repository=global-override-repository",
						"global.image.pullPolicy=Never",
						"kubeGateway.gatewayParameters.glooGateway.envoyContainer.image.tag=envoy-override-tag",
						"kubeGateway.gatewayParameters.glooGateway.envoyContainer.image.registry=envoy-override-registry",
						"kubeGateway.gatewayParameters.glooGateway.envoyContainer.image.repository=envoy-override-repository",
						"kubeGateway.gatewayParameters.glooGateway.envoyContainer.image.pullPolicy=Always",
						"kubeGateway.gatewayParameters.glooGateway.proxyDeployment.replicas=5",
						"kubeGateway.gatewayParameters.glooGateway.service.type=ClusterIP",
						"kubeGateway.gatewayParameters.glooGateway.sdsContainer.image.tag=sds-override-tag",
						"kubeGateway.gatewayParameters.glooGateway.sdsContainer.image.registry=sds-override-registry",
						"kubeGateway.gatewayParameters.glooGateway.sdsContainer.image.repository=sds-override-repository",
						"kubeGateway.gatewayParameters.glooGateway.sdsContainer.image.pullPolicy=Never",
						"kubeGateway.gatewayParameters.glooGateway.sdsContainer.logLevel=debug",
						"kubeGateway.gatewayParameters.glooGateway.sdsContainer.securityContext.runAsUser=999",
						fmt.Sprintf("kubeGateway.gatewayParameters.glooGateway.sdsContainer.sdsResources.requests.memory=%s", sdsVals[0]),
						fmt.Sprintf("kubeGateway.gatewayParameters.glooGateway.sdsContainer.sdsResources.requests.cpu=%s", sdsVals[1]),
						fmt.Sprintf("kubeGateway.gatewayParameters.glooGateway.sdsContainer.sdsResources.limits.memory=%s", sdsVals[2]),
						fmt.Sprintf("kubeGateway.gatewayParameters.glooGateway.sdsContainer.sdsResources.limits.cpu=%s", sdsVals[3]),
						"kubeGateway.gatewayParameters.glooGateway.istio.customSidecars[0].name=custom-sidecar",
						"global.istioIntegration.enabled=true",
					}
					valuesArgs = append(valuesArgs, extraValuesArgs...)
				})
				It("passes overrides to default GatewayParameters with custom sidecar", func() {
					gwpUnstructured := testManifest.ExpectCustomResource("GatewayParameters", namespace, wellknown.DefaultGatewayParametersName)
					Expect(gwpUnstructured).NotTo(BeNil())

					var gwp v1alpha1.GatewayParameters
					b, err := gwpUnstructured.MarshalJSON()
					Expect(err).ToNot(HaveOccurred())
					err = json.Unmarshal(b, &gwp)
					Expect(err).ToNot(HaveOccurred())

					gwpKube := gwp.Spec.Kube
					Expect(gwpKube).ToNot(BeNil())

					Expect(*gwpKube.GetDeployment().GetReplicas()).To(Equal(uint32(5)))

					Expect(*gwpKube.GetEnvoyContainer().GetImage().GetPullPolicy()).To(Equal(corev1.PullAlways))
					Expect(*gwpKube.GetEnvoyContainer().GetImage().GetRegistry()).To(Equal("envoy-override-registry"))
					Expect(*gwpKube.GetEnvoyContainer().GetImage().GetRepository()).To(Equal("envoy-override-repository"))
					Expect(*gwpKube.GetEnvoyContainer().GetImage().GetTag()).To(Equal("envoy-override-tag"))

					Expect(gwpKube.GetIstio().GetCustomSidecars()[0].Name).To(Equal("custom-sidecar"))
					Expect(gwpKube.GetPodTemplate().GetExtraLabels()).To(matchers.ContainMapElements(map[string]string{"gloo": "kube-gateway"}))

					Expect(*gwpKube.GetSdsContainer().GetImage().GetPullPolicy()).To(Equal(corev1.PullNever))
					Expect(*gwpKube.GetSdsContainer().GetImage().GetRegistry()).To(Equal("sds-override-registry"))
					Expect(*gwpKube.GetSdsContainer().GetImage().GetRepository()).To(Equal("sds-override-repository"))
					Expect(*gwpKube.GetSdsContainer().GetImage().GetTag()).To(Equal("sds-override-tag"))
					Expect(*gwpKube.GetSdsContainer().GetBootstrap().GetLogLevel()).To(Equal("debug"))

					Expect(*gwpKube.GetService().GetType()).To(Equal(corev1.ServiceTypeClusterIP))
				})
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
