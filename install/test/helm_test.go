package test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"
	"io/ioutil"
	"reflect"
	"strings"

	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes/empty"
	"github.com/golang/protobuf/ptypes/wrappers"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	values "github.com/solo-io/gloo/install/helm/gloo/generate"
	gwv1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gateway/pkg/defaults"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/test/matchers"
	"github.com/solo-io/k8s-utils/installutils/kuberesource"
	"github.com/solo-io/k8s-utils/manifesttestutils"
	. "github.com/solo-io/k8s-utils/manifesttestutils"
	"github.com/solo-io/reporting-client/pkg/client"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	skprotoutils "github.com/solo-io/solo-kit/pkg/utils/protoutils"
	test_matchers "github.com/solo-io/solo-kit/test/matchers"
	"helm.sh/helm/v3/pkg/releaseutil"
	appsv1 "k8s.io/api/apps/v1"
	jobsv1 "k8s.io/api/batch/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/utils/pointer"
)

func GetPodNamespaceStats() v1.EnvVar {
	return v1.EnvVar{
		Name:  "START_STATS_SERVER",
		Value: "true",
	}
}

func GetPodNameEnvVar() v1.EnvVar {
	return v1.EnvVar{
		Name: "POD_NAME",
		ValueFrom: &v1.EnvVarSource{
			FieldRef: &v1.ObjectFieldSelector{
				FieldPath: "metadata.name",
			},
		},
	}
}

func GetLogLevelEnvVar() v1.EnvVar {
	return v1.EnvVar{
		Name:  "LOG_LEVEL",
		Value: "debug",
	}
}

func GetTestExtraEnvVar() v1.EnvVar {
	return v1.EnvVar{
		Name:  "TEST_EXTRA_ENV_VAR",
		Value: "test",
	}
}

func ConvertKubeResource(unst *unstructured.Unstructured, res resources.Resource) {
	byt, err := unst.MarshalJSON()
	Expect(err).NotTo(HaveOccurred())

	err = skprotoutils.UnmarshalResource(byt, res)
	Expect(err).NotTo(HaveOccurred())
}

var _ = Describe("Helm Test", func() {
	Context("CRD creation", func() {
		It("has not diverged between Helm 2 and 3", func() {
			release, err := BuildHelm3Release(chartDir, namespace, helmValues{})
			Expect(err).NotTo(HaveOccurred())

			foundCrdFile := false
			for _, f := range release.Chart.Raw {
				if f.Name == "templates/100-crds.yaml" {
					foundCrdFile = true
					legacyCrdTemplateData := string(f.Data)

					for _, helm3Crd := range release.Chart.CRDs() {
						Expect(legacyCrdTemplateData).To(ContainSubstring(string(helm3Crd.Data)), "CRD "+helm3Crd.Name+" does not match legacy duplicate")
					}

					legacyCrdTemplate, err := template.New("").Parse(legacyCrdTemplateData)
					Expect(err).NotTo(HaveOccurred())

					renderedLegacyCrds := new(bytes.Buffer)
					err = legacyCrdTemplate.Execute(renderedLegacyCrds, map[string]interface{}{
						"Values": map[string]interface{}{
							"crds": map[string]interface{}{
								"create": true,
							},
						},
					})
					Expect(err).NotTo(HaveOccurred(), "Should be able to render the legacy CRDs")
					Expect(len(releaseutil.SplitManifests(renderedLegacyCrds.String()))).To(Equal(len(release.Chart.CRDs())), "Should have the same number of CRDs")
				}
			}
			Expect(foundCrdFile).To(BeTrue(), "Should have found the legacy CRD file")
		})
	})

	var allTests = func(rendererTestCase renderTestCase) {
		var (
			glooPorts = []v1.ContainerPort{
				{Name: "grpc-xds", ContainerPort: 9977, Protocol: "TCP"},
				{Name: "rest-xds", ContainerPort: 9976, Protocol: "TCP"},
				{Name: "grpc-validation", ContainerPort: 9988, Protocol: "TCP"},
				{Name: "wasm-cache", ContainerPort: 9979, Protocol: "TCP"},
			}
			selector         map[string]string
			testManifest     TestManifest
			statsAnnotations map[string]string
		)

		Describe(rendererTestCase.rendererName, func() {
			// each entry in valuesArgs should look like `path.to.helm.field=value`
			prepareMakefile := func(namespace string, values helmValues) {
				tm, err := rendererTestCase.renderer.RenderManifest(namespace, values)
				Expect(err).NotTo(HaveOccurred(), "Failed to render manifest")
				testManifest = tm
			}

			// helper for passing a values file
			prepareMakefileFromValuesFile := func(valuesFile string) {
				prepareMakefile(namespace, helmValues{
					valuesFile: valuesFile,
					valuesArgs: []string{
						"gatewayProxies.gatewayProxy.service.extraAnnotations.test=test",
					},
				})
			}
			BeforeEach(func() {
				statsAnnotations = map[string]string{
					"prometheus.io/path":   "/metrics",
					"prometheus.io/port":   "9091",
					"prometheus.io/scrape": "true",
				}
			})

			It("should have all resources marked with a namespace", func() {
				prepareMakefile(namespace, helmValues{})

				nonNamespacedKinds := sets.NewString(
					"ClusterRole",
					"ClusterRoleBinding",
					"ValidatingWebhookConfiguration",
				)

				// all namespaced resources should have a namespace set on them
				// this tests that nothing winds up in the default kube namespace from your config when you install (unless that's what you intended)
				testManifest.SelectResources(func(resource *unstructured.Unstructured) bool {
					return !nonNamespacedKinds.Has(resource.GetKind())
				}).ExpectAll(func(resource *unstructured.Unstructured) {
					ExpectWithOffset(1, resource.GetNamespace()).NotTo(BeEmpty(), fmt.Sprintf("Resource %+v does not have a namespace", resource))
				})
			})

			It("Should have no duplicate resources", func() {
				prepareMakefile(namespace, helmValues{})

				var resources []*unstructured.Unstructured
				// This piece of work is the simplest way to directly access the unstructured resources list backing a testManifest struct
				// without updating go-utils and adding a direct access function to the TestManifest interface.
				// We aren't doing that because updating gloo's go-utils dependency is its own task to be addressed some other time.
				testManifest.SelectResources(func(unstructured *unstructured.Unstructured) bool {
					resources = append(resources, unstructured)
					return true
				})

				for idx1, resource1 := range resources {
					for idx2, resource2 := range resources {
						if idx1 == idx2 {
							continue
						}
						Expect(constructResourceID(resource1)).NotTo(Equal(constructResourceID(resource2)))
					}
				}
			})

			Context("stats server settings", func() {
				var (
					normalPromAnnotations = map[string]string{
						"prometheus.io/path":   "/metrics",
						"prometheus.io/port":   "9091",
						"prometheus.io/scrape": "true",
					}

					gatewayProxyDeploymentPromAnnotations = map[string]string{
						"prometheus.io/path":   "/metrics",
						"prometheus.io/port":   "8081",
						"prometheus.io/scrape": "true",
					}
				)

				It("should be able to configure a stats server by default on all relevant deployments", func() {
					prepareMakefile(namespace, helmValues{})

					testManifest.SelectResources(func(resource *unstructured.Unstructured) bool {
						return resource.GetKind() == "Deployment"
					}).ExpectAll(func(deployment *unstructured.Unstructured) {
						deploymentObject, err := kuberesource.ConvertUnstructured(deployment)
						ExpectWithOffset(1, err).NotTo(HaveOccurred(), fmt.Sprintf("Deployment %+v should be able to convert from unstructured", deployment))
						structuredDeployment, ok := deploymentObject.(*appsv1.Deployment)
						ExpectWithOffset(1, ok).To(BeTrue(), fmt.Sprintf("Deployment %+v should be able to cast to a structured deployment", deployment))

						promAnnotations := normalPromAnnotations
						if structuredDeployment.GetName() == "gateway-proxy" {
							promAnnotations = gatewayProxyDeploymentPromAnnotations
						}

						deploymentAnnotations := structuredDeployment.Spec.Template.ObjectMeta.Annotations
						for annotation, value := range promAnnotations {
							ExpectWithOffset(1, deploymentAnnotations[annotation]).To(Equal(value), fmt.Sprintf("Annotation %s should be set to %s on deployment %+v", deployment, annotation, value))
						}

						if structuredDeployment.GetName() != "gateway-proxy" {
							for _, container := range structuredDeployment.Spec.Template.Spec.Containers {
								foundExpected := false
								for _, envVar := range container.Env {
									if envVar.Name == "START_STATS_SERVER" {
										foundExpected = true
										ExpectWithOffset(1, envVar.Value).To(Equal("true"), fmt.Sprintf("Should have the START_STATS_SERVER env var set to 'true' on deployment %+v", deployment))
									}
								}

								ExpectWithOffset(1, foundExpected).To(BeTrue(), fmt.Sprintf("Should have found the START_STATS_SERVER env var on deployment %+v", deployment))
							}
						}
					})
				})

				It("should be able to set custom labels for pods", func() {
					// This test expects ALL pods to be capable of setting custom labels unless exceptions are added
					// here, which means that this test will fail if new deployments are added to the helm chart without
					// custom labeling, unless those deployments aren't enabled by default (like the accessLogger).
					// Note: test panics if values-template.yaml doesn't contain at least an empty definition
					// of each label object that's modified here.
					// Note note: Update number in final expectation if you add new labels here.
					prepareMakefile(namespace, helmValues{
						valuesArgs: []string{
							"gateway.deployment.extraGatewayLabels.foo=bar",
							"gloo.deployment.extraGlooLabels.foo=bar",
							"discovery.deployment.extraDiscoveryLabels.foo=bar",
							"gatewayProxies.gatewayProxy.podTemplate.extraGatewayProxyLabels.foo=bar",
							"accessLogger.enabled=true", // required to test accessLogger
							"accessLogger.extraAccessLoggerLabels.foo=bar",
							"ingress.deployment.extraIngressLabels.foo=bar",
							"ingress.enabled=true", // required to test Ingress Proxy, but not Ingress.
							"ingressProxy.deployment.extraIngressProxyLabels.foo=bar",
							"settings.integrations.knative.enabled=true", // required to test cluster ingress proxy and knative labels.
							"settings.integrations.knative.extraKnativeExternalLabels.foo=bar",
							"settings.integrations.knative.extraKnativeInternalLabels.foo=bar",
							"gloo.deployment.extraGlooAnnotations.foo=bar",
							"ingress.deployment.extraIngressAnnotations.foo=bar",
							"settings.integrations.knative.extraKnativeExternalAnnotations.foo=bar",
							"settings.integrations.knative.extraKnativeInternalAnnotations.foo=bar",
							"discovery.deployment.extraDiscoveryAnnotations.foo=bar",
							"gateway.deployment.extraGatewayAnnotations.foo=bar",
							"accessLogger.extraAccessLoggerAnnotations.foo=bar",
							"settings.integrations.knative.proxy.extraClusterIngressProxyAnnotations.foo=bar",
						},
					})

					var resourcesTested = 0
					testManifest.SelectResources(func(resource *unstructured.Unstructured) bool {
						return resource.GetKind() == "Deployment"
					}).ExpectAll(func(deployment *unstructured.Unstructured) {
						deploymentObject, err := kuberesource.ConvertUnstructured(deployment)
						ExpectWithOffset(1, err).NotTo(HaveOccurred(), fmt.Sprintf("Deployment %+v should be able to convert from unstructured", deployment))
						structuredDeployment, ok := deploymentObject.(*appsv1.Deployment)
						ExpectWithOffset(1, ok).To(BeTrue(), fmt.Sprintf("Deployment %+v should be able to cast to a structured deployment", deployment))

						deploymentLabels := structuredDeployment.Spec.Template.Labels
						var foundTestValue = false
						for label, value := range deploymentLabels {
							if label == "foo" {
								ExpectWithOffset(1, value).To(Equal("bar"), fmt.Sprintf("Deployment %s expected test label to have"+
									" value bar. Found value %s", deployment.GetName(), value))
								foundTestValue = true
							}
						}
						ExpectWithOffset(1, foundTestValue).To(Equal(true), fmt.Sprintf("Coundn't find test label 'foo' in deployment %s", deployment.GetName()))
						resourcesTested += 1
					})
					// Is there an elegant way to parameterized the expected number of deployments based on the valueArgs?
					Expect(resourcesTested).To(Equal(9), "Tested %d resources when we were expecting 9."+
						" Was a new pod added, or is an existing pod no longer being generated?", resourcesTested)
				})

				// due to the version requirements for rendering knative-related templates, the cluster ingress proxy
				// template is mutually exclusive to the other knative templates, and needs to be tested separately.
				It("should be able to set custom labels for cluster ingress proxy pod", func() {
					prepareMakefile(namespace, helmValues{
						valuesArgs: []string{
							"settings.integrations.knative.enabled=true",
							"settings.integrations.knative.version=0.7.0",
							"settings.integrations.knative.proxy.extraClusterIngressProxyLabels.foo=bar",
						},
					})

					var resourcesTested = 0
					testManifest.SelectResources(func(resource *unstructured.Unstructured) bool {
						return resource.GetKind() == "Deployment"
					}).ExpectAll(func(deployment *unstructured.Unstructured) {
						deploymentObject, err := kuberesource.ConvertUnstructured(deployment)
						ExpectWithOffset(1, err).NotTo(HaveOccurred(), fmt.Sprintf("Deployment %+v should be able to convert from unstructured", deployment))
						structuredDeployment, ok := deploymentObject.(*appsv1.Deployment)
						ExpectWithOffset(1, ok).To(BeTrue(), fmt.Sprintf("Deployment %+v should be able to cast to a structured deployment", deployment))

						deploymentLabels := structuredDeployment.Spec.Template.Labels
						if structuredDeployment.Name != "clusteringress-proxy" {
							return
						}
						var foundTestValue = false
						for label, value := range deploymentLabels {
							if label == "foo" {
								ExpectWithOffset(1, value).To(Equal("bar"), fmt.Sprintf("Deployment %s expected test label to have"+
									" value bar. Found value %s", deployment.GetName(), value))
								foundTestValue = true
							}
						}
						ExpectWithOffset(1, foundTestValue).To(Equal(true), fmt.Sprintf("Coundn't find test label 'foo' in deployment %s", deployment.GetName()))
						resourcesTested += 1
					})
					// Is there an elegant way to parameterized the expected number of deployments based on the valueArgs?
					Expect(resourcesTested).To(Equal(1), "Tested %d resources when we were expecting 1."+
						"What happened to the clusteringress-proxy deployment?", resourcesTested)
				})

				It("should set route prefix_rewrite in clusteringress-envoy-config from global.glooStats", func() {
					prepareMakefile(namespace, helmValues{
						valuesArgs: []string{
							"settings.integrations.knative.enabled=true",
							"settings.integrations.knative.version=0.7.0",
							"settings.integrations.knative.proxy.stats=true",
							"global.glooStats.routePrefixRewrite=/stats?format=json"},
					})

					testManifest.SelectResources(func(resource *unstructured.Unstructured) bool {
						return resource.GetKind() == "ConfigMap"
					}).ExpectAll(func(configMap *unstructured.Unstructured) {
						configMapObject, err := kuberesource.ConvertUnstructured(configMap)
						ExpectWithOffset(1, err).NotTo(HaveOccurred(), fmt.Sprintf("ConfigMap %+v should be able to convert from unstructured", configMap))
						structuredConfigMap, ok := configMapObject.(*v1.ConfigMap)
						ExpectWithOffset(1, ok).To(BeTrue(), fmt.Sprintf("ConfigMap %+v should be able to cast to a structured config map", configMap))

						if structuredConfigMap.GetName() == "clusteringress-envoy-config" {
							expectedPrefixRewrite := "prefix_rewrite: /stats?format=json"
							ExpectWithOffset(1, structuredConfigMap.Data["envoy.yaml"]).To(ContainSubstring(expectedPrefixRewrite))
						}
					})
				})

				It("should set route prefix_rewrite in knative proxy configs from global.glooStats", func() {
					prepareMakefile(namespace, helmValues{
						valuesArgs: []string{
							"settings.integrations.knative.enabled=true",
							"settings.integrations.knative.version=0.8.0",
							"settings.integrations.knative.proxy.stats=true",
							"global.glooStats.routePrefixRewrite=/stats?format=json"},
					})

					testManifest.SelectResources(func(resource *unstructured.Unstructured) bool {
						return resource.GetKind() == "ConfigMap"
					}).ExpectAll(func(configMap *unstructured.Unstructured) {
						configMapObject, err := kuberesource.ConvertUnstructured(configMap)
						ExpectWithOffset(1, err).NotTo(HaveOccurred(), fmt.Sprintf("ConfigMap %+v should be able to convert from unstructured", configMap))
						structuredConfigMap, ok := configMapObject.(*v1.ConfigMap)
						ExpectWithOffset(1, ok).To(BeTrue(), fmt.Sprintf("ConfigMap %+v should be able to cast to a structured config map", configMap))

						if structuredConfigMap.GetName() == "knative-internal-proxy-config" ||
							structuredConfigMap.GetName() == "knative-external-proxy-config" {
							expectedPrefixRewrite := "prefix_rewrite: /stats?format=json"
							ExpectWithOffset(1, structuredConfigMap.Data["envoy.yaml"]).To(ContainSubstring(expectedPrefixRewrite))
						}
					})
				})

				It("should be able to set consul config values", func() {
					settings := makeUnstructureFromTemplateFile("fixtures/settings/consul_config_values.yaml", namespace)
					prepareMakefileFromValuesFile("values/val_consul_test_inputs.yaml")
					testManifest.ExpectUnstructured(settings.GetKind(), settings.GetNamespace(), settings.GetName()).To(BeEquivalentTo(settings))
				})

				It("should be able to set consul config upstream discovery values", func() {
					settings := makeUnstructureFromTemplateFile("fixtures/settings/consul_config_upstream_discovery.yaml", namespace)
					prepareMakefileFromValuesFile("values/val_consul_discovery_test_inputs.yaml")
					testManifest.ExpectUnstructured(settings.GetKind(), settings.GetNamespace(), settings.GetName()).To(BeEquivalentTo(settings))
				})

				It("should be able to override global defaults", func() {
					prepareMakefile(namespace, helmValues{
						valuesArgs: []string{"discovery.deployment.stats.enabled=true", "global.glooStats.enabled=false"},
					})

					// assert that discovery has stats enabled and gloo has stats disabled
					testManifest.SelectResources(func(resource *unstructured.Unstructured) bool {
						return resource.GetKind() == "Deployment" &&
							(resource.GetName() == "gloo" || resource.GetName() == "discovery")
					}).ExpectAll(func(deployment *unstructured.Unstructured) {
						deploymentObject, err := kuberesource.ConvertUnstructured(deployment)
						ExpectWithOffset(1, err).NotTo(HaveOccurred(), fmt.Sprintf("Deployment %+v should be able to convert from unstructured", deployment))
						structuredDeployment, ok := deploymentObject.(*appsv1.Deployment)
						ExpectWithOffset(1, ok).To(BeTrue(), fmt.Sprintf("Deployment %+v should be able to cast to a structured deployment", deployment))

						if structuredDeployment.GetName() == "gloo" {
							ExpectWithOffset(1, structuredDeployment.Spec.Template.ObjectMeta.Annotations).To(BeEmpty(), fmt.Sprintf("No annotations should be present on deployment %+v", structuredDeployment))
						} else if structuredDeployment.GetName() == "discovery" {
							for annotation, value := range normalPromAnnotations {
								ExpectWithOffset(1, structuredDeployment.Spec.Template.ObjectMeta.Annotations[annotation]).To(Equal(value), fmt.Sprintf("Annotation %s should be set to %s on deployment %+v", deployment, annotation, value))
							}
						} else {
							Fail(fmt.Sprintf("Unexpected deployment found: %+v", structuredDeployment))
						}
					})
				})
			})

			Context("gloo mtls settings", func() {
				var (
					glooMtlsSecretVolume = v1.Volume{
						Name: "gloo-mtls-certs",
						VolumeSource: v1.VolumeSource{
							Secret: &v1.SecretVolumeSource{
								SecretName:  "gloo-mtls-certs",
								Items:       nil,
								DefaultMode: proto.Int(420),
							},
						},
					}

					haveEnvoySidecar = func(containers []v1.Container) bool {
						for _, c := range containers {
							if c.Name == "envoy-sidecar" {
								return true
							}
						}
						return false
					}

					haveSdsSidecar = func(containers []v1.Container) bool {
						for _, c := range containers {
							if c.Name == "sds" {
								return true
							}
						}
						return false
					}
				)

				It("should put the secret volume in the Gloo and Gateway-Proxy Deployment and add a sidecar in the Gloo Deployment", func() {
					prepareMakefile(namespace, helmValues{
						valuesArgs: []string{"global.glooMtls.enabled=true"},
					})

					foundGlooMtlsCertgenJob := false
					testManifest.SelectResources(func(resource *unstructured.Unstructured) bool {
						return resource.GetKind() == "Job"
					}).ExpectAll(func(job *unstructured.Unstructured) {
						jobObject, err := kuberesource.ConvertUnstructured(job)
						Expect(err).NotTo(HaveOccurred(), fmt.Sprintf("Job %+v should be able to convert from unstructured", job))
						structuredDeployment, ok := jobObject.(*jobsv1.Job)
						Expect(ok).To(BeTrue(), fmt.Sprintf("Job %+v should be able to cast to a structured job", job))

						if structuredDeployment.GetName() == "gloo-mtls-certgen" {
							foundGlooMtlsCertgenJob = true
						}
					})
					Expect(foundGlooMtlsCertgenJob).To(BeTrue(), "Did not find the gloo-mtls-certgen job")

					testManifest.SelectResources(func(resource *unstructured.Unstructured) bool {
						return resource.GetKind() == "Deployment"
					}).ExpectAll(func(deployment *unstructured.Unstructured) {
						deploymentObject, err := kuberesource.ConvertUnstructured(deployment)
						Expect(err).NotTo(HaveOccurred(), fmt.Sprintf("Deployment %+v should be able to convert from unstructured", deployment))
						structuredDeployment, ok := deploymentObject.(*appsv1.Deployment)
						Expect(ok).To(BeTrue(), fmt.Sprintf("Deployment %+v should be able to cast to a structured deployment", deployment))

						if structuredDeployment.GetName() == "gloo" {
							Ω(haveEnvoySidecar(structuredDeployment.Spec.Template.Spec.Containers)).To(BeTrue())
							Ω(haveSdsSidecar(structuredDeployment.Spec.Template.Spec.Containers)).To(BeTrue())
							Expect(structuredDeployment.Spec.Template.Spec.Volumes).To(ContainElement(glooMtlsSecretVolume))
						}

						if structuredDeployment.GetName() == "gateway-proxy" {
							Ω(haveSdsSidecar(structuredDeployment.Spec.Template.Spec.Containers)).To(BeTrue())
							Expect(structuredDeployment.Spec.Template.Spec.Volumes).To(ContainElement(glooMtlsSecretVolume))
						}
					})
				})

				It("should add an additional listener to the gateway-proxy-envoy-config if $spec.extraListenersHelper is defined", func() {
					prepareMakefile(namespace, helmValues{
						valuesArgs: []string{"global.glooMtls.enabled=true,gatewayProxies.gatewayProxy.extraListenersHelper=gloo.testlistener"},
					})

					testManifest.SelectResources(func(resource *unstructured.Unstructured) bool {
						return resource.GetKind() == "ConfigMap"
					}).ExpectAll(func(configMap *unstructured.Unstructured) {
						configMapObject, err := kuberesource.ConvertUnstructured(configMap)
						Expect(err).NotTo(HaveOccurred(), fmt.Sprintf("ConfigMap %+v should be able to convert from unstructured", configMap))
						structuredConfigMap, ok := configMapObject.(*v1.ConfigMap)
						Expect(ok).To(BeTrue(), fmt.Sprintf("ConfigMap %+v should be able to cast to a structured config map", configMap))

						if structuredConfigMap.GetName() == "gateway-proxy-envoy-config" {
							expectedTestListener := "    - name: test_listener"
							Expect(structuredConfigMap.Data["envoy.yaml"]).To(ContainSubstring(expectedTestListener))
						}
					})
				})

				It("should set route prefix_rewrite in gateway-proxy-envoy-config from global.glooStats", func() {
					prepareMakefile(namespace, helmValues{
						valuesArgs: []string{
							"global.glooStats.enabled=true",
							"global.glooStats.routePrefixRewrite=/stats?format=json"},
					})

					testManifest.SelectResources(func(resource *unstructured.Unstructured) bool {
						return resource.GetKind() == "ConfigMap"
					}).ExpectAll(func(configMap *unstructured.Unstructured) {
						configMapObject, err := kuberesource.ConvertUnstructured(configMap)
						Expect(err).NotTo(HaveOccurred(), fmt.Sprintf("ConfigMap %+v should be able to convert from unstructured", configMap))
						structuredConfigMap, ok := configMapObject.(*v1.ConfigMap)
						Expect(ok).To(BeTrue(), fmt.Sprintf("ConfigMap %+v should be able to cast to a structured config map", configMap))

						if structuredConfigMap.GetName() == "gateway-proxy-envoy-config" {
							expectedPrefixRewrite := "prefix_rewrite: /stats?format=json"
							Expect(structuredConfigMap.Data["envoy.yaml"]).To(ContainSubstring(expectedPrefixRewrite))
						}
					})
				})

				It("should set route prefix_rewrite in gateway-proxy-envoy-config from gatewayProxies.gatewayProxy", func() {
					prepareMakefile(namespace, helmValues{
						valuesArgs: []string{
							"gatewayProxies.gatewayProxy.stats.enabled=true",
							"gatewayProxies.gatewayProxy.stats.routePrefixRewrite=/stats?format=json"},
					})

					testManifest.SelectResources(func(resource *unstructured.Unstructured) bool {
						return resource.GetKind() == "ConfigMap"
					}).ExpectAll(func(configMap *unstructured.Unstructured) {
						configMapObject, err := kuberesource.ConvertUnstructured(configMap)
						Expect(err).NotTo(HaveOccurred(), fmt.Sprintf("ConfigMap %+v should be able to convert from unstructured", configMap))
						structuredConfigMap, ok := configMapObject.(*v1.ConfigMap)
						Expect(ok).To(BeTrue(), fmt.Sprintf("ConfigMap %+v should be able to cast to a structured config map", configMap))

						if structuredConfigMap.GetName() == "gateway-proxy-envoy-config" {
							expectedPrefixRewrite := "prefix_rewrite: /stats?format=json"
							Expect(structuredConfigMap.Data["envoy.yaml"]).To(ContainSubstring(expectedPrefixRewrite))
						}
					})
				})
			})

			Context("gloo with istio sds settings", func() {
				var (
					istioAnnotation = "sidecar.istio.io/inject"

					istioCertsVolume = v1.Volume{
						Name: "istio-certs",
						VolumeSource: v1.VolumeSource{
							EmptyDir: &v1.EmptyDirVolumeSource{
								Medium: v1.StorageMediumMemory,
							},
						},
					}

					haveIstioSidecar = func(containers []v1.Container) bool {
						for _, c := range containers {
							if c.Name == "istio-proxy" {
								return true
							}
						}
						return false
					}

					istioSidecarVersion = func(containers []v1.Container) string {
						for _, c := range containers {
							if c.Name == "istio-proxy" {
								return c.Image
							}
						}
						return ""
					}

					haveSdsSidecar = func(containers []v1.Container) bool {
						for _, c := range containers {
							if c.Name == "sds" {
								return true
							}
						}
						return false
					}

					sdsIsIstioMode = func(containers []v1.Container) bool {
						for _, c := range containers {
							if c.Name == "sds" {
								for _, e := range c.Env {
									if e.Name == "ISTIO_MTLS_SDS_ENABLED" && e.Value == "true" {
										return true
									}
								}
							}
						}
						return false
					}
				)

				It("should add an sds sidecar AND an istio-proxy sidecar in the Gateway-Proxy Deployment", func() {
					prepareMakefile(namespace, helmValues{
						valuesArgs: []string{"global.istioSDS.enabled=true"},
					})

					testManifest.SelectResources(func(resource *unstructured.Unstructured) bool {
						return resource.GetKind() == "Deployment"
					}).ExpectAll(func(deployment *unstructured.Unstructured) {
						deploymentObject, err := kuberesource.ConvertUnstructured(deployment)
						Expect(err).NotTo(HaveOccurred(), fmt.Sprintf("Deployment %+v should be able to convert from unstructured", deployment))
						structuredDeployment, ok := deploymentObject.(*appsv1.Deployment)
						Expect(ok).To(BeTrue(), fmt.Sprintf("Deployment %+v should be able to cast to a structured deployment", deployment))

						if structuredDeployment.GetName() == "gateway-proxy" {
							Expect(len(structuredDeployment.Spec.Template.Spec.Containers)).To(Equal(3), "should have exactly 3 containers")
							Ω(haveSdsSidecar(structuredDeployment.Spec.Template.Spec.Containers)).To(BeTrue(), "gateway-proxy should have an sds sidecar")
							Ω(istioSidecarVersion(structuredDeployment.Spec.Template.Spec.Containers)).To(Equal("docker.io/istio/proxyv2:1.8.1"), "istio proxy sidecar should be the default")
							Ω(haveIstioSidecar(structuredDeployment.Spec.Template.Spec.Containers)).To(BeTrue(), "gateway-proxy should have an istio-proxy sidecar")
							Ω(sdsIsIstioMode(structuredDeployment.Spec.Template.Spec.Containers)).To(BeTrue(), "sds sidecar should have istio mode enabled")
							Expect(structuredDeployment.Spec.Template.Spec.Volumes).To(ContainElement(istioCertsVolume), "should have istio-certs volume mounted")
						}

						// Make sure gloo didn't pick up any sidecars for istio SDS (which it would for glooMTLS SDS)
						if structuredDeployment.GetName() == "gloo" {
							Ω(haveIstioSidecar(structuredDeployment.Spec.Template.Spec.Containers)).To(BeFalse(), "should not have istio-proxy sidecar in gloo")
							Ω(haveSdsSidecar(structuredDeployment.Spec.Template.Spec.Containers)).To(BeFalse(), "should not have sds sidecar in gloo")
							Expect(len(structuredDeployment.Spec.Template.Spec.Containers)).To(Equal(1), "should have exactly 1 container")
							Expect(structuredDeployment.Spec.Template.Spec.Volumes).NotTo(ContainElement(istioCertsVolume), "should not mount istio-certs in gloo")
						}

					})

					testManifest.SelectResources(func(resource *unstructured.Unstructured) bool {
						return resource.GetKind() == "ConfigMap"
					}).ExpectAll(func(configMap *unstructured.Unstructured) {
						configMapObject, err := kuberesource.ConvertUnstructured(configMap)
						Expect(err).NotTo(HaveOccurred(), fmt.Sprintf("Deployment %+v should be able to convert from unstructured", configMap))
						structuredConfigMap, ok := configMapObject.(*v1.ConfigMap)
						Expect(ok).To(BeTrue(), fmt.Sprintf("Deployment %+v should be able to cast to a structured deployment", configMap))

						if structuredConfigMap.Name == "gateway-proxy-envoy-config" {
							Expect(structuredConfigMap.Data["envoy.yaml"]).To(ContainSubstring("gateway_proxy_sds"), "should have an sds cluster configured")
						}
					})
				})

				It("should allow setting a custom istio sidecar in the Gateway-Proxy Deployment", func() {
					prepareMakefileFromValuesFile("values/val_custom_istio_sidecar.yaml")

					testManifest.SelectResources(func(resource *unstructured.Unstructured) bool {
						return resource.GetKind() == "Deployment"
					}).ExpectAll(func(deployment *unstructured.Unstructured) {
						deploymentObject, err := kuberesource.ConvertUnstructured(deployment)
						Expect(err).NotTo(HaveOccurred(), fmt.Sprintf("Deployment %+v should be able to convert from unstructured", deployment))
						structuredDeployment, ok := deploymentObject.(*appsv1.Deployment)
						Expect(ok).To(BeTrue(), fmt.Sprintf("Deployment %+v should be able to cast to a structured deployment", deployment))

						if structuredDeployment.GetName() == "gateway-proxy" {
							Expect(len(structuredDeployment.Spec.Template.Spec.Containers)).To(Equal(3), "should have exactly 3 containers")
							Ω(haveSdsSidecar(structuredDeployment.Spec.Template.Spec.Containers)).To(BeTrue(), "gateway-proxy should have an sds sidecar")
							Ω(haveIstioSidecar(structuredDeployment.Spec.Template.Spec.Containers)).To(BeTrue(), "gateway-proxy should have an istio-proxy sidecar")
							Ω(istioSidecarVersion(structuredDeployment.Spec.Template.Spec.Containers)).To(Equal("docker.io/istio/proxyv2:1.6.6"), "istio-proxy sidecar should be from the override file")
							Ω(sdsIsIstioMode(structuredDeployment.Spec.Template.Spec.Containers)).To(BeTrue(), "sds sidecar should have istio mode enabled")
							Expect(structuredDeployment.Spec.Template.Spec.Volumes).To(ContainElement(istioCertsVolume), "should have istio-certs volume mounted")
						}

						// Make sure gloo didn't pick up any sidecars for istio SDS (which it would for glooMTLS SDS)
						if structuredDeployment.GetName() == "gloo" {
							Ω(haveIstioSidecar(structuredDeployment.Spec.Template.Spec.Containers)).To(BeFalse(), "should not have istio-proxy sidecar in gloo")
							Ω(haveSdsSidecar(structuredDeployment.Spec.Template.Spec.Containers)).To(BeFalse(), "should not have sds sidecar in gloo")
							Expect(len(structuredDeployment.Spec.Template.Spec.Containers)).To(Equal(1), "should have exactly 1 container")
							Expect(structuredDeployment.Spec.Template.Spec.Volumes).NotTo(ContainElement(istioCertsVolume), "should not mount istio-certs in gloo")
						}

					})

					testManifest.SelectResources(func(resource *unstructured.Unstructured) bool {
						return resource.GetKind() == "ConfigMap"
					}).ExpectAll(func(configMap *unstructured.Unstructured) {
						configMapObject, err := kuberesource.ConvertUnstructured(configMap)
						Expect(err).NotTo(HaveOccurred(), fmt.Sprintf("Deployment %+v should be able to convert from unstructured", configMap))
						structuredConfigMap, ok := configMapObject.(*v1.ConfigMap)
						Expect(ok).To(BeTrue(), fmt.Sprintf("Deployment %+v should be able to cast to a structured deployment", configMap))

						if structuredConfigMap.Name == "gateway-proxy-envoy-config" {
							Expect(structuredConfigMap.Data["envoy.yaml"]).To(ContainSubstring("gateway_proxy_sds"), "should have an sds cluster configured")
						}
					})
				})

				It("should add an anti-injection annotation to all pods when disableAutoinjection is enabled", func() {
					prepareMakefile(namespace, helmValues{
						valuesArgs: []string{
							"global.istioIntegration.disableAutoinjection=true",
							"settings.integrations.knative.enabled=true", // ensure that as many pods as possible are checked
							"ingress.enabled=true",
						},
					})

					testManifest.SelectResources(func(resource *unstructured.Unstructured) bool {
						return resource.GetKind() == "Deployment"
					}).ExpectAll(func(deployment *unstructured.Unstructured) {
						deploymentObject, err := kuberesource.ConvertUnstructured(deployment)
						Expect(err).NotTo(HaveOccurred(), fmt.Sprintf("Deployment %+v should be able to convert from unstructured", deployment))
						structuredDeployment, ok := deploymentObject.(*appsv1.Deployment)
						Expect(ok).To(BeTrue(), fmt.Sprintf("Deployment %+v should be able to cast to a structured deployment", deployment))

						// ensure every deployment has a istio annotation set to false
						val, ok := structuredDeployment.Spec.Template.ObjectMeta.Annotations[istioAnnotation]
						Expect(ok).To(BeTrue(), fmt.Sprintf("Deployment %s should contain an istio injection annotation", deployment.GetName()))
						Expect(val).To(Equal("false"), fmt.Sprintf("Deployment %s should have an istio annotation with value of 'false'", deployment.GetName()))
					})
				})

				It("should add an Istio injection annotation for pods that can be configured for it", func() {
					prepareMakefile(namespace, helmValues{
						valuesArgs: []string{"global.istioIntegration.whitelistDiscovery=true",
							"global.istioIntegration.disableAutoinjection=false"},
					})

					testManifest.SelectResources(func(resource *unstructured.Unstructured) bool {
						return resource.GetKind() == "Deployment"
					}).ExpectAll(func(deployment *unstructured.Unstructured) {
						deploymentObject, err := kuberesource.ConvertUnstructured(deployment)
						Expect(err).NotTo(HaveOccurred(), fmt.Sprintf("Deployment %+v should be able to convert from unstructured", deployment))
						structuredDeployment, ok := deploymentObject.(*appsv1.Deployment)
						Expect(ok).To(BeTrue(), fmt.Sprintf("Deployment %+v should be able to cast to a structured deployment", deployment))

						// Ensure that the discovery pod has a true annotation, gateway-proxy has a false annotation (default), and nothing else has any annoation.
						// todo if we ever decide to add more pods to the list of 'allow istio injection' pods, then change this to a whitelist check
						if structuredDeployment.GetName() == "discovery" {
							val, ok := structuredDeployment.Spec.Template.ObjectMeta.Annotations[istioAnnotation]
							Expect(ok).To(BeTrue(), fmt.Sprintf("Deployment %s should contain an istio injection annotation", deployment.GetName()))
							Expect(val).To(Equal("true"), fmt.Sprintf("Deployment %s should have an istio annotation with value of 'true'", deployment.GetName()))
						} else {
							_, ok := structuredDeployment.Spec.Template.ObjectMeta.Annotations[istioAnnotation]
							Expect(ok).To(BeFalse(), fmt.Sprintf("Deployment %s should not contain an istio injection annotation", deployment.GetName()))
						}
					})
				})

				It("The created namespace can be labeled for Istio discovery", func() {
					prepareMakefile(namespace, helmValues{
						valuesArgs: []string{"namespace.create=true",
							"global.istioIntegration.labelInstallNamespace=true"},
					})

					testManifest.SelectResources(func(resource *unstructured.Unstructured) bool {
						return resource.GetKind() == "Namespace"
					}).ExpectAll(func(namespace *unstructured.Unstructured) {
						namespaceObject, err := kuberesource.ConvertUnstructured(namespace)
						Expect(err).NotTo(HaveOccurred(), fmt.Sprintf("Namespace %+v should be able to convert from unstructured", namespace))
						structuredNamespace, ok := namespaceObject.(*v1.Namespace)
						Expect(ok).To(BeTrue(), fmt.Sprintf("Deployment %+v should be able to cast to a structured deployment", namespace))

						// Ensure that the discovery pod has a true annotation, gateway-proxy has a false annotation (default), and nothing else has any annoation.
						if structuredNamespace.GetName() == "gloo-system" {
							val, ok := structuredNamespace.ObjectMeta.Labels["istio-injection"]
							Expect(ok).To(BeTrue(), fmt.Sprintf("Namespace %s should contain an Istio discovery label", structuredNamespace.GetName()))
							Expect(val).To(Equal("enabled"), fmt.Sprintf("Namespace %s should have an Istio discovery label with value of 'enabled'", structuredNamespace.GetName()))
						}
					})
				})
			})

			Context("gateway", func() {
				var labels map[string]string
				BeforeEach(func() {
					labels = map[string]string{
						"app":              "gloo",
						"gloo":             "gateway-proxy",
						"gateway-proxy-id": "gateway-proxy",
					}
					selector = map[string]string{
						"gateway-proxy":    "live",
						"gateway-proxy-id": "gateway-proxy",
					}
				})

				It("has a namespace", func() {
					prepareMakefile(namespace, helmValues{
						valuesArgs: []string{"gatewayProxies.gatewayProxy.service.extraAnnotations.test=test"},
					})
					rb := ResourceBuilder{
						Namespace: namespace,
						Name:      defaults.GatewayProxyName,
						Labels:    labels,
						Service: ServiceSpec{
							Ports: []PortSpec{
								{
									Name: "http",
									Port: 80,
								},
								{
									Name: "https",
									Port: 443,
								},
							},
						},
					}
					svc := rb.GetService()
					svc.Spec.Selector = selector
					svc.Spec.Type = v1.ServiceTypeLoadBalancer
					svc.Spec.Ports[0].TargetPort = intstr.FromInt(8080)
					svc.Spec.Ports[1].TargetPort = intstr.FromInt(8443)
					svc.Annotations = map[string]string{"test": "test"}
					testManifest.ExpectService(svc)
				})

				Context("access logging service", func() {
					var (
						accessLoggerName          = "gateway-proxy-access-logger"
						gatewayProxyConfigMapName = "gateway-proxy-envoy-config"
					)
					BeforeEach(func() {
						labels = map[string]string{
							"app":  "gloo",
							"gloo": "gateway-proxy-access-logger",
						}
					})

					It("can create an access logging deployment/service", func() {
						prepareMakefileFromValuesFile("values/val_access_logger.yaml")
						container := GetQuayContainerSpec("access-logger", version, GetPodNamespaceEnvVar(), GetPodNameEnvVar(),
							v1.EnvVar{
								Name:  "SERVICE_NAME",
								Value: "AccessLog",
							},
							v1.EnvVar{
								Name:  "SERVER_PORT",
								Value: "8083",
							},
						)
						container.PullPolicy = "IfNotPresent"
						svcBuilder := &ResourceBuilder{
							Namespace:  namespace,
							Name:       accessLoggerName,
							Labels:     cloneMap(labels),
							Containers: []ContainerSpec{container},
							Service: ServiceSpec{
								Ports: []PortSpec{
									{
										Name: "http",
										Port: 8083,
									},
								},
							},
						}
						svc := svcBuilder.GetService()
						svc.Spec.Selector = map[string]string{
							"app":  "gloo",
							"gloo": "gateway-proxy-access-logger",
						}
						svc.Spec.Type = ""
						svc.Spec.Ports[0].TargetPort = intstr.FromInt(8083)
						svc.Spec.Selector = cloneMap(labels)

						deploymentBuilder := &ResourceBuilder{
							Namespace:  namespace,
							Name:       accessLoggerName,
							Labels:     cloneMap(labels),
							Containers: []ContainerSpec{container},
							Service: ServiceSpec{
								Ports: []PortSpec{
									{
										Name: "http",
										Port: 8083,
									},
								},
							},
						}
						dep := deploymentBuilder.GetDeploymentAppsv1()
						dep.Spec.Template.ObjectMeta.Labels = cloneMap(labels)
						dep.Spec.Selector.MatchLabels = cloneMap(labels)
						dep.Spec.Template.Spec.Containers[0].Ports = []v1.ContainerPort{
							{Name: "http", ContainerPort: 8083, Protocol: "TCP"},
						}
						dep.Spec.Template.Annotations = statsAnnotations
						dep.Spec.Template.Spec.ServiceAccountName = "gateway-proxy"

						truez := true
						defaultUser := int64(10101)
						dep.Spec.Template.Spec.SecurityContext = &v1.PodSecurityContext{
							RunAsUser:    &defaultUser,
							RunAsNonRoot: &truez,
						}
						testManifest.ExpectDeploymentAppsV1(dep)
						testManifest.ExpectService(svc)
					})

					It("has a proxy with access logging cluster", func() {
						prepareMakefileFromValuesFile("values/val_access_logger.yaml")
						proxySpec := make(map[string]string)
						labels = map[string]string{
							"gloo":             "gateway-proxy",
							"app":              "gloo",
							"gateway-proxy-id": "gateway-proxy",
						}
						proxySpec["envoy.yaml"] = confWithAccessLogger
						cmRb := ResourceBuilder{
							Namespace: namespace,
							Name:      gatewayProxyConfigMapName,
							Labels:    labels,
							Data:      proxySpec,
						}
						proxy := cmRb.GetConfigMap()
						testManifest.ExpectConfigMapWithYamlData(proxy)
					})
				})

				Context("default gateways", func() {

					It("does not render when gatewayProxy is disabled", func() {
						prepareMakefile(namespace, helmValues{})
						testManifest.ExpectCustomResource("Gateway", namespace, defaults.GatewayProxyName)

						prepareMakefile(namespace, helmValues{
							valuesArgs: []string{"gatewayProxies.gatewayProxy.disabled=false"},
						})
						testManifest.ExpectCustomResource("Gateway", namespace, defaults.GatewayProxyName)

						prepareMakefile(namespace, helmValues{
							valuesArgs: []string{"gatewayProxies.gatewayProxy.disabled=true"},
						})
						testManifest.Expect("Gateway", namespace, defaults.GatewayProxyName).To(BeNil())
					})

					It("renders custom gateway when gatewayProxy is disabled", func() {
						prepareMakefile(namespace, helmValues{
							valuesArgs: []string{
								"gatewayProxies.gatewayProxy.disabled=true",
								"gatewayProxies.anotherGatewayProxy.disabled=true",
							},
						})
						testManifest.Expect("Gateway", namespace, defaults.GatewayProxyName).To(BeNil())
						testManifest.Expect("Gateway", namespace, "another-gateway-proxy").To(BeNil())

						prepareMakefile(namespace, helmValues{
							valuesArgs: []string{
								"gatewayProxies.gatewayProxy.disabled=true",
								"gatewayProxies.anotherGatewayProxy.disabled=false",
							},
						})
						testManifest.Expect("Gateway", namespace, defaults.GatewayProxyName).To(BeNil())
						testManifest.ExpectCustomResource("Gateway", namespace, "another-gateway-proxy")
					})

					var (
						proxyNames = []string{defaults.GatewayProxyName}
					)

					It("renders with http/https gateways by default", func() {
						prepareMakefile(namespace, helmValues{})
						gatewayUns := testManifest.ExpectCustomResource("Gateway", namespace, defaults.GatewayProxyName)
						var gateway1 gwv1.Gateway
						ConvertKubeResource(gatewayUns, &gateway1)
						Expect(gateway1.Ssl).To(BeFalse())
						Expect(gateway1.BindPort).To(Equal(uint32(8080)))
						Expect(gateway1.ProxyNames).To(Equal(proxyNames))
						Expect(gateway1.UseProxyProto).To(test_matchers.MatchProto(&wrappers.BoolValue{Value: false}))
						Expect(gateway1.BindAddress).To(Equal(defaults.GatewayBindAddress))
						gatewayUns = testManifest.ExpectCustomResource("Gateway", namespace, defaults.GatewayProxyName+"-ssl")
						ConvertKubeResource(gatewayUns, &gateway1)
						Expect(gateway1.Ssl).To(BeTrue())
						Expect(gateway1.BindPort).To(Equal(uint32(8443)))
						Expect(gateway1.ProxyNames).To(Equal(proxyNames))
						Expect(gateway1.UseProxyProto).To(test_matchers.MatchProto(&wrappers.BoolValue{Value: false}))
						Expect(gateway1.BindAddress).To(Equal(defaults.GatewayBindAddress))
					})

					It("can disable rendering http/https gateways", func() {
						prepareMakefile(namespace, helmValues{
							valuesArgs: []string{"gatewayProxies.gatewayProxy.gatewaySettings.disableGeneratedGateways=true"},
						})
						testManifest.ExpectUnstructured("Gateway", namespace, defaults.GatewayProxyName).To(BeNil())
						testManifest.ExpectUnstructured("Gateway", namespace, defaults.GatewayProxyName+"-ssl").To(BeNil())
					})

					It("can disable http gateway", func() {
						prepareMakefile(namespace, helmValues{
							valuesArgs: []string{"gatewayProxies.gatewayProxy.gatewaySettings.disableHttpGateway=true"},
						})
						testManifest.ExpectUnstructured("Gateway", namespace, defaults.GatewayProxyName).To(BeNil())
					})

					It("can disable https gateway", func() {
						prepareMakefile(namespace, helmValues{
							valuesArgs: []string{"gatewayProxies.gatewayProxy.gatewaySettings.disableHttpsGateway=true"},
						})
						testManifest.ExpectUnstructured("Gateway", namespace, defaults.GatewayProxyName+"-ssl").To(BeNil())
					})

					It("can set accessLoggingService", func() {
						name := defaults.GatewayProxyName
						bindPort := "8080"
						ssl := "false"
						gw := makeUnstructured(`
kind: Gateway
metadata:
  labels:
    app: gloo
  name: ` + name + `
  namespace: gloo-system
spec:
  bindAddress: '::'
  bindPort: ` + bindPort + `
  proxyNames:
  - gateway-proxy
  httpGateway: {}
  options:
    accessLoggingService:
      accessLog:
      - fileSink:
          path: /dev/stdout
          stringFormat: ""
  ssl: ` + ssl + `
  useProxyProto: false
apiVersion: gateway.solo.io/v1
`)
						prepareMakefileFromValuesFile("values/val_default_gateway_access_logging_service.yaml")
						testManifest.ExpectUnstructured("Gateway", namespace, defaults.GatewayProxyName).To(BeEquivalentTo(gw))

						name = defaults.GatewayProxyName + "-ssl"
						bindPort = "8443"
						ssl = "true"
						gw = makeUnstructured(`
kind: Gateway
metadata:
  labels:
    app: gloo
  name: ` + name + `
  namespace: gloo-system
spec:
  bindAddress: '::'
  bindPort: ` + bindPort + `
  proxyNames:
  - gateway-proxy
  httpGateway: {}
  options:
    accessLoggingService:
      accessLog:
      - fileSink:
          path: /dev/stdout
          stringFormat: ""
  ssl: ` + ssl + `
  useProxyProto: false
apiVersion: gateway.solo.io/v1
`)

						testManifest.ExpectUnstructured("Gateway", namespace, defaults.GatewayProxyName+"-ssl").To(BeEquivalentTo(gw))
					})

					It("can set tracing provider", func() {
						name := defaults.GatewayProxyName
						bindPort := "8080"
						ssl := "false"
						gw := makeUnstructured(`
apiVersion: gateway.solo.io/v1
kind: Gateway
metadata:
  labels:
    app: gloo
  name: ` + name + `
  namespace: gloo-system
spec:
  bindAddress: '::'
  bindPort: ` + bindPort + `
  proxyNames:
  - gateway-proxy
  httpGateway:
    options:
      httpConnectionManagerSettings:
        tracing:
          zipkinConfig:
            collector_cluster: zipkin
            collector_endpoint: /api/v2/spans
  ssl: ` + ssl + `
  useProxyProto: false
`)
						prepareMakefileFromValuesFile("values/val_tracing_provider_cluster.yaml")
						testManifest.ExpectUnstructured("Gateway", namespace, defaults.GatewayProxyName).To(BeEquivalentTo(gw))

						name = defaults.GatewayProxyName + "-ssl"
						bindPort = "8443"
						ssl = "true"
						gw = makeUnstructured(`
apiVersion: gateway.solo.io/v1
kind: Gateway
metadata:
  labels:
    app: gloo
  name: ` + name + `
  namespace: gloo-system
spec:
  bindAddress: '::'
  bindPort: ` + bindPort + `
  proxyNames:
  - gateway-proxy
  httpGateway:
    options:
      httpConnectionManagerSettings:
        tracing:
          zipkinConfig:
            collector_cluster: zipkin
            collector_endpoint: /api/v2/spans
  ssl: ` + ssl + `
  useProxyProto: false
`)

						testManifest.ExpectUnstructured("Gateway", namespace, defaults.GatewayProxyName+"-ssl").To(BeEquivalentTo(gw))
					})

					It("gwp hpa disabled by default", func() {

						testManifest.ExpectUnstructured("HorizontalPodAutoscaler", namespace, defaults.GatewayProxyName+"-hpa").To(BeNil())
					})

					It("can create gwp autoscaling/v1 hpa", func() {

						prepareMakefile(namespace, helmValues{
							valuesArgs: []string{
								"gatewayProxies.gatewayProxy.horizontalPodAutoscaler.apiVersion=autoscaling/v1",
								"gatewayProxies.gatewayProxy.horizontalPodAutoscaler.minReplicas=1",
								"gatewayProxies.gatewayProxy.horizontalPodAutoscaler.maxReplicas=2",
								"gatewayProxies.gatewayProxy.horizontalPodAutoscaler.targetCPUUtilizationPercentage=75",
							},
						})

						hpa := makeUnstructured(`
kind: HorizontalPodAutoscaler
metadata:
  labels:
    gateway-proxy-id: gateway-proxy
    gloo: gateway-proxy
    app: gloo
  name: gateway-proxy-hpa
  namespace: gloo-system
spec:
  maxReplicas: 2
  minReplicas: 1
  scaleTargetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: gateway-proxy
  targetCPUUtilizationPercentage: 75
apiVersion: autoscaling/v1
`)

						testManifest.ExpectUnstructured("HorizontalPodAutoscaler", namespace, defaults.GatewayProxyName+"-hpa").To(BeEquivalentTo(hpa))
					})

					It("can create gwp autoscaling/v2beta2 hpa", func() {

						prepareMakefileFromValuesFile("values/val_gwp_hpa_v2beta2.yaml")

						hpa := makeUnstructured(`
kind: HorizontalPodAutoscaler
metadata:
  labels:
    gateway-proxy-id: gateway-proxy
    gloo: gateway-proxy
    app: gloo
  name: gateway-proxy-hpa
  namespace: gloo-system
spec:
  maxReplicas: 2
  minReplicas: 1
  scaleTargetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: gateway-proxy
  metrics:
    - type: Resource
      resource:
        name: cpu
        target:
          type: Utilization
          averageUtilization: 50
  behavior:
    scaleDown:
      policies:
      - type: Pods
        value: 4
        periodSeconds: 60
      - type: Percent
        value: 10
        periodSeconds: 60
apiVersion: autoscaling/v2beta2
`)

						testManifest.ExpectUnstructured("HorizontalPodAutoscaler", namespace, defaults.GatewayProxyName+"-hpa").To(BeEquivalentTo(hpa))

					})

					It("gwp pdb disabled by default", func() {

						testManifest.ExpectUnstructured("PodDisruptionBudget", namespace, defaults.GatewayProxyName+"-pdb").To(BeNil())
					})

					It("can create gwp pdb with minAvailable", func() {

						prepareMakefile(namespace, helmValues{
							valuesArgs: []string{
								"gatewayProxies.gatewayProxy.podDisruptionBudget.minAvailable=2",
							},
						})

						pdb := makeUnstructured(`
apiVersion: policy/v1beta1
kind: PodDisruptionBudget
metadata:
  name: gateway-proxy-pdb
  namespace: gloo-system
spec:
  minAvailable: 2
  selector:
    matchLabels:
      gloo: gateway-proxy
`)

						testManifest.ExpectUnstructured("PodDisruptionBudget", namespace, defaults.GatewayProxyName+"-pdb").To(BeEquivalentTo(pdb))
					})

					It("can create gwp pdb with maxUnavailable", func() {

						prepareMakefile(namespace, helmValues{
							valuesArgs: []string{
								"gatewayProxies.gatewayProxy.podDisruptionBudget.maxUnavailable=2",
							},
						})

						pdb := makeUnstructured(`
apiVersion: policy/v1beta1
kind: PodDisruptionBudget
metadata:
  name: gateway-proxy-pdb
  namespace: gloo-system
spec:
  maxUnavailable: 2
  selector:
    matchLabels:
      gloo: gateway-proxy
`)

						testManifest.ExpectUnstructured("PodDisruptionBudget", namespace, defaults.GatewayProxyName+"-pdb").To(BeEquivalentTo(pdb))
					})

					It("can render with custom listener yaml", func() {
						newGatewayProxyName := "test-name"
						vsList := []*core.ResourceRef{
							{
								Name:      "one",
								Namespace: "one",
							},
						}
						prepareMakefileFromValuesFile("values/val_custom_gateways.yaml")
						for _, name := range []string{newGatewayProxyName, defaults.GatewayProxyName} {
							name := name
							gatewayUns := testManifest.ExpectCustomResource("Gateway", namespace, name)
							var gateway1 gwv1.Gateway
							ConvertKubeResource(gatewayUns, &gateway1)
							Expect(gateway1.UseProxyProto).To(test_matchers.MatchProto(&wrappers.BoolValue{
								Value: true,
							}))
							httpGateway := gateway1.GetHttpGateway()
							Expect(httpGateway).NotTo(BeNil())
							var msgList []proto.Message
							for _, v := range vsList {
								msgList = append(msgList, v)
							}
							Expect(httpGateway.VirtualServices).To(test_matchers.ConsistOfProtos(msgList...))
							gatewayUns = testManifest.ExpectCustomResource("Gateway", namespace, name+"-ssl")
							ConvertKubeResource(gatewayUns, &gateway1)
							Expect(gateway1.UseProxyProto).To(test_matchers.MatchProto(&wrappers.BoolValue{
								Value: true,
							}))
							msgList = []proto.Message{}
							for _, v := range vsList {
								msgList = append(msgList, v)
							}
							Expect(httpGateway.VirtualServices).To(test_matchers.ConsistOfProtos(msgList...))
						}

					})
				})

				Context("Failover Gateway", func() {

					var (
						proxyNames = []string{defaults.GatewayProxyName}
					)

					It("renders with http/https gateways by default", func() {
						prepareMakefile(namespace, helmValues{
							valuesArgs: []string{
								"gatewayProxies.gatewayProxy.failover.enabled=true",
								"gatewayProxies.gatewayProxy.failover.port=15444",
							},
						})
						gatewayUns := testManifest.ExpectCustomResource("Gateway", namespace, defaults.GatewayProxyName+"-failover")
						var gateway1 gwv1.Gateway
						ConvertKubeResource(gatewayUns, &gateway1)
						Expect(gateway1.BindPort).To(Equal(uint32(15444)))
						Expect(gateway1.ProxyNames).To(Equal(proxyNames))
						Expect(gateway1.BindAddress).To(Equal(defaults.GatewayBindAddress))
						tcpGateway := gateway1.GetTcpGateway()
						Expect(tcpGateway).NotTo(BeNil())
						Expect(tcpGateway.GetTcpHosts()).To(HaveLen(1))
						host := tcpGateway.GetTcpHosts()[0]
						Expect(host.GetSslConfig()).To(test_matchers.MatchProto(&gloov1.SslConfig{
							SslSecrets: &gloov1.SslConfig_SecretRef{
								SecretRef: &core.ResourceRef{
									Name:      "failover-downstream",
									Namespace: namespace,
								},
							},
						}))
						Expect(host.GetDestination().GetForwardSniClusterName()).To(test_matchers.MatchProto(&empty.Empty{}))
					})

					It("by default will not render failover gateway", func() {
						prepareMakefile(namespace, helmValues{})
						testManifest.ExpectUnstructured("Gateway", namespace, defaults.GatewayProxyName+"-failover").To(BeNil())
					})

				})

				Context("custom gateway", func() {
					Context("when the default values weren't overridden", func() {
						BeforeEach(func() {
							prepareMakefile(namespace, helmValues{
								valuesArgs: []string{
									"gatewayProxies.anotherGatewayProxy.specKey=testing",
								},
							})
						})
						It("uses default values for the gateway", func() {
							gatewayUns := testManifest.ExpectCustomResource("Gateway", namespace, "another-gateway-proxy")
							var customGateway gwv1.Gateway
							ConvertKubeResource(gatewayUns, &customGateway)
							Expect(customGateway.BindPort).To(Equal(defaults.DefaultGateway(namespace).BindPort))
						})
						It("uses default values for the deployment", func() {
							deploymentUns := testManifest.ExpectCustomResource("Deployment", namespace, "another-gateway-proxy")
							deployment, err := kuberesource.ConvertUnstructured(deploymentUns)
							Expect(err).NotTo(HaveOccurred())
							Expect(deployment).To(BeAssignableToTypeOf(&appsv1.Deployment{}))
							deploymentStr := deployment.(*appsv1.Deployment)
							Expect(*deploymentStr.Spec.Replicas).To(Equal(int32(1)))
						})
						It("uses default values for the service", func() {
							serviceUns := testManifest.ExpectCustomResource("Service", namespace, "another-gateway-proxy")
							service, err := kuberesource.ConvertUnstructured(serviceUns)
							Expect(err).NotTo(HaveOccurred())
							Expect(service).To(BeAssignableToTypeOf(&v1.Service{}))
							serviceStr := service.(*v1.Service)
							Expect(serviceStr.Spec.Type).To(Equal(v1.ServiceType("LoadBalancer")))
						})
						It("uses default values for the config map", func() {
							configMapUns := testManifest.ExpectCustomResource("ConfigMap", namespace, "another-gateway-proxy-envoy-config")
							configMap, err := kuberesource.ConvertUnstructured(configMapUns)
							Expect(err).NotTo(HaveOccurred())
							Expect(configMap).To(BeAssignableToTypeOf(&v1.ConfigMap{}))
							configMapStr := configMap.(*v1.ConfigMap)
							Expect(configMapStr.Data).ToNot(BeNil()) // Uses the default config data
						})
					})
					Context("when default values are overridden by custom gatewayproxy", func() {
						BeforeEach(func() {
							prepareMakefile(namespace, helmValues{
								valuesArgs: []string{
									"gatewayProxies.anotherGatewayProxy.podTemplate.httpPort=9999",          // used by gateway
									"gatewayProxies.anotherGatewayProxy.kind.deployment.replicas=50",        // used by deployment
									"gatewayProxies.anotherGatewayProxy.service.type=NodePort",              // used by service
									"gatewayProxies.anotherGatewayProxy.configMap.data.customData=someData", // used by config map
								},
							})
						})
						It("uses merged values for the gateway", func() {
							gatewayUns := testManifest.ExpectCustomResource("Gateway", namespace, "another-gateway-proxy")
							var customGateway gwv1.Gateway
							ConvertKubeResource(gatewayUns, &customGateway)
							Expect(customGateway.BindPort).To(Equal(uint32(9999)))
						})
						It("uses merged values for the deployment", func() {
							deploymentUns := testManifest.ExpectCustomResource("Deployment", namespace, "another-gateway-proxy")
							deployment, err := kuberesource.ConvertUnstructured(deploymentUns)
							Expect(err).NotTo(HaveOccurred())
							Expect(deployment).To(BeAssignableToTypeOf(&appsv1.Deployment{}))
							deploymentStr := deployment.(*appsv1.Deployment)
							Expect(*deploymentStr.Spec.Replicas).To(Equal(int32(50)))
						})
						It("uses merged values for the service", func() {
							serviceUns := testManifest.ExpectCustomResource("Service", namespace, "another-gateway-proxy")
							service, err := kuberesource.ConvertUnstructured(serviceUns)
							Expect(err).NotTo(HaveOccurred())
							Expect(service).To(BeAssignableToTypeOf(&v1.Service{}))
							serviceStr := *service.(*v1.Service)
							Expect(serviceStr.Spec.Type).To(Equal(v1.ServiceType("NodePort")))
						})
						It("uses merged values for the config map", func() {
							configMapUns := testManifest.ExpectCustomResource("ConfigMap", namespace, "another-gateway-proxy-envoy-config")
							configMap, err := kuberesource.ConvertUnstructured(configMapUns)
							Expect(err).NotTo(HaveOccurred())
							Expect(configMap).To(BeAssignableToTypeOf(&v1.ConfigMap{}))
							configMapStr := configMap.(*v1.ConfigMap)
							Expect(configMapStr.Data).To(Equal(map[string]string{"customData": "someData"}))
						})
					})
				})

				Context("when multiple custom gatewayproxy override disabled default proxy", func() {
					BeforeEach(func() {
						prepareMakefile(namespace, helmValues{
							valuesArgs: []string{
								"gatewayProxies.gatewayProxy.disabled=true",
								"gatewayProxies.gatewayProxy.gatewaySettings.disableHttpGateway=true",
								"gatewayProxies.gatewayProxy.gatewaySettings.customHttpsGateway.virtualServiceSelector.gateway=default",
								"gatewayProxies.firstGatewayProxy.disabled=false",
								"gatewayProxies.firstGatewayProxy.gatewaySettings.customHttpsGateway.virtualServiceSelector.gateway=first",
								"gatewayProxies.secondGatewayProxy.disabled=false",
								"gatewayProxies.secondGatewayProxy.gatewaySettings.customHttpsGateway.virtualServiceSelector.gateway=second",
							},
						})
					})
					It("correctly merges custom gatewayproxy values", func() {
						testManifest.Expect("Gateway", namespace, "gateway-proxy").To(BeNil())
						testManifest.Expect("Gateway", namespace, "first-gateway-proxy").To(BeNil())
						firstGatewayUns := testManifest.ExpectCustomResource("Gateway", namespace, "first-gateway-proxy-ssl")
						var firstGateway gwv1.Gateway
						ConvertKubeResource(firstGatewayUns, &firstGateway)
						Expect(firstGateway.GetHttpGateway().VirtualServiceSelector).To(HaveKeyWithValue("gateway", "first"))

						testManifest.Expect("Gateway", namespace, "second-gateway-proxy").To(BeNil())
						secondGatewayUns := testManifest.ExpectCustomResource("Gateway", namespace, "second-gateway-proxy-ssl")
						var secondGateway gwv1.Gateway
						ConvertKubeResource(secondGatewayUns, &secondGateway)
						Expect(secondGateway.GetHttpGateway().VirtualServiceSelector).To(HaveKeyWithValue("gateway", "second"))
					})
				})

				Context("gateway-proxy service account", func() {
					var gatewayProxyServiceAccount *v1.ServiceAccount

					BeforeEach(func() {
						saLabels := map[string]string{
							"app":  "gloo",
							"gloo": "gateway-proxy",
						}
						rb := ResourceBuilder{
							Namespace: namespace,
							Name:      "gateway-proxy",
							Args:      nil,
							Labels:    saLabels,
						}
						gatewayProxyServiceAccount = rb.GetServiceAccount()
						gatewayProxyServiceAccount.AutomountServiceAccountToken = proto.Bool(false)
					})

					It("sets extra annotations", func() {
						gatewayProxyServiceAccount.ObjectMeta.Annotations = map[string]string{"foo": "bar", "bar": "baz"}
						prepareMakefile(namespace, helmValues{
							valuesArgs: []string{
								"gateway.proxyServiceAccount.extraAnnotations.foo=bar",
								"gateway.proxyServiceAccount.extraAnnotations.bar=baz",
								"gateway.proxyServiceAccount.disableAutomount=true",
							},
						})
						testManifest.ExpectServiceAccount(gatewayProxyServiceAccount)
					})

				})

				Context("gateway-proxy service", func() {
					var gatewayProxyService *v1.Service

					BeforeEach(func() {
						serviceLabels := map[string]string{
							"app":              "gloo",
							"gloo":             "gateway-proxy",
							"gateway-proxy-id": "gateway-proxy",
						}
						rb := ResourceBuilder{
							Namespace: namespace,
							Name:      "gateway-proxy",
							Args:      nil,
							Labels:    serviceLabels,
						}
						gatewayProxyService = rb.GetService()
						selectorLabels := map[string]string{
							"gateway-proxy-id": "gateway-proxy",
							"gateway-proxy":    "live",
						}
						gatewayProxyService.Spec.Selector = selectorLabels
						gatewayProxyService.Spec.Ports = []v1.ServicePort{
							{
								Name:       "http",
								Protocol:   "TCP",
								Port:       80,
								TargetPort: intstr.IntOrString{IntVal: 8080},
							},
							{
								Name:       "https",
								Protocol:   "TCP",
								Port:       443,
								TargetPort: intstr.IntOrString{IntVal: 8443},
							},
						}
						gatewayProxyService.Spec.Type = v1.ServiceTypeLoadBalancer
					})

					It("is not created if disabled", func() {
						prepareMakefile(namespace, helmValues{})
						testManifest.ExpectService(gatewayProxyService)

						prepareMakefile(namespace, helmValues{
							valuesArgs: []string{"gatewayProxies.gatewayProxy.disabled=false"},
						})
						testManifest.ExpectService(gatewayProxyService)

						prepareMakefile(namespace, helmValues{
							valuesArgs: []string{"gatewayProxies.gatewayProxy.disabled=true"},
						})
						testManifest.Expect("Service", namespace, defaults.GatewayProxyName).To(BeNil())
					})

					It("sets extra annotations", func() {
						gatewayProxyService.ObjectMeta.Annotations = map[string]string{"foo": "bar", "bar": "baz"}
						prepareMakefile(namespace, helmValues{
							valuesArgs: []string{
								"gatewayProxies.gatewayProxy.service.extraAnnotations.foo=bar",
								"gatewayProxies.gatewayProxy.service.extraAnnotations.bar=baz",
							},
						})
						testManifest.ExpectService(gatewayProxyService)
					})

					It("sets externalIPs", func() {
						gatewayProxyService.Spec.Type = v1.ServiceTypeLoadBalancer
						gatewayProxyService.Spec.ExternalIPs = []string{"130.211.204.1", "130.211.204.2"}
						gatewayProxyService.Annotations = map[string]string{"test": "test"}
						prepareMakefileFromValuesFile("values/val_lb_external_ips.yaml")
						testManifest.ExpectService(gatewayProxyService)
					})

					It("sets external traffic policy", func() {
						gatewayProxyService.Spec.ExternalTrafficPolicy = v1.ServiceExternalTrafficPolicyTypeLocal
						prepareMakefile(namespace, helmValues{
							valuesArgs: []string{
								"gatewayProxies.gatewayProxy.service.externalTrafficPolicy=" + string(v1.ServiceExternalTrafficPolicyTypeLocal),
							},
						})
						testManifest.ExpectService(gatewayProxyService)
					})

					It("sets cluster IP", func() {
						gatewayProxyService.Spec.Type = v1.ServiceTypeClusterIP
						gatewayProxyService.Spec.ClusterIP = "test-ip"
						prepareMakefile(namespace, helmValues{
							valuesArgs: []string{
								"gatewayProxies.gatewayProxy.service.type=ClusterIP",
								"gatewayProxies.gatewayProxy.service.clusterIP=test-ip",
							},
						})
						testManifest.ExpectService(gatewayProxyService)
					})

					It("sets load balancer IP", func() {
						gatewayProxyService.Spec.Type = v1.ServiceTypeLoadBalancer
						gatewayProxyService.Spec.LoadBalancerIP = "test-lb-ip"
						prepareMakefile(namespace, helmValues{
							valuesArgs: []string{
								"gatewayProxies.gatewayProxy.service.type=LoadBalancer",
								"gatewayProxies.gatewayProxy.service.loadBalancerIP=test-lb-ip",
							},
						})
						testManifest.ExpectService(gatewayProxyService)
					})

					It("sets load balancer source ranges", func() {
						gatewayProxyService.Spec.Type = v1.ServiceTypeLoadBalancer
						gatewayProxyService.Spec.LoadBalancerSourceRanges = []string{"130.211.204.1/32", "130.211.204.2/32"}
						gatewayProxyService.Annotations = map[string]string{"test": "test"}
						prepareMakefileFromValuesFile("values/val_lb_source_ranges.yaml")
						testManifest.ExpectService(gatewayProxyService)
					})

					It("sets custom service name", func() {
						gatewayProxyService.ObjectMeta.Name = "gateway-proxy-custom"
						prepareMakefile(namespace, helmValues{
							valuesArgs: []string{
								"gatewayProxies.gatewayProxy.service.name=gateway-proxy-custom",
							},
						})
						testManifest.ExpectService(gatewayProxyService)
					})

					It("adds failover port", func() {
						gatewayProxyService.Spec.Ports = append(gatewayProxyService.Spec.Ports, v1.ServicePort{
							Name:     "failover",
							Protocol: v1.ProtocolTCP,
							Port:     15444,
							TargetPort: intstr.IntOrString{
								Type:   intstr.Int,
								IntVal: 15444,
							},
							NodePort: 32000,
						})
						prepareMakefile(namespace, helmValues{
							valuesArgs: []string{
								"gatewayProxies.gatewayProxy.failover.enabled=true",
								"gatewayProxies.gatewayProxy.failover.port=15444",
								"gatewayProxies.gatewayProxy.failover.nodePort=32000",
							},
						})
						testManifest.ExpectService(gatewayProxyService)
					})
				})

				Context("gateway-proxy deployment", func() {
					var (
						gatewayProxyDeployment *appsv1.Deployment
					)

					BeforeEach(func() {
						selector = map[string]string{
							"gloo":             "gateway-proxy",
							"gateway-proxy-id": "gateway-proxy",
						}
						podLabels := map[string]string{
							"gloo":             "gateway-proxy",
							"gateway-proxy":    "live",
							"gateway-proxy-id": "gateway-proxy",
						}
						podname := v1.EnvVar{
							Name: "POD_NAME",
							ValueFrom: &v1.EnvVarSource{
								FieldRef: &v1.ObjectFieldSelector{
									FieldPath: "metadata.name",
								},
							},
						}
						container := GetQuayContainerSpec("gloo-envoy-wrapper", version, GetPodNamespaceEnvVar(), podname)
						container.Name = "gateway-proxy"
						container.Args = []string{"--disable-hot-restart"}

						rb := ResourceBuilder{
							Namespace:  namespace,
							Name:       "gateway-proxy",
							Labels:     labels,
							Containers: []ContainerSpec{container},
						}
						deploy := rb.GetDeploymentAppsv1()
						deploy.Spec.Selector = &metav1.LabelSelector{
							MatchLabels: selector,
						}
						deploy.Spec.Template.ObjectMeta.Labels = podLabels
						deploy.Spec.Template.ObjectMeta.Annotations = map[string]string{
							"prometheus.io/path":   "/metrics",
							"prometheus.io/port":   "8081",
							"prometheus.io/scrape": "true",
						}
						deploy.Spec.Template.Spec.Volumes = []v1.Volume{{
							Name: "envoy-config",
							VolumeSource: v1.VolumeSource{
								ConfigMap: &v1.ConfigMapVolumeSource{
									LocalObjectReference: v1.LocalObjectReference{
										Name: "gateway-proxy-envoy-config",
									},
								},
							},
						}}
						deploy.Spec.Template.Spec.Containers[0].ImagePullPolicy = pullPolicy
						deploy.Spec.Template.Spec.Containers[0].Ports = []v1.ContainerPort{
							{Name: "http", ContainerPort: 8080, Protocol: "TCP"},
							{Name: "https", ContainerPort: 8443, Protocol: "TCP"},
						}
						deploy.Spec.Template.Spec.Containers[0].VolumeMounts = []v1.VolumeMount{{
							Name:      "envoy-config",
							ReadOnly:  false,
							MountPath: "/etc/envoy",
							SubPath:   "",
						}}
						truez := true
						falsez := false
						defaultUser := int64(10101)

						deploy.Spec.Template.Spec.SecurityContext = &v1.PodSecurityContext{
							FSGroup:   &defaultUser,
							RunAsUser: &defaultUser,
						}

						deploy.Spec.Template.Spec.Containers[0].SecurityContext = &v1.SecurityContext{
							Capabilities: &v1.Capabilities{
								Drop: []v1.Capability{"ALL"},
							},
							ReadOnlyRootFilesystem:   &truez,
							AllowPrivilegeEscalation: &falsez,
							RunAsNonRoot:             &truez,
							RunAsUser:                &defaultUser,
						}
						deploy.Spec.Template.Spec.ServiceAccountName = "gateway-proxy"
						gatewayProxyDeployment = deploy
					})

					Context("gateway-proxy daemonset", func() {
						var (
							daemonSet *appsv1.DaemonSet
						)
						BeforeEach(func() {
							daemonSet = &appsv1.DaemonSet{
								TypeMeta: metav1.TypeMeta{
									Kind:       "DaemonSet",
									APIVersion: "apps/v1",
								},
								ObjectMeta: gatewayProxyDeployment.ObjectMeta,
								Spec: appsv1.DaemonSetSpec{
									Selector: gatewayProxyDeployment.Spec.Selector,
									Template: gatewayProxyDeployment.Spec.Template,
								},
							}
							for i, port := range daemonSet.Spec.Template.Spec.Containers[0].Ports {
								port.HostPort = port.ContainerPort
								daemonSet.Spec.Template.Spec.Containers[0].Ports[i] = port
							}
							daemonSet.Spec.Template.Spec.DNSPolicy = v1.DNSClusterFirstWithHostNet
							daemonSet.Spec.Template.Spec.HostNetwork = true

						})

						It("creates a daemonset", func() {
							prepareMakefile(namespace, helmValues{
								valuesArgs: []string{
									"gatewayProxies.gatewayProxy.kind.deployment=null",
									"gatewayProxies.gatewayProxy.kind.daemonSet.hostPort=true",
								},
							})
							testManifest.Expect("DaemonSet", gatewayProxyDeployment.Namespace, gatewayProxyDeployment.Name).To(BeEquivalentTo(daemonSet))
						})
					})

					It("creates a deployment", func() {
						prepareMakefile(namespace, helmValues{})
						testManifest.ExpectDeploymentAppsV1(gatewayProxyDeployment)
					})

					It("supports multiple deployments", func() {
						prepareMakefile(namespace, helmValues{
							valuesArgs: []string{
								"gatewayProxies.gatewayProxyInternal.kind.deployment.replicas=1",
								"gatewayProxies.gatewayProxyInternal.configMap.data=null",
								"gatewayProxies.gatewayProxyInternal.service.extraAnnotations=null",
								"gatewayProxies.gatewayProxyInternal.service.type=ClusterIP",
								"gatewayProxies.gatewayProxyInternal.podTemplate.httpPort=8081",
								"gatewayProxies.gatewayProxyInternal.podTemplate.image.tag=dev",
							},
						})
						deploymentName := "gateway-proxy-internal"
						// deployment exists for for second declaration of gateway proxy
						testManifest.Expect("Deployment", namespace, deploymentName).NotTo(BeNil())
						testManifest.Expect("Deployment", namespace, "gateway-proxy").NotTo(BeNil())
					})

					It("supports extra args to envoy", func() {
						prepareMakefile(namespace, helmValues{
							valuesArgs: []string{
								"gatewayProxies.gatewayProxy.extraEnvoyArgs[0]=--log-format",
								// note that things that start with a percent make break yaml
								// hence the test.
								"gatewayProxies.gatewayProxy.extraEnvoyArgs[1]=%L%m%d %T.%e %t envoy] [%t][%n]%v",
							},
						})
						// deployment exists for for second declaration of gateway proxy
						gatewayProxyDeployment.Spec.Template.Spec.Containers[0].Args = append(
							gatewayProxyDeployment.Spec.Template.Spec.Containers[0].Args,
							"--log-format", "%L%m%d %T.%e %t envoy] [%t][%n]%v")
						testManifest.ExpectDeploymentAppsV1(gatewayProxyDeployment)
					})

					It("supports not specifying replicas to envoy", func() {
						prepareMakefile(namespace, helmValues{
							valuesArgs: []string{
								"gatewayProxies.gatewayProxy.kind.deployment.replicas=",
							},
						})
						// deployment exists for for second declaration of gateway proxy
						gatewayProxyDeployment.Spec.Replicas = nil
						testManifest.Expect("Deployment", namespace, "gateway-proxy").To(matchers.BeEquivalentToDiff(gatewayProxyDeployment))
					})

					It("disables net bind", func() {
						prepareMakefile(namespace, helmValues{
							valuesArgs: []string{"gatewayProxies.gatewayProxy.podTemplate.disableNetBind=true"},
						})
						gatewayProxyDeployment.Spec.Template.Spec.Containers[0].SecurityContext.Capabilities.Add = nil
						testManifest.ExpectDeploymentAppsV1(gatewayProxyDeployment)
					})

					It("unprivelged user", func() {
						prepareMakefile(namespace, helmValues{
							valuesArgs: []string{"gatewayProxies.gatewayProxy.podTemplate.runUnprivileged=true"},
						})
						truez := true
						uid := int64(10101)
						gatewayProxyDeployment.Spec.Template.Spec.Containers[0].SecurityContext.RunAsNonRoot = &truez
						gatewayProxyDeployment.Spec.Template.Spec.Containers[0].SecurityContext.RunAsUser = &uid
						testManifest.ExpectDeploymentAppsV1(gatewayProxyDeployment)
					})

					It("allows setting custom runAsUser", func() {
						prepareMakefile(namespace, helmValues{
							valuesArgs: []string{
								"gatewayProxies.gatewayProxy.podTemplate.runAsUser=10102",
								"gatewayProxies.gatewayProxy.podTemplate.runUnprivileged=true",
							},
						})
						uid := int64(10102)
						truez := true
						gatewayProxyDeployment.Spec.Template.Spec.SecurityContext.RunAsUser = &uid
						gatewayProxyDeployment.Spec.Template.Spec.Containers[0].SecurityContext.RunAsUser = &uid
						gatewayProxyDeployment.Spec.Template.Spec.Containers[0].SecurityContext.RunAsNonRoot = &truez
						testManifest.ExpectDeploymentAppsV1(gatewayProxyDeployment)
					})

					It("enables anti affinity ", func() {
						prepareMakefile(namespace, helmValues{
							valuesArgs: []string{"gatewayProxies.gatewayProxy.antiAffinity=true"},
						})
						gatewayProxyDeployment.Spec.Template.Spec.Affinity = &v1.Affinity{
							PodAntiAffinity: &v1.PodAntiAffinity{
								PreferredDuringSchedulingIgnoredDuringExecution: []v1.WeightedPodAffinityTerm{{
									Weight: 100,
									PodAffinityTerm: v1.PodAffinityTerm{
										TopologyKey: "kubernetes.io/hostname",
										LabelSelector: &metav1.LabelSelector{
											MatchLabels: map[string]string{"gloo": "gateway-proxy"},
										},
									},
								}},
							},
						}
						testManifest.ExpectDeploymentAppsV1(gatewayProxyDeployment)
					})

					It("sets affinity", func() {

						gatewayProxyDeployment.Spec.Template.Spec.Affinity = &v1.Affinity{
							NodeAffinity: &v1.NodeAffinity{
								RequiredDuringSchedulingIgnoredDuringExecution: &v1.NodeSelector{
									NodeSelectorTerms: []v1.NodeSelectorTerm{
										v1.NodeSelectorTerm{
											MatchExpressions: []v1.NodeSelectorRequirement{
												v1.NodeSelectorRequirement{
													Key:      "kubernetes.io/e2e-az-name",
													Operator: v1.NodeSelectorOpIn,
													Values:   []string{"e2e-az1", "e2e-az2"},
												},
											},
										},
									},
								},
							},
						}

						prepareMakefileFromValuesFile("values/val_gwp_affinity.yaml")

						testManifest.ExpectDeploymentAppsV1(gatewayProxyDeployment)
					})

					It("enables probes", func() {
						gatewayProxyDeployment.Spec.Template.Spec.Containers[0].ReadinessProbe = &v1.Probe{
							Handler: v1.Handler{
								Exec: &v1.ExecAction{
									Command: []string{
										"wget", "-O", "/dev/null", "127.0.0.1:19000/ready",
									},
								},
							},
							InitialDelaySeconds: 1,
							PeriodSeconds:       10,
							FailureThreshold:    10,
						}
						gatewayProxyDeployment.Spec.Template.Spec.Containers[0].LivenessProbe = &v1.Probe{
							Handler: v1.Handler{
								Exec: &v1.ExecAction{
									Command: []string{
										"wget", "-O", "/dev/null", "127.0.0.1:19000/server_info",
									},
								},
							},
							InitialDelaySeconds: 1,
							PeriodSeconds:       10,
							FailureThreshold:    10,
						}
						prepareMakefile(namespace, helmValues{
							valuesArgs: []string{"gatewayProxies.gatewayProxy.podTemplate.probes=true"},
						})
						testManifest.ExpectDeploymentAppsV1(gatewayProxyDeployment)
					})

					It("supports custom readiness probe", func() {
						prepareMakefile(namespace, helmValues{
							valuesArgs: []string{
								"gatewayProxies.gatewayProxy.podTemplate.probes=true",
								"gatewayProxies.gatewayProxy.podTemplate.customReadinessProbe.initialDelaySeconds=5",
								"gatewayProxies.gatewayProxy.podTemplate.customReadinessProbe.failureThreshold=3",
								"gatewayProxies.gatewayProxy.podTemplate.customReadinessProbe.periodSeconds=10",
								"gatewayProxies.gatewayProxy.podTemplate.customReadinessProbe.httpGet.path=/gloo/health",
								"gatewayProxies.gatewayProxy.podTemplate.customReadinessProbe.httpGet.port=8080",
								"gatewayProxies.gatewayProxy.podTemplate.customReadinessProbe.httpGet.scheme=HTTP",
							},
						})

						gatewayProxyDeployment.Spec.Template.Spec.Containers[0].ReadinessProbe = &v1.Probe{
							Handler: v1.Handler{
								HTTPGet: &v1.HTTPGetAction{
									Path:   "/gloo/health",
									Port:   intstr.FromInt(8080),
									Scheme: "HTTP",
								},
							},
							InitialDelaySeconds: 5,
							PeriodSeconds:       10,
							FailureThreshold:    3,
						}
						gatewayProxyDeployment.Spec.Template.Spec.Containers[0].LivenessProbe = &v1.Probe{
							Handler: v1.Handler{
								Exec: &v1.ExecAction{
									Command: []string{
										"wget", "-O", "/dev/null", "127.0.0.1:19000/server_info",
									},
								},
							},
							InitialDelaySeconds: 1,
							PeriodSeconds:       10,
							FailureThreshold:    10,
						}

						testManifest.ExpectDeploymentAppsV1(gatewayProxyDeployment)
					})

					It("renders terminationGracePeriodSeconds when present", func() {
						prepareMakefile(namespace, helmValues{
							valuesArgs: []string{
								"gatewayProxies.gatewayProxy.podTemplate.terminationGracePeriodSeconds=45",
							},
						})

						intz := int64(45)
						gatewayProxyDeployment.Spec.Template.Spec.TerminationGracePeriodSeconds = &intz

						testManifest.ExpectDeploymentAppsV1(gatewayProxyDeployment)
					})

					It("renders preStop hook for gracefulShutdown", func() {
						prepareMakefile(namespace, helmValues{
							valuesArgs: []string{
								"gatewayProxies.gatewayProxy.podTemplate.gracefulShutdown.enabled=true",
							},
						})

						gatewayProxyDeployment.Spec.Template.Spec.Containers[0].Lifecycle = &v1.Lifecycle{
							PreStop: &v1.Handler{
								Exec: &v1.ExecAction{
									Command: []string{
										"/bin/sh",
										"-c",
										"wget --post-data \"\" -O /dev/null 127.0.0.1:19000/healthcheck/fail; sleep 25",
									},
								},
							},
						}

						testManifest.ExpectDeploymentAppsV1(gatewayProxyDeployment)
					})

					It("renders preStop hook for gracefulShutdown with custom sleep time", func() {
						prepareMakefile(namespace, helmValues{
							valuesArgs: []string{
								"gatewayProxies.gatewayProxy.podTemplate.gracefulShutdown.enabled=true",
								"gatewayProxies.gatewayProxy.podTemplate.gracefulShutdown.sleepTimeSeconds=45",
							},
						})

						gatewayProxyDeployment.Spec.Template.Spec.Containers[0].Lifecycle = &v1.Lifecycle{
							PreStop: &v1.Handler{
								Exec: &v1.ExecAction{
									Command: []string{
										"/bin/sh",
										"-c",
										"wget --post-data \"\" -O /dev/null 127.0.0.1:19000/healthcheck/fail; sleep 45",
									},
								},
							},
						}

						testManifest.ExpectDeploymentAppsV1(gatewayProxyDeployment)
					})

					It("has limits", func() {
						prepareMakefile(namespace, helmValues{
							valuesArgs: []string{
								"gatewayProxies.gatewayProxy.podTemplate.resources.limits.memory=2",
								"gatewayProxies.gatewayProxy.podTemplate.resources.limits.cpu=3",
								"gatewayProxies.gatewayProxy.podTemplate.resources.requests.memory=4",
								"gatewayProxies.gatewayProxy.podTemplate.resources.requests.cpu=5",
							},
						})

						// Add the limits we are testing:
						gatewayProxyDeployment.Spec.Template.Spec.Containers[0].Resources = v1.ResourceRequirements{
							Limits: v1.ResourceList{
								v1.ResourceMemory: resource.MustParse("2"),
								v1.ResourceCPU:    resource.MustParse("3"),
							},
							Requests: v1.ResourceList{
								v1.ResourceMemory: resource.MustParse("4"),
								v1.ResourceCPU:    resource.MustParse("5"),
							},
						}
						testManifest.ExpectDeploymentAppsV1(gatewayProxyDeployment)
					})

					It("can overwrite the container image information", func() {
						gatewayProxyDeployment.Spec.Template.Spec.Containers[0].Image = fmt.Sprintf("gcr.io/solo-public/gloo-envoy-wrapper:%s", version)
						gatewayProxyDeployment.Spec.Template.Spec.Containers[0].ImagePullPolicy = "Always"
						prepareMakefile(namespace, helmValues{
							valuesArgs: []string{
								"gatewayProxies.gatewayProxy.podTemplate.image.pullPolicy=Always",
								"gatewayProxies.gatewayProxy.podTemplate.image.registry=gcr.io/solo-public",
							},
						})

						testManifest.ExpectDeploymentAppsV1(gatewayProxyDeployment)
					})

					It("adds readConfig annotations", func() {
						gatewayProxyDeployment.Spec.Template.Annotations["readconfig-stats"] = "/stats"
						gatewayProxyDeployment.Spec.Template.Annotations["readconfig-ready"] = "/ready"
						gatewayProxyDeployment.Spec.Template.Annotations["readconfig-config_dump"] = "/config_dump"
						gatewayProxyDeployment.Spec.Template.Annotations["readconfig-port"] = "8082"

						prepareMakefile(namespace, helmValues{
							valuesArgs: []string{"gatewayProxies.gatewayProxy.readConfig=true"},
						})
						testManifest.ExpectDeploymentAppsV1(gatewayProxyDeployment)
					})

					It("can add extra sidecar containers to the gateway-proxy deployment", func() {
						gatewayProxyDeployment.Spec.Template.Spec.Containers = append(
							gatewayProxyDeployment.Spec.Template.Spec.Containers,
							v1.Container{
								Name:  "nginx",
								Image: "nginx:1.7.9",
								Ports: []v1.ContainerPort{{ContainerPort: 80}},
							})

						gatewayProxyDeployment.Spec.Template.Spec.Containers[0].VolumeMounts = append(
							gatewayProxyDeployment.Spec.Template.Spec.Containers[0].VolumeMounts,
							v1.VolumeMount{
								Name:      "shared-data",
								MountPath: "/usr/share/shared-data",
							})

						gatewayProxyDeployment.Spec.Template.Spec.Volumes = append(
							gatewayProxyDeployment.Spec.Template.Spec.Volumes,
							v1.Volume{
								Name: "shared-data",
								VolumeSource: v1.VolumeSource{
									EmptyDir: &v1.EmptyDirVolumeSource{},
								},
							})

						prepareMakefile(namespace, helmValues{
							valuesArgs: []string{"gatewayProxies.gatewayProxy.extraContainersHelper=gloo.testcontainer"},
						})
						testManifest.ExpectDeploymentAppsV1(gatewayProxyDeployment)
					})

					It("ISTIO_META_MESH_ID env var default value set", func() {

						var value = "cluster.local"

						prepareMakefile(namespace, helmValues{
							valuesArgs: []string{
								"global.glooMtls.enabled=true", // adds gloo/gateway proxy side containers
								"global.istioSDS.enabled=true", // add default itsio sds sidecar
							},
						})

						testManifest.SelectResources(func(resource *unstructured.Unstructured) bool {
							return resource.GetKind() == "Deployment"
						}).ExpectAll(func(deployment *unstructured.Unstructured) {
							deploymentObject, err := kuberesource.ConvertUnstructured(deployment)
							Expect(err).NotTo(HaveOccurred(), fmt.Sprintf("Deployment %+v should be able to convert from unstructured", deployment))
							structuredDeployment, ok := deploymentObject.(*appsv1.Deployment)
							Expect(ok).To(BeTrue(), fmt.Sprintf("Deployment %+v should be able to cast to a structured deployment", deployment))

							//get ISTIO_META_MESH_ID env var value
							var istioMetaMeshID string
							for _, c := range structuredDeployment.Spec.Template.Spec.Containers {
								for _, e := range c.Env {
									if e.Name == "ISTIO_META_MESH_ID" {
										istioMetaMeshID = e.Value
										break
									}
								}
							}
							if structuredDeployment.GetName() == "gateway-proxy" {
								Expect(istioMetaMeshID).To(Equal(value), "ISTIO_META_MESH_ID should equal "+value)
							}

						})
					})

					It("can set ISTIO_META_MESH_ID env var", func() {

						var value = "foo"

						prepareMakefile(namespace, helmValues{
							valuesArgs: []string{
								"global.glooMtls.enabled=true", // adds gloo/gateway proxy side containers
								"global.istioSDS.enabled=true", // add default itsio sds sidecar
								"gatewayProxies.gatewayProxy.istioMetaMeshId=" + value,
							},
						})

						testManifest.SelectResources(func(resource *unstructured.Unstructured) bool {
							return resource.GetKind() == "Deployment"
						}).ExpectAll(func(deployment *unstructured.Unstructured) {
							deploymentObject, err := kuberesource.ConvertUnstructured(deployment)
							Expect(err).NotTo(HaveOccurred(), fmt.Sprintf("Deployment %+v should be able to convert from unstructured", deployment))
							structuredDeployment, ok := deploymentObject.(*appsv1.Deployment)
							Expect(ok).To(BeTrue(), fmt.Sprintf("Deployment %+v should be able to cast to a structured deployment", deployment))

							//get ISTIO_META_MESH_ID env var value
							var istioMetaMeshID string
							for _, c := range structuredDeployment.Spec.Template.Spec.Containers {
								for _, e := range c.Env {
									if e.Name == "ISTIO_META_MESH_ID" {
										istioMetaMeshID = e.Value
										break
									}
								}
							}
							if structuredDeployment.GetName() == "gateway-proxy" {
								Expect(istioMetaMeshID).To(Equal(value), "ISTIO_META_MESH_ID should equal "+value)
							}

						})
					})

					It("ISTIO_META_CLUSTER_ID env var default value set", func() {

						var value = "Kubernetes"

						prepareMakefile(namespace, helmValues{
							valuesArgs: []string{
								"global.glooMtls.enabled=true", // adds gloo/gateway proxy side containers
								"global.istioSDS.enabled=true", // add default itsio sds sidecar
							},
						})

						testManifest.SelectResources(func(resource *unstructured.Unstructured) bool {
							return resource.GetKind() == "Deployment"
						}).ExpectAll(func(deployment *unstructured.Unstructured) {
							deploymentObject, err := kuberesource.ConvertUnstructured(deployment)
							Expect(err).NotTo(HaveOccurred(), fmt.Sprintf("Deployment %+v should be able to convert from unstructured", deployment))
							structuredDeployment, ok := deploymentObject.(*appsv1.Deployment)
							Expect(ok).To(BeTrue(), fmt.Sprintf("Deployment %+v should be able to cast to a structured deployment", deployment))

							//get ISTIO_META_CLUSTER_ID env var value
							var istioMetaClusterID string
							for _, c := range structuredDeployment.Spec.Template.Spec.Containers {
								for _, e := range c.Env {
									if e.Name == "ISTIO_META_CLUSTER_ID" {
										istioMetaClusterID = e.Value
										break
									}
								}
							}
							if structuredDeployment.GetName() == "gateway-proxy" {
								Expect(istioMetaClusterID).To(Equal(value), "ISTIO_META_CLUSTER_ID should equal "+value)
							}

						})
					})

					It("can set ISTIO_META_CLUSTER_ID env var", func() {

						var value = "bar"

						prepareMakefile(namespace, helmValues{
							valuesArgs: []string{
								"global.glooMtls.enabled=true", // adds gloo/gateway proxy side containers
								"global.istioSDS.enabled=true", // add default itsio sds sidecar
								"gatewayProxies.gatewayProxy.istioMetaClusterId=" + value,
							},
						})

						testManifest.SelectResources(func(resource *unstructured.Unstructured) bool {
							return resource.GetKind() == "Deployment"
						}).ExpectAll(func(deployment *unstructured.Unstructured) {
							deploymentObject, err := kuberesource.ConvertUnstructured(deployment)
							Expect(err).NotTo(HaveOccurred(), fmt.Sprintf("Deployment %+v should be able to convert from unstructured", deployment))
							structuredDeployment, ok := deploymentObject.(*appsv1.Deployment)
							Expect(ok).To(BeTrue(), fmt.Sprintf("Deployment %+v should be able to cast to a structured deployment", deployment))

							//get ISTIO_META_CLUSTER_ID env var value
							var istioMetaClusterID string
							for _, c := range structuredDeployment.Spec.Template.Spec.Containers {
								for _, e := range c.Env {
									if e.Name == "ISTIO_META_CLUSTER_ID" {
										istioMetaClusterID = e.Value
										break
									}
								}
							}
							if structuredDeployment.GetName() == "gateway-proxy" {
								Expect(istioMetaClusterID).To(Equal(value), "ISTIO_META_CLUSTER_ID should equal "+value)
							}

						})
					})

					It("can add extra volume mounts to the gateway-proxy container deployment", func() {

						gatewayProxyDeployment.Spec.Template.Spec.Containers[0].VolumeMounts = append(
							gatewayProxyDeployment.Spec.Template.Spec.Containers[0].VolumeMounts,
							v1.VolumeMount{
								Name:      "tls-crt",
								MountPath: "/certs/crt",
								ReadOnly:  true,
							},
							v1.VolumeMount{
								Name:      "tls-key",
								MountPath: "/certs/key",
								ReadOnly:  true,
							},
							v1.VolumeMount{
								Name:      "sds-uds-path",
								MountPath: "/var/run/sds",
							},
						)

						gatewayProxyDeployment.Spec.Template.Spec.Volumes = append(
							gatewayProxyDeployment.Spec.Template.Spec.Volumes,
							v1.Volume{
								Name: "tls-crt",
								VolumeSource: v1.VolumeSource{
									Secret: &v1.SecretVolumeSource{
										SecretName: "gloo-test-cert",
										Items: []v1.KeyToPath{
											{Key: "tls.crt", Path: "tls.crt"},
										},
									},
								},
							},
							v1.Volume{
								Name: "tls-key",
								VolumeSource: v1.VolumeSource{
									Secret: &v1.SecretVolumeSource{
										SecretName: "gloo-test-cert",
										Items: []v1.KeyToPath{
											{Key: "tls.key", Path: "tls.key"},
										},
									},
								},
							},
							v1.Volume{
								Name: "sds-uds-path",
								VolumeSource: v1.VolumeSource{
									HostPath: &v1.HostPathVolumeSource{
										Path: "/var/run/sds",
									},
								},
							})

						prepareMakefile(namespace, helmValues{
							valuesArgs: []string{
								"gatewayProxies.gatewayProxy.extraProxyVolumeMounts[0].mountPath=/certs/crt",
								"gatewayProxies.gatewayProxy.extraProxyVolumeMounts[0].name=tls-crt",
								"gatewayProxies.gatewayProxy.extraProxyVolumeMounts[0].readOnly=true",
								"gatewayProxies.gatewayProxy.extraProxyVolumeMounts[1].mountPath=/certs/key",
								"gatewayProxies.gatewayProxy.extraProxyVolumeMounts[1].name=tls-key",
								"gatewayProxies.gatewayProxy.extraProxyVolumeMounts[1].readOnly=true",
								"gatewayProxies.gatewayProxy.extraVolumes[0].Name=tls-crt",
								"gatewayProxies.gatewayProxy.extraVolumes[0].Secret.secretName=gloo-test-cert",
								"gatewayProxies.gatewayProxy.extraVolumes[0].Secret.items[0].key=tls.crt",
								"gatewayProxies.gatewayProxy.extraVolumes[0].Secret.items[0].path=tls.crt",
								"gatewayProxies.gatewayProxy.extraVolumes[1].Name=tls-key",
								"gatewayProxies.gatewayProxy.extraVolumes[1].Secret.secretName=gloo-test-cert",
								"gatewayProxies.gatewayProxy.extraVolumes[1].Secret.items[0].key=tls.key",
								"gatewayProxies.gatewayProxy.extraVolumes[1].Secret.items[0].path=tls.key",
								"gatewayProxies.gatewayProxy.extraVolumeHelper=gloo.testVolume",
								"gatewayProxies.gatewayProxy.extraProxyVolumeMountHelper=gloo.testVolumeMount",
							},
						})
						testManifest.ExpectDeploymentAppsV1(gatewayProxyDeployment)
					})

					It("can set log level env var", func() {
						gatewayProxyDeployment.Spec.Template.Spec.Containers[0].Env = append(
							gatewayProxyDeployment.Spec.Template.Spec.Containers[0].Env,
							GetLogLevelEnvVar(),
						)
						prepareMakefile(namespace, helmValues{
							valuesArgs: []string{"gatewayProxies.gatewayProxy.logLevel=debug"},
						})
						testManifest.ExpectDeploymentAppsV1(gatewayProxyDeployment)
					})

					It("can set the envoy log level arg", func() {
						gatewayProxyDeployment.Spec.Template.Spec.Containers[0].Args = append(
							gatewayProxyDeployment.Spec.Template.Spec.Containers[0].Args,
							"--log-level debug",
						)
						prepareMakefile(namespace, helmValues{
							valuesArgs: []string{"gatewayProxies.gatewayProxy.envoyLogLevel=debug"},
						})
						testManifest.ExpectDeploymentAppsV1(gatewayProxyDeployment)
					})

					It("can accept extra env vars", func() {
						gatewayProxyDeployment.Spec.Template.Spec.Containers[0].Env = append(
							[]v1.EnvVar{GetTestExtraEnvVar()},
							gatewayProxyDeployment.Spec.Template.Spec.Containers[0].Env...,
						)
						prepareMakefile(namespace, helmValues{
							valuesArgs: []string{
								"gatewayProxies.gatewayProxy.kind.deployment.customEnv[0].Name=TEST_EXTRA_ENV_VAR",
								"gatewayProxies.gatewayProxy.kind.deployment.customEnv[0].Value=test",
							},
						})
						testManifest.ExpectDeploymentAppsV1(gatewayProxyDeployment)
					})

					It("can accept custom port values", func() {
						const testName = "TEST_CUSTOM_PORT"
						const testPort = int32(1234)
						const testTargetPort = int32(1235)
						prepareMakefile(namespace, helmValues{
							valuesArgs: []string{
								fmt.Sprintf("gatewayProxies.gatewayProxy.service.customPorts[0].name=%s", testName),
								fmt.Sprintf("gatewayProxies.gatewayProxy.service.customPorts[0].port=%d", testPort),
								fmt.Sprintf("gatewayProxies.gatewayProxy.service.customPorts[0].targetPort=%d", testTargetPort),
								"gatewayProxies.gatewayProxy.service.customPorts[0].protocol=TCP",
							},
						})
						// pull proxy service, cast it, then check for custom resources (which should always be the
						// first element of the Ports array).
						service := testManifest.ExpectCustomResource("Service", namespace, defaults.GatewayProxyName)
						serviceObject, err := kuberesource.ConvertUnstructured(service)
						Expect(err).NotTo(HaveOccurred(), fmt.Sprintf("Service %+v should be able to convert from unstructured", service))
						structuredService, ok := serviceObject.(*v1.Service)
						Expect(ok).To(BeTrue(), fmt.Sprintf("Service %+v should be able to cast to a structured deployment", service))
						customPort := structuredService.Spec.Ports[2]
						Expect(customPort.Name).To(Equal(testName))
						Expect(customPort.Protocol).To(Equal(v1.ProtocolTCP))
						Expect(customPort.Port).To(Equal(testPort))
						Expect(customPort.TargetPort.IntVal).To(Equal(testTargetPort))
					})

					It("does not disable gateway proxy", func() {
						prepareMakefile(namespace, helmValues{
							valuesArgs: []string{"gatewayProxies.gatewayProxy.disabled=false"},
						})
						testManifest.ExpectDeploymentAppsV1(gatewayProxyDeployment)
					})

					It("disables gateway proxy", func() {
						prepareMakefile(namespace, helmValues{
							valuesArgs: []string{"gatewayProxies.gatewayProxy.disabled=true"},
						})
						testManifest.Expect(gatewayProxyDeployment.Kind,
							gatewayProxyDeployment.GetNamespace(),
							gatewayProxyDeployment.GetName()).To(BeNil())
					})

					It("Removes rest_xds_cluster when enableRestEds is false", func() {
						prepareMakefile(namespace, helmValues{
							valuesArgs: []string{"settings.enableRestEds=false"},
						})

						testManifest.SelectResources(func(resource *unstructured.Unstructured) bool {
							return resource.GetKind() == "ConfigMap"
						}).ExpectAll(func(configMap *unstructured.Unstructured) {
							configMapObject, err := kuberesource.ConvertUnstructured(configMap)
							Expect(err).NotTo(HaveOccurred(), fmt.Sprintf("Deployment %+v should be able to convert from unstructured", configMap))
							structuredConfigMap, ok := configMapObject.(*v1.ConfigMap)
							Expect(ok).To(BeTrue(), fmt.Sprintf("Deployment %+v should be able to cast to a structured deployment", configMap))

							if structuredConfigMap.Name == "gateway-proxy-envoy-config" {
								Expect(structuredConfigMap.Data["envoy.yaml"]).To(Not(ContainSubstring("rest_xds_cluster")), "should not have an rest_xds_cluster configured")
							}
						})

					})

					It("Adds rest_xds_cluster when enableRestEds is true", func() {
						prepareMakefile(namespace, helmValues{
							valuesArgs: []string{"settings.enableRestEds=true"},
						})

						testManifest.SelectResources(func(resource *unstructured.Unstructured) bool {
							return resource.GetKind() == "ConfigMap"
						}).ExpectAll(func(configMap *unstructured.Unstructured) {
							configMapObject, err := kuberesource.ConvertUnstructured(configMap)
							Expect(err).NotTo(HaveOccurred(), fmt.Sprintf("Deployment %+v should be able to convert from unstructured", configMap))
							structuredConfigMap, ok := configMapObject.(*v1.ConfigMap)
							Expect(ok).To(BeTrue(), fmt.Sprintf("Deployment %+v should be able to cast to a structured deployment", configMap))

							if structuredConfigMap.Name == "gateway-proxy-envoy-config" {
								Expect(structuredConfigMap.Data["envoy.yaml"]).To(ContainSubstring("rest_xds_cluster"), "should have an rest_xds_cluster configured")
							}
						})

					})

					Context("pass image pull secrets", func() {
						pullSecretName := "test-pull-secret"
						pullSecret := []v1.LocalObjectReference{
							{Name: pullSecretName},
						}

						It("via global values", func() {
							prepareMakefile(namespace, helmValues{
								valuesArgs: []string{
									fmt.Sprintf("global.image.pullSecret=%s", pullSecretName),
								},
							})
							gatewayProxyDeployment.Spec.Template.Spec.ImagePullSecrets = pullSecret
							testManifest.ExpectDeploymentAppsV1(gatewayProxyDeployment)
						})

						It("via podTemplate values", func() {
							prepareMakefile(namespace, helmValues{
								valuesArgs: []string{
									fmt.Sprintf("gatewayProxies.gatewayProxy.podTemplate.image.pullSecret=%s", pullSecretName),
								},
							})
							gatewayProxyDeployment.Spec.Template.Spec.ImagePullSecrets = pullSecret
							testManifest.ExpectDeploymentAppsV1(gatewayProxyDeployment)
						})

						It("podTemplate values win over global", func() {
							prepareMakefile(namespace, helmValues{
								valuesArgs: []string{
									"global.image.pullSecret=wrong",
									fmt.Sprintf("gatewayProxies.gatewayProxy.podTemplate.image.pullSecret=%s", pullSecretName),
								},
							})
							gatewayProxyDeployment.Spec.Template.Spec.ImagePullSecrets = pullSecret
							testManifest.ExpectDeploymentAppsV1(gatewayProxyDeployment)
						})
					})
				})

				Context("gateway validation resources", func() {
					It("creates a service for the gateway validation port", func() {
						gwService := makeUnstructured(`
apiVersion: v1
kind: Service
metadata:
 labels:
   discovery.solo.io/function_discovery: disabled
   app: gloo
   gloo: gateway
 name: gateway
 namespace: ` + namespace + `
spec:
 ports:
 - name: https
   port: 443
   protocol: TCP
   targetPort: 8443
 selector:
   gloo: gateway
`)

						prepareMakefile(namespace, helmValues{})
						testManifest.ExpectUnstructured(gwService.GetKind(), gwService.GetNamespace(), gwService.GetName()).To(BeEquivalentTo(gwService))

					})

					It("creates settings with the gateway config with old mapping", func() {
						settings := makeUnstructureFromTemplateFile("fixtures/settings/gateway_settings.yaml", namespace)
						prepareMakefile(namespace, helmValues{
							valuesArgs: []string{
								"settings.invalidConfigPolicy.replaceInvalidRoutes=true",
								"settings.invalidConfigPolicy.invalidRouteResponseBody=Gloo Gateway has invalid configuration. Administrators should run `glooctl check` to find and fix config errors.",
								"settings.invalidConfigPolicy.invalidRouteResponseCode=404",
							},
						})
						testManifest.ExpectUnstructured(settings.GetKind(), settings.GetNamespace(), settings.GetName()).To(BeEquivalentTo(settings))
					})

					It("creates settings with the gateway config", func() {
						settings := makeUnstructureFromTemplateFile("fixtures/settings/gateway_settings.yaml", namespace)
						prepareMakefile(namespace, helmValues{
							valuesArgs: []string{
								"settings.invalidConfigPolicy.replaceInvalidRoutes=true",
								"settings.invalidConfigPolicy.invalidRouteResponseBody=Gloo Gateway has invalid configuration. Administrators should run `glooctl check` to find and fix config errors.",
								"settings.invalidConfigPolicy.invalidRouteResponseCode=404",
							},
						})
						testManifest.ExpectUnstructured(settings.GetKind(), settings.GetNamespace(), settings.GetName()).To(BeEquivalentTo(settings))
					})

					It("correctly sets the `disableKubernetesDestinations` field in the settings", func() {
						settings := makeUnstructureFromTemplateFile("fixtures/settings/disable_kubernetes_destinations.yaml", namespace)

						prepareMakefile(namespace, helmValues{
							valuesArgs: []string{
								"settings.disableKubernetesDestinations=true",
							},
						})
						testManifest.ExpectUnstructured(settings.GetKind(), settings.GetNamespace(), settings.GetName()).To(BeEquivalentTo(settings))
					})

					It("correctly sets the `validation.disableTransformationValidation` field in the validation settings", func() {
						settings := makeUnstructureFromTemplateFile("fixtures/settings/disable_transformation_validation.yaml", namespace)

						prepareMakefile(namespace, helmValues{
							valuesArgs: []string{
								"gateway.validation.disableTransformationValidation=true",
							},
						})
						testManifest.ExpectUnstructured(settings.GetKind(), settings.GetNamespace(), settings.GetName()).To(BeEquivalentTo(settings))
					})

					It("correctly allows setting readGatewaysFromAllNamespaces field in the settings when validation disabled", func() {
						settings := makeUnstructureFromTemplateFile("fixtures/settings/read_gateways_from_all_namespaces.yaml", namespace)

						prepareMakefile(namespace, helmValues{
							valuesArgs: []string{
								"gateway.validation.enabled=false",
								"gateway.readGatewaysFromAllNamespaces=true",
							},
						})
						testManifest.ExpectUnstructured(settings.GetKind(), settings.GetNamespace(), settings.GetName()).To(BeEquivalentTo(settings))
					})

					It("correctly allows setting ratelimit descriptors in the rateLimit field.", func() {
						settings := makeUnstructureFromTemplateFile("fixtures/settings/ratelimit_descriptors.yaml", namespace)

						prepareMakefile(namespace, helmValues{
							valuesArgs: []string{
								"settings.rateLimit.descriptors[0].key=generic_key",
								"settings.rateLimit.descriptors[0].value=per-second",
								"settings.rateLimit.descriptors[0].rateLimit.requestsPerUnit=2",
								"settings.rateLimit.descriptors[0].rateLimit.unit=SECOND",
							},
						})
						testManifest.ExpectUnstructured(settings.GetKind(), settings.GetNamespace(), settings.GetName()).To(BeEquivalentTo(settings))
					})

					It("correctly sets the `disableProxyGarbageCollection` field in the settings", func() {
						settings := makeUnstructureFromTemplateFile("fixtures/settings/disable_proxy_garbage_collection.yaml", namespace)

						prepareMakefile(namespace, helmValues{
							valuesArgs: []string{
								"settings.disableProxyGarbageCollection=true",
							},
						})
						testManifest.ExpectUnstructured(settings.GetKind(), settings.GetNamespace(), settings.GetName()).To(BeEquivalentTo(settings))
					})

					It("correctly sets the `regexMaxProgramSize` field in the settings", func() {
						settings := makeUnstructureFromTemplateFile("fixtures/settings/set_regex_max_program_size.yaml", namespace)

						prepareMakefile(namespace, helmValues{
							valuesArgs: []string{
								"settings.regexMaxProgramSize=500",
							},
						})
						testManifest.ExpectUnstructured(settings.GetKind(), settings.GetNamespace(), settings.GetName()).To(BeEquivalentTo(settings))
					})

					It("correctly sets the `gloo.enableRestEds` to false in the settings", func() {
						settings := makeUnstructureFromTemplateFile("fixtures/settings/enable_rest_eds.yaml", namespace)
						prepareMakefile(namespace, helmValues{
							valuesArgs: []string{
								"settings.enableRestEds=false",
							},
						})
						testManifest.ExpectUnstructured(settings.GetKind(), settings.GetNamespace(), settings.GetName()).To(BeEquivalentTo(settings))
					})

					It("enable default credentials", func() {
						settings := makeUnstructureFromTemplateFile("fixtures/settings/enable_default_credentials.yaml", namespace)

						prepareMakefile(namespace, helmValues{
							valuesArgs: []string{
								"settings.aws.enableCredentialsDiscovery=true",
							},
						})
						testManifest.ExpectUnstructured(settings.GetKind(), settings.GetNamespace(), settings.GetName()).To(BeEquivalentTo(settings))
					})

					It("allows setting extauth", func() {
						expectedYaml := makeUnstructured(`
apiVersion: gloo.solo.io/v1
kind: Settings
metadata:
  labels:
    app: gloo
    gloo: settings
  name: default
  namespace: gloo-system
spec:
  gloo:
    xdsBindAddr: "0.0.0.0:9977"
    restXdsBindAddr: "0.0.0.0:9976"
    enableRestEds: true
    disableKubernetesDestinations: false
    disableProxyGarbageCollection: false
    invalidConfigPolicy:
      invalidRouteResponseBody: Gloo Gateway has invalid configuration. Administrators should run ` + "`glooctl check`" + ` to find and fix config errors.
      invalidRouteResponseCode: 404
      replaceInvalidRoutes: false
  discoveryNamespace: gloo-system
  kubernetesArtifactSource: {}
  kubernetesConfigSource: {}
  kubernetesSecretSource: {}
  refreshRate: 60s

  gateway:
    readGatewaysFromAllNamespaces: false
    validation:
      proxyValidationServerAddr: gloo:9988
      alwaysAccept: true
      allowWarnings: true
      disableTransformationValidation: false
  discovery:
    fdsMode: WHITELIST
  extauth:
    extauthzServerRef:
      name: test
      namespace: testspace
`)
						prepareMakefile(namespace, helmValues{
							valuesArgs: []string{
								"global.extensions.extAuth.extauthzServerRef.name=test",
								"global.extensions.extAuth.extauthzServerRef.namespace=testspace",
							},
						})

						testManifest.ExpectUnstructured(expectedYaml.GetKind(), expectedYaml.GetNamespace(), expectedYaml.GetName()).To(BeEquivalentTo(expectedYaml))

					})

					It("finds resources on all containers, with identical resources on all sds and sidecar containers", func() {
						envoySidecarVals := []string{"100Mi", "200m", "300Mi", "400m"}
						sdsVals := []string{"101Mi", "201m", "301Mi", "401m"}

						prepareMakefile(namespace, helmValues{
							valuesArgs: []string{
								"global.glooMtls.enabled=true", // adds gloo/gateway proxy side containers
								"global.istioSDS.enabled=true", // add default itsio sds sidecar
								fmt.Sprintf("global.glooMtls.envoySidecarResources.requests.memory=%s", envoySidecarVals[0]),
								fmt.Sprintf("global.glooMtls.envoySidecarResources.requests.cpu=%s", envoySidecarVals[1]),
								fmt.Sprintf("global.glooMtls.envoySidecarResources.limits.memory=%s", envoySidecarVals[2]),
								fmt.Sprintf("global.glooMtls.envoySidecarResources.limits.cpu=%s", envoySidecarVals[3]),
								fmt.Sprintf("global.glooMtls.sdsResources.requests.memory=%s", sdsVals[0]),
								fmt.Sprintf("global.glooMtls.sdsResources.requests.cpu=%s", sdsVals[1]),
								fmt.Sprintf("global.glooMtls.sdsResources.limits.memory=%s", sdsVals[2]),
								fmt.Sprintf("global.glooMtls.sdsResources.limits.cpu=%s", sdsVals[3]),
							},
						})

						// get all deployments for arbitrary examination/testing
						var deployments []*unstructured.Unstructured
						testManifest.SelectResources(func(unstructured *unstructured.Unstructured) bool {
							if unstructured.GetKind() == "Deployment" {
								deployments = append(deployments, unstructured)
							}
							return true
						})

						for _, deployment := range deployments {
							// marshall unstructured object into deployment
							rawDeploy, err := deployment.MarshalJSON()
							Expect(err).NotTo(HaveOccurred())
							deploy := appsv1.Deployment{}
							err = json.Unmarshal(rawDeploy, &deploy)
							Expect(err).NotTo(HaveOccurred())

							// look for sidecar and sds containers, then test their resource values.
							for _, container := range deploy.Spec.Template.Spec.Containers {
								// still make sure non-sds/sidecar containers have non-nil resources, since all
								// other containers should have default resources values set in their templates.
								Expect(container.Resources).NotTo(BeNil(), "deployment/container %s/%s had nil resources", deployment.GetName(), container.Name)
								if container.Name == "envoy-sidecar" || container.Name == "sds" || container.Name == "istio-proxy" {
									var expectedVals = sdsVals
									// istio-proxy is another sds container
									if container.Name == "envoy-sidecar" {
										expectedVals = envoySidecarVals
									}

									Expect(container.Resources.Requests.Memory().String()).To(Equal(expectedVals[0]),
										"deployment/container %s/%s had incorrect request memory: expected %s, got %s",
										deployment.GetName(), container.Name, expectedVals[0], container.Resources.Requests.Memory().String())

									Expect(container.Resources.Requests.Cpu().String()).To(Equal(expectedVals[1]),
										"deployment/container %s/%s had incorrect request cpu: expected %s, got %s",
										deployment.GetName(), container.Name, expectedVals[1], container.Resources.Requests.Cpu().String())

									Expect(container.Resources.Limits.Memory().String()).To(Equal(expectedVals[2]),
										"deployment/container %s/%s had incorrect limit memory: expected %s, got %s",
										deployment.GetName(), container.Name, expectedVals[2], container.Resources.Limits.Memory().String())

									Expect(container.Resources.Limits.Cpu().String()).To(Equal(expectedVals[3]),
										"deployment/container %s/%s had incorrect limit cpu: expected %s, got %s",
										deployment.GetName(), container.Name, expectedVals[3], container.Resources.Limits.Cpu().String())
								}
							}
						}
					})

					It("enable sts discovery", func() {
						settings := makeUnstructureFromTemplateFile("fixtures/settings/sts_discovery.yaml", namespace)

						prepareMakefile(namespace, helmValues{
							valuesArgs: []string{
								"settings.aws.enableServiceAccountCredentials=true",
								"settings.aws.stsCredentialsRegion=us-east-2",
							},
						})
						testManifest.ExpectUnstructured(settings.GetKind(), settings.GetNamespace(), settings.GetName()).To(BeEquivalentTo(settings))
					})

					It("creates the validating webhook configuration", func() {
						vwc := makeUnstructured(`

apiVersion: admissionregistration.k8s.io/v1beta1
kind: ValidatingWebhookConfiguration
metadata:
  name: gloo-gateway-validation-webhook-` + namespace + `
  labels:
    app: gloo
    gloo: gateway
  annotations:
    "helm.sh/hook": pre-install
    "helm.sh/hook-weight": "5" # should come before cert-gen job
webhooks:
 - name: gateway.` + namespace + `.svc  # must be a domain with at least three segments separated by dots
   clientConfig:
     service:
       name: gateway
       namespace: ` + namespace + `
       path: "/validation"
     caBundle: "" # update manually or use certgen job
   rules:
     - operations: [ "CREATE", "UPDATE", "DELETE" ]
       apiGroups: ["gateway.solo.io"]
       apiVersions: ["v1"]
       resources: ["*"]
   sideEffects: None
   failurePolicy: Ignore

`)
						prepareMakefile(namespace, helmValues{})
						testManifest.ExpectUnstructured(vwc.GetKind(), vwc.GetNamespace(), vwc.GetName()).To(BeEquivalentTo(vwc))
					})

					It("adds the validation port and mounts the certgen secret to the gateway deployment", func() {

						gwDeployment := makeUnstructured(`
# Source: gloo/templates/5-gateway-deployment.yaml

apiVersion: apps/v1
kind: Deployment
metadata:
  labels:
    app: gloo
    gloo: gateway
  name: gateway
  namespace: ` + namespace + `
spec:
  replicas: 1
  selector:
    matchLabels:
      gloo: gateway
  template:
    metadata:
      labels:
        gloo: gateway
      annotations:
        prometheus.io/path: /metrics
        prometheus.io/port: "9091"
        prometheus.io/scrape: "true"
    spec:
      serviceAccountName: gateway
      containers:
      - image: quay.io/solo-io/gateway:` + version + `
        imagePullPolicy: IfNotPresent
        name: gateway
        ports:
          - containerPort: 8443
            name: https
            protocol: TCP

        securityContext:
          readOnlyRootFilesystem: true
          allowPrivilegeEscalation: false
          runAsNonRoot: true
          runAsUser: 10101
          capabilities:
            drop:
            - ALL
        env:
          - name: POD_NAMESPACE
            valueFrom:
              fieldRef:
                fieldPath: metadata.namespace
          - name: START_STATS_SERVER
            value: "true"
          - name: VALIDATION_MUST_START
            value: "true"
        volumeMounts:
          - mountPath: /etc/gateway/validation-certs
            name: validation-certs
        readinessProbe:
          tcpSocket:
            port: 8443
          initialDelaySeconds: 1
          periodSeconds: 2
          failureThreshold: 10
      volumes:
        - name: validation-certs
          secret:
            defaultMode: 420
            secretName: gateway-validation-certs
`)
						prepareMakefile(namespace, helmValues{})
						testManifest.ExpectUnstructured(gwDeployment.GetKind(), gwDeployment.GetNamespace(), gwDeployment.GetName()).To(BeEquivalentTo(gwDeployment))
					})

					It("creates the certgen job, rbac, and service account", func() {
						prepareMakefile(namespace, helmValues{})
						job := makeUnstructured(`
apiVersion: batch/v1
kind: Job
metadata:
  labels:
    app: gloo
    gloo: gateway-certgen
  name: gateway-certgen
  namespace: ` + namespace + `
  annotations:
    "helm.sh/hook": pre-install
    "helm.sh/hook-delete-policy": "hook-succeeded"
    "helm.sh/hook-weight": "10"
spec:
  ttlSecondsAfterFinished: 60
  template:
    metadata:
      labels:
        gloo: gateway-certgen
    spec:
      serviceAccountName: certgen
      containers:
        - image: quay.io/solo-io/certgen:` + version + `
          imagePullPolicy: IfNotPresent
          name: certgen
          securityContext:
            runAsUser: 10101
            runAsNonRoot: true
          env:
            - name: POD_NAMESPACE
              valueFrom:
                fieldRef:
                  fieldPath: metadata.namespace
          args:
            - "--secret-name=gateway-validation-certs"
            - "--svc-name=gateway"
            - "--validating-webhook-configuration-name=gloo-gateway-validation-webhook-` + namespace + `"
      restartPolicy: OnFailure

`)
						testManifest.ExpectUnstructured(job.GetKind(), job.GetNamespace(), job.GetName()).To(BeEquivalentTo(job))

						clusterRole := makeUnstructured(`

# this role requires access to cluster-scoped resources
kind: ClusterRole
apiVersion: rbac.authorization.k8s.io/v1
metadata:
    name: gloo-gateway-secret-create-vwc-update-` + namespace + `
    labels:
        app: gloo
        gloo: rbac
    annotations:
      "helm.sh/hook": pre-install
      "helm.sh/hook-weight": "5"
rules:
- apiGroups: [""]
  resources: ["secrets"]
  verbs: ["create", "get", "update"]
- apiGroups: ["admissionregistration.k8s.io"]
  resources: ["validatingwebhookconfigurations"]
  verbs: ["get", "update"]
`)
						testManifest.ExpectUnstructured(clusterRole.GetKind(), clusterRole.GetNamespace(), clusterRole.GetName()).To(BeEquivalentTo(clusterRole))

						clusterRoleBinding := makeUnstructured(`
# this role requires access to cluster-scoped resources
kind: ClusterRoleBinding
apiVersion: rbac.authorization.k8s.io/v1
metadata:
  name: gloo-gateway-secret-create-vwc-update-` + namespace + `
  labels:
    app: gloo
    gloo: rbac
  annotations:
    "helm.sh/hook": "pre-install"
    "helm.sh/hook-weight": "5"
subjects:
- kind: ServiceAccount
  name: certgen
  namespace: ` + namespace + `
roleRef:
  kind: ClusterRole
  name: gloo-gateway-secret-create-vwc-update-` + namespace + `
  apiGroup: rbac.authorization.k8s.io
---
`)
						testManifest.ExpectUnstructured(clusterRoleBinding.GetKind(), clusterRoleBinding.GetNamespace(), clusterRoleBinding.GetName()).To(BeEquivalentTo(clusterRoleBinding))

						serviceAccount := makeUnstructured(`

apiVersion: v1
kind: ServiceAccount
metadata:
  labels:
    app: gloo
    gloo: rbac
  annotations:
    "helm.sh/hook": "pre-install"
    "helm.sh/hook-weight": "5"
  name: certgen
  namespace: ` + namespace + `

`)
						testManifest.ExpectUnstructured(serviceAccount.GetKind(), serviceAccount.GetNamespace(), serviceAccount.GetName()).To(BeEquivalentTo(serviceAccount))

					})
				})
			})

			Context("gloo service account", func() {
				var glooServiceAccount *v1.ServiceAccount

				BeforeEach(func() {
					saLabels := map[string]string{
						"app":  "gloo",
						"gloo": "gloo",
					}
					rb := ResourceBuilder{
						Namespace: namespace,
						Name:      "gloo",
						Args:      nil,
						Labels:    saLabels,
					}
					glooServiceAccount = rb.GetServiceAccount()
					glooServiceAccount.AutomountServiceAccountToken = proto.Bool(false)
				})

				It("sets extra annotations", func() {
					glooServiceAccount.ObjectMeta.Annotations = map[string]string{"foo": "bar", "bar": "baz"}
					prepareMakefile(namespace, helmValues{
						valuesArgs: []string{
							"gloo.serviceAccount.extraAnnotations.foo=bar",
							"gloo.serviceAccount.extraAnnotations.bar=baz",
							"gloo.serviceAccount.disableAutomount=true",
						},
					})
					testManifest.ExpectServiceAccount(glooServiceAccount)
				})

			})

			Context("control plane deployments", func() {
				updateDeployment := func(deploy *appsv1.Deployment) {
					deploy.Spec.Selector = &metav1.LabelSelector{
						MatchLabels: selector,
					}
					deploy.Spec.Template.ObjectMeta.Labels = map[string]string{}
					for k, v := range selector {
						deploy.Spec.Template.ObjectMeta.Labels[k] = v
					}

					truez := true
					falsez := false
					user := int64(10101)
					deploy.Spec.Template.Spec.Containers[0].SecurityContext = &v1.SecurityContext{
						Capabilities: &v1.Capabilities{
							Drop: []v1.Capability{"ALL"},
						},
						RunAsNonRoot:             &truez,
						RunAsUser:                &user,
						ReadOnlyRootFilesystem:   &truez,
						AllowPrivilegeEscalation: &falsez,
					}
					deploy.Spec.Template.Spec.Containers[0].ImagePullPolicy = pullPolicy
				}

				Context("gloo deployment", func() {
					var (
						glooDeployment *appsv1.Deployment
						labels         map[string]string
					)
					BeforeEach(func() {
						labels = map[string]string{
							"gloo": "gloo",
							"app":  "gloo",
						}
						selector = map[string]string{
							"gloo": "gloo",
						}
						container := GetQuayContainerSpec("gloo", version, GetPodNamespaceEnvVar(), GetPodNamespaceStats())

						rb := ResourceBuilder{
							Namespace:   namespace,
							Name:        "gloo",
							Labels:      labels,
							Annotations: statsAnnotations,
							Containers:  []ContainerSpec{container},
						}
						deploy := rb.GetDeploymentAppsv1()
						updateDeployment(deploy)
						deploy.Spec.Template.Spec.Volumes = []v1.Volume{{
							Name: "labels-volume",
							VolumeSource: v1.VolumeSource{
								DownwardAPI: &v1.DownwardAPIVolumeSource{
									Items: []v1.DownwardAPIVolumeFile{{
										Path: "labels",
										FieldRef: &v1.ObjectFieldSelector{
											FieldPath: "metadata.labels",
										},
									}},
								},
							},
						}}
						deploy.Spec.Template.Spec.Containers[0].VolumeMounts = []v1.VolumeMount{{
							Name:      "labels-volume",
							MountPath: "/etc/gloo",
							ReadOnly:  true,
						}}
						deploy.Spec.Template.Spec.Containers[0].Ports = glooPorts
						deploy.Spec.Template.Spec.Containers[0].Resources = v1.ResourceRequirements{
							Requests: v1.ResourceList{
								v1.ResourceMemory: resource.MustParse("256Mi"),
								v1.ResourceCPU:    resource.MustParse("500m"),
							},
						}
						deploy.Spec.Template.Spec.Containers[0].ReadinessProbe = &v1.Probe{
							Handler: v1.Handler{
								TCPSocket: &v1.TCPSocketAction{
									Port: intstr.FromInt(9977),
								},
							},
							InitialDelaySeconds: 1,
							PeriodSeconds:       2,
							FailureThreshold:    10,
						}
						deploy.Spec.Template.Spec.ServiceAccountName = "gloo"
						glooDeployment = deploy
					})

					It("should create a deployment", func() {
						prepareMakefile(namespace, helmValues{})
						testManifest.ExpectDeploymentAppsV1(glooDeployment)
					})

					It("should allow overriding runAsUser", func() {
						prepareMakefile(namespace, helmValues{
							valuesArgs: []string{"gloo.deployment.runAsUser=10102"},
						})
						uid := int64(10102)
						glooDeployment.Spec.Template.Spec.Containers[0].SecurityContext.RunAsUser = &uid
						testManifest.ExpectDeploymentAppsV1(glooDeployment)
					})

					It("should disable usage stats collection when appropriate", func() {
						prepareMakefile(namespace, helmValues{
							valuesArgs: []string{"gloo.deployment.disableUsageStatistics=true"},
						})

						glooDeployment.Spec.Template.Spec.Containers[0].Env = append(glooDeployment.Spec.Template.Spec.Containers[0].Env, v1.EnvVar{
							Name:  client.DisableUsageVar,
							Value: "true",
						})

						testManifest.ExpectDeploymentAppsV1(glooDeployment)
					})

					It("has limits", func() {
						prepareMakefile(namespace, helmValues{
							valuesArgs: []string{
								"gloo.deployment.resources.limits.memory=2",
								"gloo.deployment.resources.limits.cpu=3",
								"gloo.deployment.resources.requests.memory=4",
								"gloo.deployment.resources.requests.cpu=5",
							},
						})

						// Add the limits we are testing:
						glooDeployment.Spec.Template.Spec.Containers[0].Resources = v1.ResourceRequirements{
							Limits: v1.ResourceList{
								v1.ResourceMemory: resource.MustParse("2"),
								v1.ResourceCPU:    resource.MustParse("3"),
							},
							Requests: v1.ResourceList{
								v1.ResourceMemory: resource.MustParse("4"),
								v1.ResourceCPU:    resource.MustParse("5"),
							},
						}
						testManifest.ExpectDeploymentAppsV1(glooDeployment)
					})

					It("can overwrite the container image information", func() {
						container := GetContainerSpec("gcr.io/solo-public", "gloo", version, GetPodNamespaceEnvVar(), GetPodNamespaceStats())
						container.PullPolicy = "Always"
						rb := ResourceBuilder{
							Namespace:   namespace,
							Name:        "gloo",
							Labels:      labels,
							Annotations: statsAnnotations,
							Containers:  []ContainerSpec{container},
						}
						deploy := rb.GetDeploymentAppsv1()
						updateDeployment(deploy)
						deploy.Spec.Template.Spec.Containers[0].Ports = glooPorts
						deploy.Spec.Template.Spec.ServiceAccountName = "gloo"

						glooDeployment = deploy
						prepareMakefile(namespace, helmValues{
							valuesArgs: []string{
								"gloo.deployment.image.pullPolicy=Always",
								"gloo.deployment.image.registry=gcr.io/solo-public",
							},
						})

					})

					It("can set log level env var", func() {
						glooDeployment.Spec.Template.Spec.Containers[0].Env = append(
							glooDeployment.Spec.Template.Spec.Containers[0].Env,
							GetLogLevelEnvVar(),
						)
						prepareMakefile(namespace, helmValues{
							valuesArgs: []string{"gloo.logLevel=debug"},
						})
						testManifest.ExpectDeploymentAppsV1(glooDeployment)
					})

					It("can accept extra env vars", func() {
						glooDeployment.Spec.Template.Spec.Containers[0].Env = append(
							[]v1.EnvVar{GetTestExtraEnvVar()},
							glooDeployment.Spec.Template.Spec.Containers[0].Env...,
						)
						prepareMakefile(namespace, helmValues{
							valuesArgs: []string{
								"gloo.deployment.customEnv[0].Name=TEST_EXTRA_ENV_VAR",
								"gloo.deployment.customEnv[0].Value=test",
							},
						})
						testManifest.ExpectDeploymentAppsV1(glooDeployment)
					})

					Context("pass image pull secrets", func() {
						pullSecretName := "test-pull-secret"
						pullSecret := []v1.LocalObjectReference{
							{Name: pullSecretName},
						}

						It("via global values", func() {
							prepareMakefile(namespace, helmValues{
								valuesArgs: []string{
									fmt.Sprintf("global.image.pullSecret=%s", pullSecretName),
								},
							})
							glooDeployment.Spec.Template.Spec.ImagePullSecrets = pullSecret
							testManifest.ExpectDeploymentAppsV1(glooDeployment)
						})

						It("via podTemplate values", func() {
							prepareMakefile(namespace, helmValues{
								valuesArgs: []string{
									fmt.Sprintf("gloo.deployment.image.pullSecret=%s", pullSecretName),
								},
							})
							glooDeployment.Spec.Template.Spec.ImagePullSecrets = pullSecret
							testManifest.ExpectDeploymentAppsV1(glooDeployment)
						})

						It("podTemplate values win over global", func() {
							prepareMakefile(namespace, helmValues{
								valuesArgs: []string{
									"global.image.pullSecret=wrong",
									fmt.Sprintf("gloo.deployment.image.pullSecret=%s", pullSecretName),
								},
							})
							glooDeployment.Spec.Template.Spec.ImagePullSecrets = pullSecret
							testManifest.ExpectDeploymentAppsV1(glooDeployment)
						})
					})
				})

				Context("gateway service account", func() {
					var gatewayServiceAccount *v1.ServiceAccount

					BeforeEach(func() {
						saLabels := map[string]string{
							"app":  "gloo",
							"gloo": "gateway",
						}
						rb := ResourceBuilder{
							Namespace: namespace,
							Name:      "gateway",
							Args:      nil,
							Labels:    saLabels,
						}
						gatewayServiceAccount = rb.GetServiceAccount()
						gatewayServiceAccount.AutomountServiceAccountToken = proto.Bool(false)
					})

					It("sets extra annotations", func() {
						gatewayServiceAccount.ObjectMeta.Annotations = map[string]string{"foo": "bar", "bar": "baz"}
						prepareMakefile(namespace, helmValues{
							valuesArgs: []string{
								"gateway.serviceAccount.extraAnnotations.foo=bar",
								"gateway.serviceAccount.extraAnnotations.bar=baz",
								"gateway.serviceAccount.disableAutomount=true",
							},
						})
						testManifest.ExpectServiceAccount(gatewayServiceAccount)
					})

				})

				Context("gateway deployment", func() {
					var (
						gatewayDeployment *appsv1.Deployment
						labels            map[string]string
					)
					BeforeEach(func() {
						labels = map[string]string{
							"gloo": "gateway",
							"app":  "gloo",
						}
						selector = map[string]string{
							"gloo": "gateway",
						}
						container := GetQuayContainerSpec("gateway", version, GetPodNamespaceEnvVar(), GetPodNamespaceStats())

						rb := ResourceBuilder{
							Namespace:   namespace,
							Name:        "gateway",
							Labels:      labels,
							Annotations: statsAnnotations,
							Containers:  []ContainerSpec{container},
						}
						deploy := rb.GetDeploymentAppsv1()
						updateDeployment(deploy)
						deploy.Spec.Template.Spec.ServiceAccountName = "gateway"

						deploy.Spec.Template.Spec.Volumes = []v1.Volume{{
							Name: "validation-certs",
							VolumeSource: v1.VolumeSource{
								Secret: &v1.SecretVolumeSource{
									SecretName:  "gateway-validation-certs",
									DefaultMode: proto.Int(420),
								},
							},
						}}
						deploy.Spec.Template.Spec.Containers[0].VolumeMounts = []v1.VolumeMount{{
							Name:      "validation-certs",
							MountPath: "/etc/gateway/validation-certs",
						}}
						deploy.Spec.Template.Spec.Containers[0].Ports = []v1.ContainerPort{{
							Name:          "https",
							ContainerPort: 8443,
							Protocol:      "TCP",
						}}
						deploy.Spec.Template.Spec.Containers[0].Env = append(deploy.Spec.Template.Spec.Containers[0].Env, v1.EnvVar{
							Name:  "VALIDATION_MUST_START",
							Value: "true",
						})

						deploy.Spec.Template.Spec.Containers[0].ReadinessProbe = &v1.Probe{
							Handler: v1.Handler{
								TCPSocket: &v1.TCPSocketAction{
									Port: intstr.FromInt(8443),
								},
							},
							InitialDelaySeconds: 1,
							PeriodSeconds:       2,
							FailureThreshold:    10,
						}

						gatewayDeployment = deploy
					})

					It("has a creates a deployment", func() {
						prepareMakefile(namespace, helmValues{})
						testManifest.ExpectDeploymentAppsV1(gatewayDeployment)
					})

					It("has limits", func() {
						prepareMakefile(namespace, helmValues{
							valuesArgs: []string{
								"gateway.deployment.resources.limits.memory=2",
								"gateway.deployment.resources.limits.cpu=3",
								"gateway.deployment.resources.requests.memory=4",
								"gateway.deployment.resources.requests.cpu=5",
							},
						})

						// Add the limits we are testing:
						gatewayDeployment.Spec.Template.Spec.Containers[0].Resources = v1.ResourceRequirements{
							Limits: v1.ResourceList{
								v1.ResourceMemory: resource.MustParse("2"),
								v1.ResourceCPU:    resource.MustParse("3"),
							},
							Requests: v1.ResourceList{
								v1.ResourceMemory: resource.MustParse("4"),
								v1.ResourceCPU:    resource.MustParse("5"),
							},
						}
						testManifest.ExpectDeploymentAppsV1(gatewayDeployment)
					})

					It("can overwrite the container image information", func() {
						container := GetContainerSpec("gcr.io/solo-public", "gateway", version, GetPodNamespaceEnvVar(), GetPodNamespaceStats())
						container.PullPolicy = "Always"
						rb := ResourceBuilder{
							Namespace:   namespace,
							Name:        "gateway",
							Labels:      labels,
							Annotations: statsAnnotations,
							Containers:  []ContainerSpec{container},
						}
						deploy := rb.GetDeploymentAppsv1()
						updateDeployment(deploy)

						gatewayDeployment = deploy
						prepareMakefile(namespace, helmValues{
							valuesArgs: []string{
								"gateway.deployment.image.pullPolicy=Always",
								"gateway.deployment.image.registry=gcr.io/solo-public",
							},
						})

					})

					It("can set log level env var", func() {
						gatewayDeployment.Spec.Template.Spec.Containers[0].Env = append(
							gatewayDeployment.Spec.Template.Spec.Containers[0].Env,
							GetLogLevelEnvVar(),
						)
						prepareMakefile(namespace, helmValues{
							valuesArgs: []string{"gateway.logLevel=debug"},
						})
						testManifest.ExpectDeploymentAppsV1(gatewayDeployment)
					})

					It("can accept extra env vars", func() {
						gatewayDeployment.Spec.Template.Spec.Containers[0].Env = append(
							[]v1.EnvVar{GetTestExtraEnvVar()},
							gatewayDeployment.Spec.Template.Spec.Containers[0].Env...,
						)
						prepareMakefile(namespace, helmValues{
							valuesArgs: []string{
								"gateway.deployment.customEnv[0].Name=TEST_EXTRA_ENV_VAR",
								"gateway.deployment.customEnv[0].Value=test",
							},
						})
						testManifest.ExpectDeploymentAppsV1(gatewayDeployment)
					})

					It("allows setting custom runAsUser", func() {
						prepareMakefile(namespace, helmValues{
							valuesArgs: []string{"gateway.deployment.runAsUser=10102"},
						})
						uid := int64(10102)
						gatewayDeployment.Spec.Template.Spec.Containers[0].SecurityContext.RunAsUser = &uid
						testManifest.ExpectDeploymentAppsV1(gatewayDeployment)
					})

					Context("pass image pull secrets", func() {
						pullSecretName := "test-pull-secret"
						pullSecret := []v1.LocalObjectReference{
							{Name: pullSecretName},
						}

						It("via global values", func() {
							prepareMakefile(namespace, helmValues{
								valuesArgs: []string{
									fmt.Sprintf("global.image.pullSecret=%s", pullSecretName),
								},
							})
							gatewayDeployment.Spec.Template.Spec.ImagePullSecrets = pullSecret
							testManifest.ExpectDeploymentAppsV1(gatewayDeployment)
						})

						It("via podTemplate values", func() {
							prepareMakefile(namespace, helmValues{
								valuesArgs: []string{
									fmt.Sprintf("gateway.deployment.image.pullSecret=%s", pullSecretName),
								},
							})
							gatewayDeployment.Spec.Template.Spec.ImagePullSecrets = pullSecret
							testManifest.ExpectDeploymentAppsV1(gatewayDeployment)
						})

						It("podTemplate values win over global", func() {
							prepareMakefile(namespace, helmValues{
								valuesArgs: []string{
									"global.image.pullSecret=wrong",
									fmt.Sprintf("gateway.deployment.image.pullSecret=%s", pullSecretName),
								},
							})
							gatewayDeployment.Spec.Template.Spec.ImagePullSecrets = pullSecret
							testManifest.ExpectDeploymentAppsV1(gatewayDeployment)
						})
					})
				})

				Context("discovery service account", func() {
					var discoveryServiceAccount *v1.ServiceAccount

					BeforeEach(func() {
						saLabels := map[string]string{
							"app":  "gloo",
							"gloo": "discovery",
						}
						rb := ResourceBuilder{
							Namespace: namespace,
							Name:      "discovery",
							Args:      nil,
							Labels:    saLabels,
						}
						discoveryServiceAccount = rb.GetServiceAccount()
						discoveryServiceAccount.AutomountServiceAccountToken = proto.Bool(false)
					})

					It("sets extra annotations", func() {
						discoveryServiceAccount.ObjectMeta.Annotations = map[string]string{"foo": "bar", "bar": "baz"}
						prepareMakefile(namespace, helmValues{
							valuesArgs: []string{
								"discovery.serviceAccount.extraAnnotations.foo=bar",
								"discovery.serviceAccount.extraAnnotations.bar=baz",
								"discovery.serviceAccount.disableAutomount=true",
							},
						})
						testManifest.ExpectServiceAccount(discoveryServiceAccount)
					})

				})

				Context("discovery deployment", func() {
					var (
						discoveryDeployment *appsv1.Deployment
						labels              map[string]string
					)
					BeforeEach(func() {
						labels = map[string]string{
							"gloo": "discovery",
							"app":  "gloo",
						}
						selector = map[string]string{
							"gloo": "discovery",
						}
						container := GetQuayContainerSpec("discovery", version, GetPodNamespaceEnvVar(), GetPodNamespaceStats())

						rb := ResourceBuilder{
							Namespace:   namespace,
							Name:        "discovery",
							Labels:      labels,
							Annotations: statsAnnotations,
							Containers:  []ContainerSpec{container},
						}
						deploy := rb.GetDeploymentAppsv1()
						updateDeployment(deploy)
						deploy.Spec.Template.Spec.ServiceAccountName = "discovery"
						user := int64(10101)
						deploy.Spec.Template.Spec.SecurityContext = &v1.PodSecurityContext{
							FSGroup: &user,
						}
						discoveryDeployment = deploy
					})

					It("has a creates a deployment", func() {
						prepareMakefile(namespace, helmValues{})
						testManifest.ExpectDeploymentAppsV1(discoveryDeployment)
					})

					It("disables probes", func() {
						prepareMakefile(namespace, helmValues{
							valuesArgs: []string{"discovery.deployment.probes=false"},
						})
						discoveryDeployment.Spec.Template.Spec.Containers[0].ReadinessProbe = nil
						discoveryDeployment.Spec.Template.Spec.Containers[0].LivenessProbe = nil
						testManifest.ExpectDeploymentAppsV1(discoveryDeployment)
					})

					It("has limits", func() {
						prepareMakefile(namespace, helmValues{
							valuesArgs: []string{
								"discovery.deployment.resources.limits.memory=2",
								"discovery.deployment.resources.limits.cpu=3",
								"discovery.deployment.resources.requests.memory=4",
								"discovery.deployment.resources.requests.cpu=5",
							},
						})

						// Add the limits we are testing:
						discoveryDeployment.Spec.Template.Spec.Containers[0].Resources = v1.ResourceRequirements{
							Limits: v1.ResourceList{
								v1.ResourceMemory: resource.MustParse("2"),
								v1.ResourceCPU:    resource.MustParse("3"),
							},
							Requests: v1.ResourceList{
								v1.ResourceMemory: resource.MustParse("4"),
								v1.ResourceCPU:    resource.MustParse("5"),
							},
						}
						testManifest.ExpectDeploymentAppsV1(discoveryDeployment)
					})

					It("can overwrite the container image information", func() {
						container := GetContainerSpec("gcr.io/solo-public", "discovery", version, GetPodNamespaceEnvVar(), GetPodNamespaceStats())
						container.PullPolicy = "Always"
						rb := ResourceBuilder{
							Namespace:   namespace,
							Name:        "discovery",
							Labels:      labels,
							Annotations: statsAnnotations,
							Containers:  []ContainerSpec{container},
						}
						deploy := rb.GetDeploymentAppsv1()
						updateDeployment(deploy)

						discoveryDeployment = deploy
						deploy.Spec.Template.Spec.ServiceAccountName = "discovery"
						prepareMakefile(namespace, helmValues{
							valuesArgs: []string{
								"discovery.deployment.image.pullPolicy=Always",
								"discovery.deployment.image.registry=gcr.io/solo-public",
							},
						})

					})

					It("can set log level env var", func() {
						discoveryDeployment.Spec.Template.Spec.Containers[0].Env = append(
							discoveryDeployment.Spec.Template.Spec.Containers[0].Env,
							GetLogLevelEnvVar(),
						)
						prepareMakefile(namespace, helmValues{
							valuesArgs: []string{"discovery.logLevel=debug"},
						})
						testManifest.ExpectDeploymentAppsV1(discoveryDeployment)
					})

					It("can accept extra env vars", func() {
						discoveryDeployment.Spec.Template.Spec.Containers[0].Env = append(
							[]v1.EnvVar{GetTestExtraEnvVar()},
							discoveryDeployment.Spec.Template.Spec.Containers[0].Env...,
						)
						prepareMakefile(namespace, helmValues{
							valuesArgs: []string{
								"discovery.deployment.customEnv[0].Name=TEST_EXTRA_ENV_VAR",
								"discovery.deployment.customEnv[0].Value=test",
							},
						})
						testManifest.ExpectDeploymentAppsV1(discoveryDeployment)
					})

					It("allows setting custom runAsUser", func() {
						prepareMakefile(namespace, helmValues{
							valuesArgs: []string{"discovery.deployment.runAsUser=10102"},
						})
						uid := int64(10102)
						discoveryDeployment.Spec.Template.Spec.Containers[0].SecurityContext.RunAsUser = &uid
						testManifest.ExpectDeploymentAppsV1(discoveryDeployment)
					})

					Context("pass image pull secrets", func() {
						pullSecretName := "test-pull-secret"
						pullSecret := []v1.LocalObjectReference{
							{Name: pullSecretName},
						}

						It("via global values", func() {
							prepareMakefile(namespace, helmValues{
								valuesArgs: []string{
									fmt.Sprintf("global.image.pullSecret=%s", pullSecretName),
								},
							})
							discoveryDeployment.Spec.Template.Spec.ImagePullSecrets = pullSecret
							testManifest.ExpectDeploymentAppsV1(discoveryDeployment)
						})

						It("via podTemplate values", func() {
							prepareMakefile(namespace, helmValues{
								valuesArgs: []string{
									fmt.Sprintf("discovery.deployment.image.pullSecret=%s", pullSecretName),
								},
							})
							discoveryDeployment.Spec.Template.Spec.ImagePullSecrets = pullSecret
							testManifest.ExpectDeploymentAppsV1(discoveryDeployment)
						})

						It("podTemplate values win over global", func() {
							prepareMakefile(namespace, helmValues{
								valuesArgs: []string{
									"global.image.pullSecret=wrong",
									fmt.Sprintf("discovery.deployment.image.pullSecret=%s", pullSecretName),
								},
							})
							discoveryDeployment.Spec.Template.Spec.ImagePullSecrets = pullSecret
							testManifest.ExpectDeploymentAppsV1(discoveryDeployment)
						})
					})
				})

			})

			Describe("configmaps", func() {
				var (
					gatewayProxyConfigMapName = "gateway-proxy-envoy-config"
				)

				labels := map[string]string{
					"gloo":             "gateway-proxy",
					"app":              "gloo",
					"gateway-proxy-id": "gateway-proxy",
				}

				It("is not created if disabled", func() {

					prepareMakefile(namespace, helmValues{
						valuesArgs: []string{"settings.aws.enableServiceAccountCredentials=true",
							"gatewayProxies.gatewayProxy.disabled=false"},
					})
					proxySpec := make(map[string]string)
					proxySpec["envoy.yaml"] = fmt.Sprintf(awsFmtString, "", "")
					cmRb := ResourceBuilder{
						Namespace: namespace,
						Name:      gatewayProxyConfigMapName,
						Labels:    labels,
						Data:      proxySpec,
					}
					proxy := cmRb.GetConfigMap()
					testManifest.ExpectConfigMapWithYamlData(proxy)

					prepareMakefile(namespace, helmValues{
						valuesArgs: []string{"gatewayProxies.gatewayProxy.disabled=true"},
					})
					testManifest.Expect("ConfigMap", namespace, defaults.GatewayProxyName).To(BeNil())
				})

				It("can create a gateway proxy with added static clusters", func() {
					prepareMakefileFromValuesFile("values/val_static_clusters.yaml")

					byt, err := ioutil.ReadFile("fixtures/envoy_config/static_clusters.yaml")
					Expect(err).ToNot(HaveOccurred())
					envoyBootstrapYaml := string(byt)

					envoyBootstrapSpec := make(map[string]string)
					envoyBootstrapSpec["envoy.yaml"] = envoyBootstrapYaml

					cmRb := ResourceBuilder{
						Namespace: namespace,
						Name:      gatewayProxyConfigMapName,
						Labels:    labels,
						Data:      envoyBootstrapSpec,
					}
					envoyBootstrapCm := cmRb.GetConfigMap()
					testManifest.ExpectConfigMapWithYamlData(envoyBootstrapCm)
				})

				It("can create a gateway proxy config with added bootstrap extensions", func() {

					prepareMakefileFromValuesFile("values/val_custom_bootstrap_extensions.yaml")

					byt, err := ioutil.ReadFile("fixtures/envoy_config/bootstrap_extensions.yaml")
					Expect(err).ToNot(HaveOccurred())
					envoyBootstrapYaml := string(byt)

					envoyBootstrapSpec := make(map[string]string)
					envoyBootstrapSpec["envoy.yaml"] = envoyBootstrapYaml

					cmRb := ResourceBuilder{
						Namespace: namespace,
						Name:      gatewayProxyConfigMapName,
						Labels:    labels,
						Data:      envoyBootstrapSpec,
					}
					envoyBootstrapCm := cmRb.GetConfigMap()
					testManifest.ExpectConfigMapWithYamlData(envoyBootstrapCm)
				})

				It("can create a gateway proxy config with custom static layer", func() {

					prepareMakefileFromValuesFile("values/val_custom_static_bootstrap.yaml")

					byt, err := ioutil.ReadFile("fixtures/envoy_config/custom_static_bootstrap.yaml")
					Expect(err).ToNot(HaveOccurred())
					envoyBootstrapYaml := string(byt)

					envoyBootstrapSpec := make(map[string]string)
					envoyBootstrapSpec["envoy.yaml"] = envoyBootstrapYaml

					cmRb := ResourceBuilder{
						Namespace: namespace,
						Name:      gatewayProxyConfigMapName,
						Labels:    labels,
						Data:      envoyBootstrapSpec,
					}
					envoyBootstrapCm := cmRb.GetConfigMap()
					testManifest.ExpectConfigMapWithYamlData(envoyBootstrapCm)
				})

				Describe("gateway proxy - AWS", func() {

					It("has a global cluster", func() {

						prepareMakefile(namespace, helmValues{
							valuesArgs: []string{"settings.aws.enableServiceAccountCredentials=true"},
						})
						proxySpec := make(map[string]string)
						proxySpec["envoy.yaml"] = fmt.Sprintf(awsFmtString, "", "")
						cmRb := ResourceBuilder{
							Namespace: namespace,
							Name:      gatewayProxyConfigMapName,
							Labels:    labels,
							Data:      proxySpec,
						}
						proxy := cmRb.GetConfigMap()
						testManifest.ExpectConfigMapWithYamlData(proxy)
					})

					It("has a regional cluster", func() {

						prepareMakefile(namespace, helmValues{
							valuesArgs: []string{
								"settings.aws.enableServiceAccountCredentials=true",
								"settings.aws.stsCredentialsRegion=us-east-2",
							},
						})
						proxySpec := make(map[string]string)
						proxySpec["envoy.yaml"] = fmt.Sprintf(awsFmtString, "us-east-2.", "us-east-2.")
						cmRb := ResourceBuilder{
							Namespace: namespace,
							Name:      gatewayProxyConfigMapName,
							Labels:    labels,
							Data:      proxySpec,
						}
						proxy := cmRb.GetConfigMap()
						testManifest.ExpectConfigMapWithYamlData(proxy)
					})
				})

				Describe("gateway proxy - tracing config", func() {
					It("has a proxy without tracing", func() {
						prepareMakefile(namespace, helmValues{
							valuesArgs: []string{"gatewayProxies.gatewayProxy.service.extraAnnotations.test=test"},
						})
						proxySpec := make(map[string]string)
						proxySpec["envoy.yaml"] = confWithoutTracing
						cmRb := ResourceBuilder{
							Namespace: namespace,
							Name:      gatewayProxyConfigMapName,
							Labels:    labels,
							Data:      proxySpec,
						}
						proxy := cmRb.GetConfigMap()
						testManifest.ExpectConfigMapWithYamlData(proxy)
					})

					It("has a proxy with tracing cluster", func() {
						prepareMakefileFromValuesFile("values/val_tracing_provider_cluster.yaml")
						proxySpec := make(map[string]string)
						proxySpec["envoy.yaml"] = confWithTracingCluster
						cmRb := ResourceBuilder{
							Namespace: namespace,
							Name:      gatewayProxyConfigMapName,
							Labels:    labels,
							Data:      proxySpec,
						}
						proxy := cmRb.GetConfigMap()
						testManifest.ExpectConfigMapWithYamlData(proxy)
					})
				})

				Describe("gateway proxy -- readConfig config", func() {
					It("has a listener for reading a subset of the admin api", func() {
						prepareMakefile(namespace, helmValues{
							valuesArgs: []string{"gatewayProxies.gatewayProxy.readConfig=true"},
						})
						proxySpec := make(map[string]string)
						proxySpec["envoy.yaml"] = confWithReadConfig
						cmRb := ResourceBuilder{
							Namespace: namespace,
							Name:      gatewayProxyConfigMapName,
							Labels:    labels,
							Data:      proxySpec,
						}
						proxy := cmRb.GetConfigMap()
						testManifest.ExpectConfigMapWithYamlData(proxy)
					})
				})
				Describe("gateway proxy -- readConfigMulticluster config", func() {
					It("has a service for the gateway proxy config dump port", func() {
						prepareMakefile(namespace, helmValues{
							valuesArgs: []string{"gatewayProxies.gatewayProxy.readConfig=true",
								"gatewayProxies.gatewayProxy.readConfigMulticluster=true"},
						})
						serviceLabels := map[string]string{
							"gloo":             "gateway-proxy",
							"gateway-proxy-id": "gateway-proxy",
						}
						rb := ResourceBuilder{
							Namespace: namespace,
							Name:      "gateway-proxy-config-dump-service",
							Args:      nil,
							Labels:    serviceLabels,
						}
						gatewayProxyConfigDumpService := rb.GetService()
						gatewayProxyConfigDumpService.Spec.Selector = serviceLabels
						gatewayProxyConfigDumpService.Spec.Ports = []v1.ServicePort{
							{
								Protocol: "TCP",
								Port:     8082,
							},
						}
						gatewayProxyConfigDumpService.Spec.Type = v1.ServiceTypeClusterIP
						testManifest.ExpectService(gatewayProxyConfigDumpService)
					})
				})
				Describe("supports multiple gateway proxy config maps", func() {
					It("can parse multiple config maps", func() {
						prepareMakefile(namespace, helmValues{
							valuesArgs: []string{
								"gatewayProxies.gatewayProxyInternal.kind.deployment.replicas=1",
								"gatewayProxies.gatewayProxyInternal.configMap.data=null",
								"gatewayProxies.gatewayProxyInternal.service.extraAnnotations=null",
								"gatewayProxies.gatewayProxyInternal.service.type=ClusterIP",
								"gatewayProxies.gatewayProxyInternal.podTemplate.httpPort=8081",
								"gatewayProxies.gatewayProxyInternal.podTemplate.image.tag=dev",
							},
						})
						cmName := "gateway-proxy-internal-envoy-config"
						// cm exists for for second declaration of gateway proxy
						testManifest.Expect("ConfigMap", namespace, cmName).NotTo(BeNil())
						testManifest.Expect("ConfigMap", namespace, "gateway-proxy-envoy-config").NotTo(BeNil())
					})
				})

			})

			Context("ingress-proxy service", func() {

				var ingressProxyService *v1.Service

				BeforeEach(func() {
					serviceLabels := map[string]string{
						"app":  "gloo",
						"gloo": "ingress-proxy",
					}
					rb := ResourceBuilder{
						Namespace: namespace,
						Name:      "ingress-proxy",
						Args:      nil,
						Labels:    serviceLabels,
					}
					ingressProxyService = rb.GetService()
					selectorLabels := map[string]string{
						"gloo": "ingress-proxy",
					}
					ingressProxyService.Spec.Selector = selectorLabels
					ingressProxyService.Spec.Ports = []v1.ServicePort{
						{
							Name:       "http",
							Protocol:   "TCP",
							Port:       80,
							TargetPort: intstr.IntOrString{IntVal: 8080},
						},
						{
							Name:       "https",
							Protocol:   "TCP",
							Port:       443,
							TargetPort: intstr.IntOrString{IntVal: 8443},
						},
					}
					ingressProxyService.Spec.Type = v1.ServiceTypeLoadBalancer
				})

				It("sets extra annotations", func() {
					ingressProxyService.ObjectMeta.Annotations = map[string]string{"foo": "bar", "bar": "baz"}
					prepareMakefile(namespace, helmValues{
						valuesArgs: []string{
							"ingress.enabled=true",
							"ingressProxy.service.extraAnnotations.foo=bar",
							"ingressProxy.service.extraAnnotations.bar=baz",
						},
					})
					testManifest.ExpectService(ingressProxyService)
				})

				It("sets type", func() {
					ingressProxyService.Spec.Type = v1.ServiceTypeNodePort
					prepareMakefile(namespace, helmValues{
						valuesArgs: []string{
							"ingress.enabled=true",
							"ingressProxy.service.type=NodePort",
						},
					})
					testManifest.ExpectService(ingressProxyService)
				})

				It("sets loadBalancerIP", func() {
					ingressProxyService.Spec.LoadBalancerIP = "1.2.3.4"
					prepareMakefile(namespace, helmValues{
						valuesArgs: []string{
							"ingress.enabled=true",
							"ingressProxy.service.loadBalancerIP=1.2.3.4",
						},
					})
					testManifest.ExpectService(ingressProxyService)
				})

				It("should set route prefix_rewrite in ingress-envoy-config from global.glooStats", func() {
					prepareMakefile(namespace, helmValues{
						valuesArgs: []string{
							"ingress.enabled=true",
							"ingressProxy.deployment.stats=true",
							"global.glooStats.enabled=true",
							"global.glooStats.routePrefixRewrite=/stats?format=json"},
					})

					testManifest.SelectResources(func(resource *unstructured.Unstructured) bool {
						return resource.GetKind() == "ConfigMap"
					}).ExpectAll(func(configMap *unstructured.Unstructured) {
						configMapObject, err := kuberesource.ConvertUnstructured(configMap)
						Expect(err).NotTo(HaveOccurred(), fmt.Sprintf("ConfigMap %+v should be able to convert from unstructured", configMap))
						structuredConfigMap, ok := configMapObject.(*v1.ConfigMap)
						Expect(ok).To(BeTrue(), fmt.Sprintf("ConfigMap %+v should be able to cast to a structured config map", configMap))

						if structuredConfigMap.GetName() == "ingress-envoy-config" {
							expectedPrefixRewrite := "prefix_rewrite: /stats?format=json"
							Expect(structuredConfigMap.Data["envoy.yaml"]).To(ContainSubstring(expectedPrefixRewrite))
						}
					})
				})
			})

			Describe("merge ingress and gateway", func() {

				// helper for passing a values file
				prepareMakefileFromValuesFile := func(valuesFile string) {
					prepareMakefile(namespace, helmValues{
						valuesFile: valuesFile,
					})
				}

				It("merges the config correctly, allow override of ingress without altering gloo", func() {
					deploymentLabels := map[string]string{
						"app": "gloo", "gloo": "gloo",
					}
					selectors := map[string]string{
						"gloo": "gloo",
					}
					podLabels := map[string]string{
						"gloo": "gloo",
					}
					var glooDeploymentPostMerge = &appsv1.Deployment{
						TypeMeta: metav1.TypeMeta{
							Kind:       "Deployment",
							APIVersion: "apps/v1",
						},
						ObjectMeta: metav1.ObjectMeta{
							Name:      "gloo",
							Namespace: "gloo-system",
							Labels:    deploymentLabels,
						},
						Spec: appsv1.DeploymentSpec{
							Replicas: pointer.Int32Ptr(1),
							Selector: &metav1.LabelSelector{MatchLabels: selectors},
							Template: v1.PodTemplateSpec{
								ObjectMeta: metav1.ObjectMeta{
									Labels:      podLabels,
									Annotations: statsAnnotations,
								},
								Spec: v1.PodSpec{
									Volumes: []v1.Volume{{
										Name: "labels-volume",
										VolumeSource: v1.VolumeSource{
											DownwardAPI: &v1.DownwardAPIVolumeSource{
												Items: []v1.DownwardAPIVolumeFile{{
													Path: "labels",
													FieldRef: &v1.ObjectFieldSelector{
														FieldPath: "metadata.labels",
													},
												}},
											},
										},
									}},
									ServiceAccountName: "gloo",
									Containers: []v1.Container{
										{
											VolumeMounts: []v1.VolumeMount{{
												Name:      "labels-volume",
												MountPath: "/etc/gloo",
												ReadOnly:  true,
											}},
											Name: "gloo",
											// Note: this was NOT overwritten
											Image: fmt.Sprintf("quay.io/solo-io/gloo:%v", version),
											Ports: glooPorts,
											Env: []v1.EnvVar{
												{
													Name: "POD_NAMESPACE",
													ValueFrom: &v1.EnvVarSource{
														FieldRef: &v1.ObjectFieldSelector{APIVersion: "", FieldPath: "metadata.namespace"},
													},
												},
												{
													Name:  "START_STATS_SERVER",
													Value: "true",
												},
											},
											Resources: v1.ResourceRequirements{
												Limits: nil,
												Requests: v1.ResourceList{
													v1.ResourceMemory: resource.MustParse("256Mi"),
													v1.ResourceCPU:    resource.MustParse("500m"),
												},
											},
											ImagePullPolicy: "IfNotPresent",
											SecurityContext: &v1.SecurityContext{
												Capabilities:             &v1.Capabilities{Add: nil, Drop: []v1.Capability{"ALL"}},
												RunAsUser:                pointer.Int64Ptr(10101),
												RunAsNonRoot:             pointer.BoolPtr(true),
												ReadOnlyRootFilesystem:   pointer.BoolPtr(true),
												AllowPrivilegeEscalation: pointer.BoolPtr(false),
											},
											ReadinessProbe: &v1.Probe{
												Handler: v1.Handler{
													TCPSocket: &v1.TCPSocketAction{
														Port: intstr.FromInt(9977),
													},
												},
												InitialDelaySeconds: 1,
												PeriodSeconds:       2,
												FailureThreshold:    10,
											},
										},
									},
								},
							},
						},
					}
					ingressDeploymentLabels := map[string]string{
						"app": "gloo", "gloo": "ingress",
					}
					ingressSelector := map[string]string{
						"gloo": "ingress",
					}
					ingressPodLabels := map[string]string{
						"gloo": "ingress",
					}
					var ingressDeploymentPostMerge = &appsv1.Deployment{
						TypeMeta: metav1.TypeMeta{
							Kind:       "Deployment",
							APIVersion: "apps/v1",
						},
						ObjectMeta: metav1.ObjectMeta{
							Name:      "ingress",
							Namespace: "gloo-system",
							Labels:    ingressDeploymentLabels,
						},
						Spec: appsv1.DeploymentSpec{
							Replicas: pointer.Int32Ptr(1),
							Selector: &metav1.LabelSelector{MatchLabels: ingressSelector},
							Template: v1.PodTemplateSpec{
								ObjectMeta: metav1.ObjectMeta{
									Labels: ingressPodLabels,
								},
								Spec: v1.PodSpec{
									SecurityContext: &v1.PodSecurityContext{
										RunAsUser:    pointer.Int64Ptr(10101),
										RunAsNonRoot: pointer.BoolPtr(true),
									},
									Containers: []v1.Container{
										{
											Name: "ingress",
											// Note: this WAS overwritten
											Image: "docker.io/ilackarms/ingress:test-ilackarms",
											Env: []v1.EnvVar{
												{
													Name: "POD_NAMESPACE",
													ValueFrom: &v1.EnvVarSource{
														FieldRef: &v1.ObjectFieldSelector{APIVersion: "", FieldPath: "metadata.namespace"},
													},
												},
												{
													Name:  "ENABLE_KNATIVE_INGRESS",
													Value: "true",
												},
												{
													Name:  "KNATIVE_VERSION",
													Value: "0.8.0",
												},
												{
													Name:  "DISABLE_KUBE_INGRESS",
													Value: "true",
												},
											},
											Resources: v1.ResourceRequirements{
												Limits: nil,
											},
											ImagePullPolicy: "Always",
										},
									},
								},
							},
						},
					}
					prepareMakefileFromValuesFile("merge_ingress_values.yaml")
					testManifest.ExpectDeploymentAppsV1(glooDeploymentPostMerge)
					testManifest.ExpectDeploymentAppsV1(ingressDeploymentPostMerge)
				})
			})

			Describe("Deployment Privileges Test", func() {

				// Helper func for testing pod & container root privileges logic
				expectNonRoot := func(testManifest manifesttestutils.TestManifest) {
					deployments := testManifest.SelectResources(func(resource *unstructured.Unstructured) bool {
						return resource.GetKind() == "Deployment"
					})

					Expect(deployments.NumResources()).NotTo(BeZero())

					deployments.ExpectAll(func(resource *unstructured.Unstructured) {
						rawDeploy, err := resource.MarshalJSON()
						Expect(err).NotTo(HaveOccurred())

						deploy := appsv1.Deployment{}
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
				Context("Gloo", func() {
					Context("all cluster-scoped deployments", func() {
						It("is running all deployments with non root user permissions by default", func() {

							prepareMakefile(namespace, helmValues{})

							expectNonRoot(testManifest)
						})

						It("is running all deployments with non root user permissions with knative, accessLogger, ingress, and mTLS enabled", func() {

							prepareMakefile(namespace, helmValues{
								valuesArgs: []string{
									"gateway.enabled=false",
									"settings.integrations.knative.enabled=true",
									"settings.integrations.knative.version=v0.10.0",
									"accessLogger.enabled=true",
									"ingress.enabled=true",
									"global.glooMtls.enabled=true",
								},
							})

							expectNonRoot(testManifest)
						})
					})
				})

			})
		})

		Context("Reflection", func() {
			// Values which, for whatever reason, are excluded from pointer-checking, all of which should
			// include an explanation here for why they're present.
			//  - 3 of the image values are used in our helm generate code, which doesn't like pointers.
			//    We're not changing them for this test, since that code is likely to be removed/changed soon.
			//  - The one exception is the Repository value, which is instead needed by value during codegen.
			var (
				pointerExceptions = map[string]interface{}{}
			)
			It("All non-embedded fields in values.go have the omitempty tag", func() {
				// The following code iterates over each struct in values.go,
				// then iterates over each struct's fields and checks that they either contain the
				// omitempty tag, or don't have tags at all. It also ensures that a variety of primitive types (and strings)
				// are pointers to said types instead of direct values.
				// These changes are necessary to make value deduplication possible between gloo-OS and gloo-E's charts.
				// Without the omitempty tag and pointers, a bunch of problems can crop up. See the slab doc
				// https://soloio.slab.com/posts/helm-chart-merging-issues-r7r2617z for more info.

				// keep track of missing values for final error check
				var missingVals []string

				// All of the structs in values.go form a tree, with the HelmConfig struct as the root.
				structQueue := []reflect.Type{reflect.TypeOf(values.HelmConfig{})}

				// recursively iterate over every child struct of HelmConfig.
				for len(structQueue) > 0 {
					var inspectedStruct reflect.Type
					inspectedStruct, structQueue = structQueue[0], structQueue[1:]
					// for some reason, strings and interfaces slip past my earlier checks (including this same condition
					// in the helper function).
					structName := inspectedStruct.Name()
					if structName == "" || structName == "string" {
						continue
					}
					// iterate over struct fields
					for i := 0; i < inspectedStruct.NumField(); i++ {
						structField := inspectedStruct.Field(i)
						// Check that the field contains a json tag, and if so, that it includes the omitempty tag.
						// Values without any tags are assumed to be embedded structs, and are ignored.
						tagStr, ok := structField.Tag.Lookup("json")
						if ok && !strings.Contains(strings.ToLower(tagStr), "omitempty") {
							fmt.Sprintf("Missing omitempty in %s.%s", inspectedStruct.Name(), structField.Name)
							missingVals = append(missingVals, fmt.Sprintf("{ no omitempty - %s.%s }", inspectedStruct.Name(), structField.Name))
						}

						// Extract the field type, and add it to the structs-to-check queue.
						// The structs can't be cyclic, so don't both with keeping track of what we've seen
						// It's not worth the code clutter to do so for a test.
						fieldType := structField.Type.Kind()
						if fieldType == reflect.Ptr {
							structQueue = appendIfNilPath(structQueue, structField.Type.Elem())
						} else if fieldType == reflect.Array {
							structQueue = appendIfNilPath(structQueue, structField.Type.Elem())
						} else if fieldType == reflect.Map {
							structQueue = append(structQueue, structField.Type.Elem()) // map keys are exclusively strings, this gets the item type
						} else if fieldType == reflect.Struct {
							structQueue = appendIfNilPath(structQueue, structField.Type)
						} else if fieldType == reflect.String ||
							fieldType == reflect.Bool ||
							fieldType == reflect.Int ||
							fieldType == reflect.Int8 ||
							fieldType == reflect.Int32 ||
							fieldType == reflect.Int64 ||
							fieldType == reflect.Uint ||
							fieldType == reflect.Uint32 ||
							fieldType == reflect.Float64 {
							_, found := pointerExceptions[fmt.Sprintf("%s.%s", inspectedStruct.Name(), structField.Name)]
							if !found {
								missingVals = append(missingVals, fmt.Sprintf("{ primitive or simple value not pointer-ed - %s.%s }", inspectedStruct.Name(), structField.Name))
							}
						}
					}
				}
				Expect(fmt.Sprintf("%v", missingVals)).To(Equal("[]")) // this makes the failure message simply be a list of what we're missing
			})
		})
	}

	runTests(allTests)
})

// Helper function that adds a reflected type to a queue if it is a struct from the generate package.
func appendIfNilPath(queue []reflect.Type, newVal reflect.Type) []reflect.Type {
	if newVal.Kind() == reflect.Struct {
		pkgName := newVal.PkgPath()
		if pkgName == "github.com/solo-io/gloo/install/helm/gloo/generate" {
			return append(queue, newVal)
		}
	}
	return queue
}

func cloneMap(input map[string]string) map[string]string {
	ret := map[string]string{}
	for k, v := range input {
		ret[k] = v
	}

	return ret
}

func constructResourceID(resource *unstructured.Unstructured) string {
	// technically vulnerable to resources that have commas in their names, but that's not a big concern
	return fmt.Sprintf("%s,%s,%s", resource.GetNamespace(), resource.GetName(), resource.GroupVersionKind().String())
}
