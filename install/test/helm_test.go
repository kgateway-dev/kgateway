package test

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"reflect"
	"strconv"
	"strings"
	"unicode"

	"github.com/ghodss/yaml"
	"github.com/golang/protobuf/proto"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/format"
	"github.com/onsi/gomega/types"
	values "github.com/solo-io/gloo/install/helm/gloo/generate"
	"github.com/solo-io/gloo/projects/gateway/pkg/defaults"
	"github.com/solo-io/gloo/test/gomega/matchers"
	"github.com/solo-io/k8s-utils/installutils/kuberesource"
	. "github.com/solo-io/k8s-utils/manifesttestutils"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources"
	skprotoutils "github.com/solo-io/solo-kit/pkg/utils/protoutils"
	"github.com/solo-io/solo-kit/pkg/utils/statusutils"
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
func GetValidationEnvVar() v1.EnvVar {
	return v1.EnvVar{
		Name:  "VALIDATION_MUST_START",
		Value: "true",
	}
}
func ConvertKubeResource(unst *unstructured.Unstructured, res resources.Resource) {
	byt, err := unst.MarshalJSON()
	Expect(err).NotTo(HaveOccurred())

	err = skprotoutils.UnmarshalResource(byt, res)
	Expect(err).NotTo(HaveOccurred())
}

var _ = Describe("Helm Test", func() {

	var allTests = func(rendererTestCase renderTestCase) {
		var (
			glooPorts = []v1.ContainerPort{
				{Name: "grpc-xds", ContainerPort: 9977, Protocol: "TCP"},
				{Name: "rest-xds", ContainerPort: 9976, Protocol: "TCP"},
				{Name: "grpc-validation", ContainerPort: 9988, Protocol: "TCP"},
				{Name: "grpc-proxydebug", ContainerPort: 9966, Protocol: "TCP"},
				{Name: "wasm-cache", ContainerPort: 9979, Protocol: "TCP"},
			}
			selector         map[string]string
			testManifest     TestManifest
			renderErr        error
			statsAnnotations map[string]string
		)

		// Create a TestManifest out of the custom resource yaml from the configmap,
		// and then perform assertions on the resources (which are assumed to all be Gateways).
		// `matchers` is a map of Gateway name to the assertion to be made on that Gateway.
		// There must be a matcher for every Gateway that's expected to be in the manifest.
		assertCustomResourceManifest := func(matchers map[string]types.GomegaMatcher) {
			configMap := getConfigMap(testManifest, namespace, customResourceConfigMapName)
			ExpectWithOffset(1, configMap.Data).NotTo(BeNil())
			customResourceYaml := configMap.Data["custom-resources"]
			customResourceManifest := NewTestManifestFromYaml(customResourceYaml)
			// make sure that the number of resources found in the manifest equals the number of
			// matchers passed in, so we can ensure that every resource has an associated matcher
			ExpectWithOffset(1, customResourceManifest.NumResources()).To(Equal(len(matchers)))
			for gwName, matcher := range matchers {
				customResourceManifest.ExpectUnstructured("Gateway", namespace, gwName).To(matcher)
			}
		}

		BeforeEach(func() {
			// Ensure that tests do not share manifests or renderErrs by accident
			testManifest = nil
			renderErr = nil
		})

		Describe(rendererTestCase.rendererName, func() {
			// each entry in valuesArgs should look like `path.to.helm.field=value`
			prepareMakefile := func(namespace string, values helmValues) {
				tm, err := rendererTestCase.renderer.RenderManifest(namespace, values)
				ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Failed to render manifest")
				testManifest = tm
			}

			expectRenderError := func(namespace string, values helmValues) {
				_, err := rendererTestCase.renderer.RenderManifest(namespace, values)
				ExpectWithOffset(1, err).To(HaveOccurred())
				renderErr = err
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

			It("should allow image digest pinning", func() {
				shaTest := "sha256:1234123412341234123412341234213412341234123412341234123412341234"
				prepareMakefile(namespace, helmValues{
					valuesArgs: []string{
						"gloo.deployment.image.digest=" + shaTest,
					},
				})
				testManifest.SelectResources(func(resource *unstructured.Unstructured) bool {
					return resource.GetKind() == "Deployment" && resource.GetName() == "gloo"
				}).ExpectAll(func(deployment *unstructured.Unstructured) {
					deploymentObject, err := kuberesource.ConvertUnstructured(deployment)
					ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Failed to render manifest")
					structuredDeployment, ok := deploymentObject.(*appsv1.Deployment)
					Expect(ok).To(BeTrue(), fmt.Sprintf("Deployment %+v should be able to cast to a structured deployment", deployment))

					containers := structuredDeployment.Spec.Template.Spec.Containers
					Expect(containers).To(HaveLen(1), "should have exactly 1 container")
					image := containers[0].Image
					Expect(image).To(ContainSubstring(shaTest), "should have sha digest in image")
				})
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

			It("does not create the gloo pdb by default", func() {
				prepareMakefile(namespace, helmValues{})

				testManifest.ExpectUnstructured("PodDisruptionBudget", namespace, "gloo-pdb").To(BeNil())
			})

			It("can create gloo pdb with minAvailable", func() {

				prepareMakefile(namespace, helmValues{
					valuesArgs: []string{
						"gloo.podDisruptionBudget.minAvailable='2'",
					},
				})

				pdb := makeUnstructured(`
apiVersion: policy/v1
kind: PodDisruptionBudget
metadata:
  name: gloo-pdb
  namespace: gloo-system
spec:
  minAvailable: '2'
  selector:
    matchLabels:
      gloo: gloo
`)

				testManifest.ExpectUnstructured("PodDisruptionBudget", namespace, "gloo-pdb").To(BeEquivalentTo(pdb))
			})

			It("can create gloo pdb with maxUnavailable", func() {

				prepareMakefile(namespace, helmValues{
					valuesArgs: []string{
						"gloo.podDisruptionBudget.maxUnavailable='2'",
					},
				})

				pdb := makeUnstructured(`
apiVersion: policy/v1
kind: PodDisruptionBudget
metadata:
  name: gloo-pdb
  namespace: gloo-system
spec:
  maxUnavailable: '2'
  selector:
    matchLabels:
      gloo: gloo
`)

				testManifest.ExpectUnstructured("PodDisruptionBudget", namespace, "gloo-pdb").To(BeEquivalentTo(pdb))
			})

			It("errors rendering when gloo pdb has both maxUnavailable and minAvailable set", func() {

				expectRenderError(namespace, helmValues{
					valuesArgs: []string{
						"gloo.podDisruptionBudget.maxUnavailable='2'",
						"gloo.podDisruptionBudget.minAvailable='2'",
					},
				})

				Expect(renderErr).NotTo(BeNil())
				Expect(renderErr.Error()).To(ContainSubstring("gloo PDB values minAvailable and maxUnavailable are mutually exclusive"))
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
					Expect(resourcesTested).To(Equal(8), "Tested %d resources when we were expecting 9."+
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

				Context("should be able to set custom labels and annotations on jobs", func() {
					// creates a template from the specified valuesArgs, then confirms the jobs with the specified names
					// have the expected labels and annotations
					testJobLabelsAndAnnotations := func(valuesArgs []string, jobNames []string, expectedLabels map[string]string, expectedAnnotations map[string]string) {
						// create the chart with the specified values args
						prepareMakefile(namespace, helmValues{
							valuesArgs: valuesArgs,
						})

						// convert the job names we expect to be affected into a map, for easy lookup
						expectedNames := map[string]bool{}
						for _, name := range jobNames {
							expectedNames[name] = true
						}

						testManifest.SelectResources(func(resource *unstructured.Unstructured) bool {
							// get jobs with the expected names
							return resource.GetKind() == "Job" && expectedNames[resource.GetName()]
						}).ExpectAll(func(job *unstructured.Unstructured) {
							// convert the job to a structured object
							jobObject, err := kuberesource.ConvertUnstructured(job)
							ExpectWithOffset(1, err).NotTo(HaveOccurred(), fmt.Sprintf("Job %+v should be able to convert from unstructured", job))
							structuredJob, ok := jobObject.(*jobsv1.Job)
							ExpectWithOffset(1, ok).To(BeTrue(), fmt.Sprintf("Job %+v should be able to cast to a structured job", job))

							// check the labels
							jobLabels := structuredJob.Spec.Template.Labels
							var foundTestValue = false
							for label, expectedValue := range expectedLabels {
								ExpectWithOffset(1, jobLabels[label]).To(Equal(expectedValue), fmt.Sprintf("Job %s expected test label to have"+
									" value %s. Found value %s", job.GetName(), expectedValue, jobLabels[label]))
								foundTestValue = true
							}
							ExpectWithOffset(1, foundTestValue).To(Equal(true), fmt.Sprintf("Coundn't find test label 'foo' in job %s", job.GetName()))

							// check the annotations
							jobAnnotations := structuredJob.Spec.Template.Annotations
							foundTestValue = false
							for label, expectedValue := range expectedAnnotations {
								ExpectWithOffset(1, jobAnnotations[label]).To(Equal(expectedValue), fmt.Sprintf("Job %s expected test annotation to have"+
									" value %s. Found value %s", job.GetName(), expectedValue, jobAnnotations[label]))
								foundTestValue = true
							}
							ExpectWithOffset(1, foundTestValue).To(Equal(true), fmt.Sprintf("Coundn't find test annotation 'foo2' in job %s", job.GetName()))
						})
					}

					It("should be able to set custom labels and annotations on certGenJob", func() {
						values := []string{
							"gateway.certGenJob.extraPodLabels.foo=bar",
							"gateway.certGenJob.extraPodAnnotations.foo2=baz",
						}
						jobNames := []string{
							"gloo-mtls-certgen-cronjob",
							"gloo-mtls-certgen",
							"gateway-certgen",
							"gateway-certgen-cronjob",
						}

						expectedLabels := map[string]string{
							"foo": "bar",
						}

						expectedAnnotations := map[string]string{
							"foo2": "baz",
						}

						testJobLabelsAndAnnotations(values, jobNames, expectedLabels, expectedAnnotations)

					})

					It("should be able to set custom labels and annotations on cleanupJob", func() {
						values := []string{
							"gateway.cleanupJob.extraPodLabels.foo=bar",
							"gateway.cleanupJob.extraPodAnnotations.foo2=baz",
						}

						jobNames := []string{
							"gloo-resource-cleanup",
						}

						expectedLabels := map[string]string{
							"foo": "bar",
						}

						expectedAnnotations := map[string]string{
							"foo2": "baz",
						}

						testJobLabelsAndAnnotations(values, jobNames, expectedLabels, expectedAnnotations)
					})

					It("should be able to set custom labels and annotations on rolloutJob", func() {
						values := []string{
							"gateway.rolloutJob.extraPodLabels.foo=bar",
							"gateway.rolloutJob.extraPodAnnotations.foo2=baz",
						}

						jobNames := []string{
							"gloo-resource-rollout",
							"gloo-resource-migration",
						}

						expectedLabels := map[string]string{
							"foo": "bar",
						}

						expectedAnnotations := map[string]string{
							"foo2": "baz",
						}

						testJobLabelsAndAnnotations(values, jobNames, expectedLabels, expectedAnnotations)
					})
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

				deploymentContainsMonitoringPort := func(deployment *appsv1.Deployment) bool {
					for _, container := range deployment.Spec.Template.Spec.Containers {
						for _, containerPort := range container.Ports {
							if containerPort.Name == "http-monitoring" {
								return true
							}
						}
					}
					return false
				}

				serviceContainsMonitoringPort := func(service *v1.Service) bool {
					for _, servicePort := range service.Spec.Ports {
						if servicePort.Name == "http-monitoring" {
							return true
						}
					}
					return false
				}

				It("should be able to override global defaults", func() {
					prepareMakefile(namespace, helmValues{
						valuesArgs: []string{
							"discovery.deployment.stats.enabled=true",
							"discovery.deployment.stats.podMonitorEnabled=true",

							"global.glooStats.enabled=false",
							"global.glooStats.podMonitorEnabled=false",
						},
					})

					// assert that discovery has stats enabled
					testManifest.SelectResources(func(resource *unstructured.Unstructured) bool {
						return resource.GetKind() == "Deployment" && resource.GetName() == "discovery"
					}).ExpectAll(func(deployment *unstructured.Unstructured) {
						deploymentObject, err := kuberesource.ConvertUnstructured(deployment)
						ExpectWithOffset(1, err).NotTo(HaveOccurred(), fmt.Sprintf("Deployment %+v should be able to convert from unstructured", deployment))
						structuredDeployment, ok := deploymentObject.(*appsv1.Deployment)
						ExpectWithOffset(1, ok).To(BeTrue(), fmt.Sprintf("Deployment %+v should be able to cast to a structured deployment", deployment))

						for annotation, value := range normalPromAnnotations {
							ExpectWithOffset(1, structuredDeployment.Spec.Template.ObjectMeta.Annotations[annotation]).To(Equal(value), fmt.Sprintf("Annotation %s should be set to %s on deployment %+v", deployment, annotation, value))
						}

						foundMonitoringPort := deploymentContainsMonitoringPort(structuredDeployment)
						ExpectWithOffset(1, foundMonitoringPort).To(BeTrue(), fmt.Sprintf("'http-monitoring' port should be set on deployment %+v", deployment))

					})

					// assert that gloo has stats disabled
					testManifest.SelectResources(func(resource *unstructured.Unstructured) bool {
						return resource.GetKind() == "Deployment" && resource.GetName() == "gloo"
					}).ExpectAll(func(deployment *unstructured.Unstructured) {
						deploymentObject, err := kuberesource.ConvertUnstructured(deployment)
						ExpectWithOffset(1, err).NotTo(HaveOccurred(), fmt.Sprintf("Deployment %+v should be able to convert from unstructured", deployment))
						structuredDeployment, ok := deploymentObject.(*appsv1.Deployment)
						ExpectWithOffset(1, ok).To(BeTrue(), fmt.Sprintf("Deployment %+v should be able to cast to a structured deployment", deployment))

						ExpectWithOffset(1, structuredDeployment.Spec.Template.ObjectMeta.Annotations).To(BeEmpty(), fmt.Sprintf("No annotations should be present on deployment %+v", structuredDeployment))

						foundMonitoringPort := deploymentContainsMonitoringPort(structuredDeployment)
						ExpectWithOffset(1, foundMonitoringPort).To(BeFalse(), fmt.Sprintf("'http-monitoring' port should not be set on deployment %+v", deployment))
					})
				})

				It("should be able to expose http-monitoring port on all relevant services", func() {
					prepareMakefile(namespace, helmValues{
						valuesArgs: []string{
							// to enable accessLogger service
							"gateway.enabled=true",
							"accessLogger.enabled=true",
							"accessLogger.stats.serviceMonitorEnabled=true",

							"global.glooStats.enabled=true",
							"global.glooStats.serviceMonitorEnabled=true",
						},
					})

					expectedServicesWithHttpMonitoring := []string{
						"gloo",
						"discovery",
						"gateway-proxy-access-logger",
						"gateway-proxy-monitoring-service",
					}
					var actualServicesWithHttpMonitoring []string

					testManifest.SelectResources(func(resource *unstructured.Unstructured) bool {
						return resource.GetKind() == "Service"
					}).ExpectAll(func(service *unstructured.Unstructured) {
						serviceObject, err := kuberesource.ConvertUnstructured(service)
						ExpectWithOffset(1, err).NotTo(HaveOccurred(), fmt.Sprintf("Service %+v should be able to convert from unstructured", service))
						structuredService, ok := serviceObject.(*v1.Service)
						ExpectWithOffset(1, ok).To(BeTrue(), fmt.Sprintf("Service %+v should be able to cast to a structured service", service))
						ExpectWithOffset(1, structuredService.Labels["app"]).To(Equal("gloo"), "Service has app=gloo label")

						if serviceContainsMonitoringPort(structuredService) {
							actualServicesWithHttpMonitoring = append(actualServicesWithHttpMonitoring, structuredService.GetName())
						}
					})

					Expect(actualServicesWithHttpMonitoring).To(Equal(expectedServicesWithHttpMonitoring))
				})

				It("should be able to expose http-monitoring port on all relevant deployments", func() {
					prepareMakefile(namespace, helmValues{
						valuesArgs: []string{
							// to enable accessLogger deployment
							"gateway.enabled=true",
							"accessLogger.enabled=true",
							"accessLogger.stats.podMonitorEnabled=true",

							"global.glooStats.enabled=true",
							"global.glooStats.podMonitorEnabled=true",
						},
					})

					expectedDeploymentsWithHttpMonitoring := []string{
						"gloo",
						"discovery",
						"gateway-proxy-access-logger",
						"gateway-proxy",
					}
					var actualDeploymentsWithHttpMonitoring []string

					testManifest.SelectResources(func(resource *unstructured.Unstructured) bool {
						return resource.GetKind() == "Deployment"
					}).ExpectAll(func(deployment *unstructured.Unstructured) {
						deploymentObject, err := kuberesource.ConvertUnstructured(deployment)
						ExpectWithOffset(1, err).NotTo(HaveOccurred(), fmt.Sprintf("Deployment %+v should be able to convert from unstructured", deployment))
						structuredDeployment, ok := deploymentObject.(*appsv1.Deployment)
						ExpectWithOffset(1, ok).To(BeTrue(), fmt.Sprintf("Deployment %+v should be able to cast to a structured deployment", deployment))
						ExpectWithOffset(1, structuredDeployment.Labels["app"]).To(Equal("gloo"), "Deployment has app=gloo label")

						if deploymentContainsMonitoringPort(structuredDeployment) {
							actualDeploymentsWithHttpMonitoring = append(actualDeploymentsWithHttpMonitoring, structuredDeployment.GetName())
						}
					})

					Expect(actualDeploymentsWithHttpMonitoring).To(Equal(expectedDeploymentsWithHttpMonitoring))
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
								DefaultMode: proto.Int32(420),
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

				It("should set stats_config in gateway-proxy-envoy-config from gatewayProxies.gatewayProxy.envoyStatsConfig", func() {
					prepareMakefile(namespace, helmValues{
						valuesArgs: []string{
							"gatewayProxies.gatewayProxy.envoyStatsConfig.stats_tags[0].fixed_value=zero",
							"gatewayProxies.gatewayProxy.envoyStatsConfig.stats_tags[0].tag_name=one",
						},
					})

					resources := testManifest.SelectResources(func(resource *unstructured.Unstructured) bool {
						return resource.GetKind() == "ConfigMap" && resource.GetName() == "gateway-proxy-envoy-config"
					})

					Expect(resources.NumResources()).To(Equal(1), "Should find only one gateway-proxy-envoy-config ConfigMap")

					resources.ExpectAll(func(configMap *unstructured.Unstructured) {
						configMapObject, err := kuberesource.ConvertUnstructured(configMap)
						Expect(err).NotTo(HaveOccurred(), fmt.Sprintf("ConfigMap %+v should be able to convert from unstructured", configMap))
						structuredConfigMap, ok := configMapObject.(*v1.ConfigMap)
						Expect(ok).To(BeTrue(), fmt.Sprintf("ConfigMap %+v should be able to cast to a structured config map", configMap))

						expect := Expect(structuredConfigMap.Data["envoy.yaml"])
						expect.To(ContainSubstring("stats_config:"))
						expect.To(ContainSubstring("stats_tags:"))
						expect.To(ContainSubstring("- fixed_value: zero"))
					})
				})

				It("Should have 'pre-install' and 'pre-upgrade' hook if gateway.certGenJob.runOnUpdate is true", func() {
					prepareMakefile(namespace, helmValues{
						valuesArgs: []string{
							"global.glooMtls.enabled=true",
							"gateway.certGenJob.runOnUpdate=true"},
					})

					testManifest.SelectResources(func(resource *unstructured.Unstructured) bool {
						return resource.GetKind() == "Job"
					}).ExpectAll(func(job *unstructured.Unstructured) {
						jobObject, err := kuberesource.ConvertUnstructured(job)
						Expect(err).NotTo(HaveOccurred(), fmt.Sprintf("Job %+v should be able to convert from unstructured", job))
						structuredDeployment, ok := jobObject.(*jobsv1.Job)
						Expect(ok).To(BeTrue(), fmt.Sprintf("Job %+v should be able to cast to a structured job", job))

						if structuredDeployment.GetName() == "gloo-mtls-certgen" {
							for annotation_name, annotation_value := range structuredDeployment.GetAnnotations() {
								if annotation_name == "helm.sh/hook" {
									Expect(annotation_value).To(Equal("pre-install, pre-upgrade"))
								}
							}
						}
					})
				})

				It("Should have only 'pre-install' hook if gateway.certGenJob.runOnUpdate is false", func() {
					prepareMakefile(namespace, helmValues{
						valuesArgs: []string{
							"global.glooMtls.enabled=true",
							"gateway.certGenJob.runOnUpdate=false"},
					})

					testManifest.SelectResources(func(resource *unstructured.Unstructured) bool {
						return resource.GetKind() == "Job"
					}).ExpectAll(func(job *unstructured.Unstructured) {
						jobObject, err := kuberesource.ConvertUnstructured(job)
						Expect(err).NotTo(HaveOccurred(), fmt.Sprintf("Job %+v should be able to convert from unstructured", job))
						structuredDeployment, ok := jobObject.(*jobsv1.Job)
						Expect(ok).To(BeTrue(), fmt.Sprintf("Job %+v should be able to cast to a structured job", job))

						if structuredDeployment.GetName() == "gloo-mtls-certgen" {
							for annotation_name, annotation_value := range structuredDeployment.GetAnnotations() {
								if annotation_name == "helm.sh/hook" {
									Expect(annotation_value).To(Equal("pre-install"))
								}
							}
						}
					})
				})
			})

			Context("gloo with linkerd settings", func() {
				var (
					linkerdInjectionLabel = "linkerd.io/inject"
				)
				It("linkerd injection should always be disabled in job pod templates when linkerd is enabled", func() {
					prepareMakefile(namespace, helmValues{
						valuesArgs: []string{
							"settings.linkerd=true",
						},
					})
					testManifest.SelectResources(func(resource *unstructured.Unstructured) bool {
						return resource.GetKind() == "Job"
					}).ExpectAll(func(job *unstructured.Unstructured) {
						jobObj, err := kuberesource.ConvertUnstructured(job)
						Expect(err).NotTo(HaveOccurred(), fmt.Sprintf("Job %+v should be able to convert from unstructured", job))
						structuredJob, ok := jobObj.(*jobsv1.Job)
						Expect(ok).To(BeTrue(), fmt.Sprintf("Job %+v should be able to cast to a structured job", job))

						val, ok := structuredJob.Spec.Template.ObjectMeta.Annotations[linkerdInjectionLabel]
						Expect(ok).To(BeTrue(), fmt.Sprintf("Job %s should contain an linkerd injection annotation", job.GetName()))
						Expect(val).To(Equal("disabled"), fmt.Sprintf("Job %s should have an linkerd injection annotation with value of 'disabled'", job.GetName()))
					})
				})
			})

			Context("gloo with istio sds settings", func() {
				var (
					IstioInjectionLabel          = "sidecar.istio.io/inject"
					istioExcludedPortsAnnotation = "traffic.sidecar.istio.io/excludeInboundPorts"
					IstioRevisionLabel           = "istio.io/rev"
					istioCertsVolume             = v1.Volume{
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
							Ω(istioSidecarVersion(structuredDeployment.Spec.Template.Spec.Containers)).To(Equal("docker.io/istio/proxyv2:1.9.5"), "istio proxy sidecar should be the default")
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

				It("should add an anti-injection label to gateway-proxy pods when disableAutoinjection is enabled", func() {
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
						deploymentName := deployment.GetName()
						deploymentObject, err := kuberesource.ConvertUnstructured(deployment)
						Expect(err).NotTo(HaveOccurred(), fmt.Sprintf("Deployment %+v should be able to convert from unstructured", deployment))
						structuredDeployment, ok := deploymentObject.(*appsv1.Deployment)
						Expect(ok).To(BeTrue(), fmt.Sprintf("Deployment %+v should be able to cast to a structured deployment", deployment))

						if deploymentName == "gateway-proxy" {
							// ensure every deployment has a istio annotation set to false
							val, ok := structuredDeployment.Spec.Template.ObjectMeta.Labels[IstioInjectionLabel]
							Expect(ok).To(BeTrue(), fmt.Sprintf("Deployment %s should contain an istio injection label", deploymentName))
							Expect(val).To(Equal("false"), fmt.Sprintf("Deployment %s should have an istio annotation with value of 'false'", deploymentName))
						}
					})
				})

				It("should add an Istio injection label for pods that can be configured for it", func() {
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

						// Ensure that the discovery pod has a true label, gateway-proxy has a false label (default), and nothing else has any annoation.
						// todo if we ever decide to add more pods to the list of 'allow istio injection' pods, then change this to a whitelist check
						if structuredDeployment.GetName() == "discovery" {
							val, ok := structuredDeployment.Spec.Template.ObjectMeta.Labels[IstioInjectionLabel]
							Expect(ok).To(BeTrue(), fmt.Sprintf("Deployment %s should contain an istio injection annotation", deployment.GetName()))
							Expect(val).To(Equal("true"), fmt.Sprintf("Deployment %s should have an istio annotation with value of 'true'", deployment.GetName()))
							_, ok = structuredDeployment.Spec.Template.ObjectMeta.Labels[IstioRevisionLabel]
							Expect(ok).To(BeFalse(), fmt.Sprintf("Deployment %s should not contain an istio revision annotation", deployment.GetName()))
						} else {
							_, ok := structuredDeployment.Spec.Template.ObjectMeta.Labels[IstioInjectionLabel]
							Expect(ok).To(BeFalse(), fmt.Sprintf("Deployment %s should not contain an istio injection label", deployment.GetName()))
						}
					})
				})

				It("should add an Istio injection label for pods that can be configured for it", func() {
					httpPort := 8080
					httpsPort := 8443
					secondDeploymentHttpPort := 1337
					secondDeploymentHttpsPort := 1338
					prepareMakefile(namespace, helmValues{
						valuesArgs: []string{"global.istioIntegration.whitelistDiscovery=true",
							"global.istioIntegration.enableIstioSidecarOnGateway=true",
							fmt.Sprintf("gatewayProxies.gatewayProxy.podTemplate.httpPort=%d", httpPort),
							fmt.Sprintf("gatewayProxies.gatewayProxy.podTemplate.httpsPort=%d", httpsPort),
							// shortcut to get a proxy with a name and mostly default values but avoid hiding any bugs around needing to set disabled=false
							fmt.Sprintf("gatewayProxies.secondGatewayProxy.podTemplate.httpPort=%d", secondDeploymentHttpPort),
							fmt.Sprintf("gatewayProxies.secondGatewayProxy.podTemplate.httpsPort=%d", secondDeploymentHttpsPort),
						},
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
						deploymentName := structuredDeployment.GetName()
						if deploymentName == "discovery" {
							val, ok := structuredDeployment.Spec.Template.ObjectMeta.Labels[IstioInjectionLabel]
							Expect(ok).To(BeTrue(), fmt.Sprintf("Deployment %s should contain an istio injection annotation", deployment.GetName()))
							Expect(val).To(Equal("true"), fmt.Sprintf("Deployment %s should have an istio annotation with value of 'true'", deployment.GetName()))
						} else if deploymentName == "gateway-proxy" {
							_, ok := structuredDeployment.Spec.Template.ObjectMeta.Labels[IstioInjectionLabel]
							Expect(ok).To(BeTrue(), fmt.Sprintf("Deployment %s should contain an istio injection annotation", deploymentName))
							excludedPortString, ok := structuredDeployment.Spec.Template.ObjectMeta.Annotations[istioExcludedPortsAnnotation]
							Expect(ok).To(BeTrue())
							Expect(excludedPortString).To(Equal(fmt.Sprintf("%d,%d", httpPort, httpsPort)), fmt.Sprintf("Deployment %s should exclude specified ports", deploymentName))
						} else if deploymentName == "second-gateway-proxy" {
							_, ok := structuredDeployment.Spec.Template.ObjectMeta.Labels[IstioInjectionLabel]
							Expect(ok).To(BeTrue(), fmt.Sprintf("Deployment %s should contain an istio injection annotation", deploymentName))
							excludedPortString, ok := structuredDeployment.Spec.Template.ObjectMeta.Annotations[istioExcludedPortsAnnotation]
							Expect(ok).To(BeTrue())
							Expect(excludedPortString).To(Equal(fmt.Sprintf("%d,%d", secondDeploymentHttpPort, secondDeploymentHttpsPort)), fmt.Sprintf("Deployment %s should exclude specified ports", deploymentName))
						} else if deploymentName == "named-gateway-proxy" {
							excludedPortString, ok := structuredDeployment.Spec.Template.ObjectMeta.Annotations[istioExcludedPortsAnnotation]
							Expect(ok).To(BeTrue())
							Expect(excludedPortString).To(Equal(fmt.Sprintf("%d,%d", httpPort, httpsPort)), fmt.Sprintf("Deployment %s should exclude the same specified ports as the default", deploymentName))
						}
					})
				})

				It("should add an Istio revision label for pods that can be configured for it", func() {
					httpPort := 8080
					httpsPort := 8443
					secondDeploymentHttpPort := 1337
					secondDeploymentHttpsPort := 1338
					prepareMakefile(namespace, helmValues{
						valuesArgs: []string{"global.istioIntegration.whitelistDiscovery=true",
							"global.istioIntegration.enableIstioSidecarOnGateway=true",
							"global.istioIntegration.istioSidecarRevTag=some-revision",
							fmt.Sprintf("gatewayProxies.gatewayProxy.podTemplate.httpPort=%d", httpPort),
							fmt.Sprintf("gatewayProxies.gatewayProxy.podTemplate.httpsPort=%d", httpsPort),
							// shortcut to get a proxy with a name and mostly default values but avoid hiding any bugs around needing to set disabled=false
							fmt.Sprintf("gatewayProxies.secondGatewayProxy.podTemplate.httpPort=%d", secondDeploymentHttpPort),
							fmt.Sprintf("gatewayProxies.secondGatewayProxy.podTemplate.httpsPort=%d", secondDeploymentHttpsPort),
						},
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
						deploymentName := structuredDeployment.GetName()
						if deploymentName == "discovery" {
							val, ok := structuredDeployment.Spec.Template.ObjectMeta.Labels[IstioInjectionLabel]
							Expect(ok).To(BeFalse(), fmt.Sprintf("Deployment %s should contain an istio injection annotation", deployment.GetName()))
							val, ok = structuredDeployment.Spec.Template.ObjectMeta.Labels[IstioRevisionLabel]
							Expect(ok).To(BeTrue(), fmt.Sprintf("Deployment %s should contain an istio revision annotation", deployment.GetName()))
							Expect(val).To(Equal("some-revision"), fmt.Sprintf("Deployment %s should have a label for the istio revision tag", deployment.GetName()))
						} else if deploymentName == "gateway-proxy" {
							_, ok := structuredDeployment.Spec.Template.ObjectMeta.Labels[IstioInjectionLabel]
							Expect(ok).To(BeFalse(), fmt.Sprintf("Deployment %s should not contain a default istio injection annotation", deploymentName))
							val, ok := structuredDeployment.Spec.Template.ObjectMeta.Labels[IstioRevisionLabel]
							Expect(ok).To(BeTrue(), fmt.Sprintf("Deployment %s should contain an istio revision annotation", deployment.GetName()))
							Expect(val).To(Equal("some-revision"), fmt.Sprintf("Deployment %s should have a label for the istio revision tag", deployment.GetName()))
							excludedPortString, ok := structuredDeployment.Spec.Template.ObjectMeta.Annotations[istioExcludedPortsAnnotation]
							Expect(ok).To(BeTrue())
							Expect(excludedPortString).To(Equal(fmt.Sprintf("%d,%d", httpPort, httpsPort)), fmt.Sprintf("Deployment %s should exclude specified ports", deploymentName))
						} else if deploymentName == "second-gateway-proxy" {
							_, ok := structuredDeployment.Spec.Template.ObjectMeta.Labels[IstioInjectionLabel]
							Expect(ok).To(BeFalse(), fmt.Sprintf("Deployment %s should not contain an istio injection annotation", deploymentName))
							val, ok := structuredDeployment.Spec.Template.ObjectMeta.Labels[IstioRevisionLabel]
							Expect(ok).To(BeTrue(), fmt.Sprintf("Deployment %s should contain an istio revision annotation", deployment.GetName()))
							Expect(val).To(Equal("some-revision"), fmt.Sprintf("Deployment %s should have a label for the istio revision tag", deployment.GetName()))
							excludedPortString, ok := structuredDeployment.Spec.Template.ObjectMeta.Annotations[istioExcludedPortsAnnotation]
							Expect(ok).To(BeTrue())
							Expect(excludedPortString).To(Equal(fmt.Sprintf("%d,%d", secondDeploymentHttpPort, secondDeploymentHttpsPort)), fmt.Sprintf("Deployment %s should exclude specified ports", deploymentName))
						} else if deploymentName == "named-gateway-proxy" {
							_, ok := structuredDeployment.Spec.Template.ObjectMeta.Labels[IstioInjectionLabel]
							Expect(ok).To(BeFalse(), fmt.Sprintf("Deployment %s should not contain an istio injection annotation", deploymentName))
							val, ok := structuredDeployment.Spec.Template.ObjectMeta.Labels[IstioRevisionLabel]
							Expect(ok).To(BeTrue(), fmt.Sprintf("Deployment %s should contain an istio revision annotation", deployment.GetName()))
							Expect(val).To(Equal("some-revision"), fmt.Sprintf("Deployment %s should have a label for the istio revision tag", deployment.GetName()))
							excludedPortString, ok := structuredDeployment.Spec.Template.ObjectMeta.Annotations[istioExcludedPortsAnnotation]
							Expect(ok).To(BeTrue())
							Expect(excludedPortString).To(Equal(fmt.Sprintf("%d,%d", httpPort, httpsPort)), fmt.Sprintf("Deployment %s should exclude the same specified ports as the default", deploymentName))
						}
					})
				})
				It("istio injection should always be disabled in job pod templates", func() {
					prepareMakefile(namespace, helmValues{
						valuesArgs: []string{
							"global.istioIntegration.disableAutoinjection=false",
							"global.glooMtls.enabled=true",
						},
					})

					testManifest.SelectResources(func(resource *unstructured.Unstructured) bool {
						return resource.GetKind() == "Job"
					}).ExpectAll(func(job *unstructured.Unstructured) {
						jobObj, err := kuberesource.ConvertUnstructured(job)
						Expect(err).NotTo(HaveOccurred(), fmt.Sprintf("Job %+v should be able to convert from unstructured", job))
						structuredJob, ok := jobObj.(*jobsv1.Job)
						Expect(ok).To(BeTrue(), fmt.Sprintf("Job %+v should be able to cast to a structured job", job))

						val, ok := structuredJob.Spec.Template.ObjectMeta.Labels[IstioInjectionLabel]
						Expect(ok).To(BeTrue(), fmt.Sprintf("Job %s should contain an istio injection annotation", job.GetName()))
						Expect(val).To(Equal("false"), fmt.Sprintf("Job %s should have an istio annotation with value of 'false'", job.GetName()))
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
				It("The created namespace can be labeled with a revision tag for Istio discovery", func() {
					prepareMakefile(namespace, helmValues{
						valuesArgs: []string{"namespace.create=true",
							"global.istioIntegration.labelInstallNamespace=true",
							"global.istioIntegration.istioSidecarRevTag=some-revision"},
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
							_, ok := structuredNamespace.ObjectMeta.Labels["istio-injection"]
							Expect(ok).To(BeFalse(), fmt.Sprintf("Namespace %s should not contain a generic Istio discovery label", structuredNamespace.GetName()))
							val, ok := structuredNamespace.ObjectMeta.Labels["istio.io/rev"]
							Expect(ok).To(BeTrue(), fmt.Sprintf("Namespace %s should contain a revisioned Istio discovery label", structuredNamespace.GetName()))
							Expect(val).To(Equal("some-revision"))
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
					// Gateways are not directly included in the helm manifest anymore; instead, their contents are
					// stored in a ConfigMap, and the resource rollout job mounts the ConfigMap as a file and applies
					// the file via kubectl.
					// In these tests, to confirm the existence of the Gateway CRs, we look for the gateway in the
					// ConfigMap data.

					gw := makeUnstructuredGateway(namespace, defaults.GatewayProxyName, false)
					gwSsl := makeUnstructuredGateway(namespace, defaults.GatewayProxyName, true)

					It("does not render when gatewaySettings is disabled", func() {
						// by default, gateway-proxy should render
						prepareMakefile(namespace, helmValues{})
						assertCustomResourceManifest(map[string]types.GomegaMatcher{
							defaults.GatewayProxyName:                    BeEquivalentTo(gw),
							getSslGatewayName(defaults.GatewayProxyName): BeEquivalentTo(gwSsl),
						})

						// if explicitly setting enabled=true, gateway-proxy should render
						prepareMakefile(namespace, helmValues{
							valuesArgs: []string{"gatewayProxies.gatewayProxy.gatewaySettings.enabled=true"},
						})
						assertCustomResourceManifest(map[string]types.GomegaMatcher{
							defaults.GatewayProxyName:                    BeEquivalentTo(gw),
							getSslGatewayName(defaults.GatewayProxyName): BeEquivalentTo(gwSsl),
						})

						// if explicitly setting enabled=false, gateway-proxy should not render
						prepareMakefile(namespace, helmValues{
							valuesArgs: []string{"gatewayProxies.gatewayProxy.gatewaySettings.enabled=false"},
						})
						assertCustomResourceManifest(map[string]types.GomegaMatcher{})
					})

					It("does not render when gatewayProxy is disabled", func() {
						// by default, gateway-proxy should render
						prepareMakefile(namespace, helmValues{})
						assertCustomResourceManifest(map[string]types.GomegaMatcher{
							defaults.GatewayProxyName:                    BeEquivalentTo(gw),
							getSslGatewayName(defaults.GatewayProxyName): BeEquivalentTo(gwSsl),
						})

						// if explicitly setting disabled=false, gateway-proxy should render
						prepareMakefile(namespace, helmValues{
							valuesArgs: []string{"gatewayProxies.gatewayProxy.disabled=false"},
						})
						assertCustomResourceManifest(map[string]types.GomegaMatcher{
							defaults.GatewayProxyName:                    BeEquivalentTo(gw),
							getSslGatewayName(defaults.GatewayProxyName): BeEquivalentTo(gwSsl),
						})

						// if explicitly setting disabled=true, gateway-proxy should not render
						prepareMakefile(namespace, helmValues{
							valuesArgs: []string{"gatewayProxies.gatewayProxy.disabled=true"},
						})
						assertCustomResourceManifest(map[string]types.GomegaMatcher{})
					})

					It("renders custom gateway when gatewayProxy is disabled", func() {
						// when explicitly disabling both gateways, neither should render
						prepareMakefile(namespace, helmValues{
							valuesArgs: []string{
								"gatewayProxies.gatewayProxy.disabled=true",
								"gatewayProxies.anotherGatewayProxy.disabled=true",
							},
						})
						assertCustomResourceManifest(map[string]types.GomegaMatcher{})

						// when disabling default gateway and enabling custom gateway, only custom gateway should render
						prepareMakefile(namespace, helmValues{
							valuesArgs: []string{
								"gatewayProxies.gatewayProxy.disabled=true",
								"gatewayProxies.anotherGatewayProxy.disabled=false",
							},
						})
						anotherGw := makeUnstructuredGateway(namespace, "another-gateway-proxy", false)
						anotherGwSsl := makeUnstructuredGateway(namespace, "another-gateway-proxy", true)
						assertCustomResourceManifest(map[string]types.GomegaMatcher{
							"another-gateway-proxy":     BeEquivalentTo(anotherGw),
							"another-gateway-proxy-ssl": BeEquivalentTo(anotherGwSsl),
						})
					})

					It("renders custom gateway when gatewayProxy is disabled and custom disabled is not set", func() {
						prepareMakefile(namespace, helmValues{
							valuesArgs: []string{
								"gatewayProxies.gatewayProxy.disabled=true",
								// anotherGatewayProxy should exist but not have a value for disabled
								"gatewayProxies.anotherGatewayProxy.loopbackAddress=127.0.0.1",
							},
						})
						anotherGw := makeUnstructuredGateway(namespace, "another-gateway-proxy", false)
						anotherGwSsl := makeUnstructuredGateway(namespace, "another-gateway-proxy", true)
						assertCustomResourceManifest(map[string]types.GomegaMatcher{
							"another-gateway-proxy":     BeEquivalentTo(anotherGw),
							"another-gateway-proxy-ssl": BeEquivalentTo(anotherGwSsl),
						})
					})

					It("does not overwrite nodeSelectors specified for custom gateway proxy", func() {
						prepareMakefile(namespace, helmValues{
							valuesArgs: []string{
								"gatewayProxies.gatewayProxy.podTemplate.nodeSelector.default=default",
								// anotherGatewayProxy should have its own nodeSelector and not the default
								"gatewayProxies.anotherGatewayProxy.podTemplate.nodeSelector.custom=custom",
							},
						})
						gwpUns := testManifest.ExpectCustomResource("Deployment", namespace, "another-gateway-proxy")
						gwp, err := kuberesource.ConvertUnstructured(gwpUns)
						Expect(err).NotTo(HaveOccurred())
						Expect(gwp).To(BeAssignableToTypeOf(&appsv1.Deployment{}))
						gwpStr := *gwp.(*appsv1.Deployment)
						Expect(gwpStr.Spec.Template.Spec.NodeSelector).To(Equal(map[string]string{"custom": "custom"}))
					})

					It("uses default nodeSelectors for custom gateway proxy when none is specified", func() {
						prepareMakefile(namespace, helmValues{
							valuesArgs: []string{
								"gatewayProxies.gatewayProxy.podTemplate.nodeSelector.default=default",
								// anotherGatewayProxy should get the default nodeSelector
								"gatewayProxies.anotherGatewayProxy.loopbackAddress=127.0.0.1",
							},
						})
						gwpUns := testManifest.ExpectCustomResource("Deployment", namespace, "another-gateway-proxy")
						gwp, err := kuberesource.ConvertUnstructured(gwpUns)
						Expect(err).NotTo(HaveOccurred())
						Expect(gwp).To(BeAssignableToTypeOf(&appsv1.Deployment{}))
						gwpStr := *gwp.(*appsv1.Deployment)
						Expect(gwpStr.Spec.Template.Spec.NodeSelector).To(Equal(map[string]string{"default": "default"}))
					})

					It("uses appropriate nodeSelectors for custom gateway proxies depending on whether any is specified", func() {
						prepareMakefile(namespace, helmValues{
							valuesArgs: []string{
								// unspecifiedGatewayProxy should get the default nodeSelector
								"gatewayProxies.unspecifiedGatewayProxy.loopbackAddress=127.0.0.1",
								// specifiedGatewayProxy should keep its specified nodeSelector
								"gatewayProxies.specifiedGatewayProxy.podTemplate.nodeSelector.custom=custom",
								// default specified last to catch accidental overwriting
								"gatewayProxies.gatewayProxy.podTemplate.nodeSelector.default=default",
							},
						})

						unspecifiedUns := testManifest.ExpectCustomResource("Deployment", namespace, "unspecified-gateway-proxy")
						unspecified, err := kuberesource.ConvertUnstructured(unspecifiedUns)
						Expect(err).NotTo(HaveOccurred())
						Expect(unspecified).To(BeAssignableToTypeOf(&appsv1.Deployment{}))
						unspecifiedStr := *unspecified.(*appsv1.Deployment)
						Expect(unspecifiedStr.Spec.Template.Spec.NodeSelector).To(Equal(map[string]string{"default": "default"}))

						specifiedUns := testManifest.ExpectCustomResource("Deployment", namespace, "specified-gateway-proxy")
						specified, err := kuberesource.ConvertUnstructured(specifiedUns)
						Expect(err).NotTo(HaveOccurred())
						Expect(specified).To(BeAssignableToTypeOf(&appsv1.Deployment{}))
						specifiedStr := *specified.(*appsv1.Deployment)
						Expect(specifiedStr.Spec.Template.Spec.NodeSelector).To(Equal(map[string]string{"custom": "custom"}))

						gwpUns := testManifest.ExpectCustomResource("Deployment", namespace, "gateway-proxy")
						gwp, err := kuberesource.ConvertUnstructured(gwpUns)
						Expect(err).NotTo(HaveOccurred())
						Expect(gwp).To(BeAssignableToTypeOf(&appsv1.Deployment{}))
						gwpStr := *gwp.(*appsv1.Deployment)
						Expect(gwpStr.Spec.Template.Spec.NodeSelector).To(Equal(map[string]string{"default": "default"}))
					})

					It("renders with http/https gateways by default", func() {
						prepareMakefile(namespace, helmValues{})
						assertCustomResourceManifest(map[string]types.GomegaMatcher{
							defaults.GatewayProxyName:                    BeEquivalentTo(gw),
							getSslGatewayName(defaults.GatewayProxyName): BeEquivalentTo(gwSsl),
						})
					})

					It("can disable rendering http/https gateways", func() {
						prepareMakefile(namespace, helmValues{
							valuesArgs: []string{"gatewayProxies.gatewayProxy.gatewaySettings.disableGeneratedGateways=true"},
						})
						assertCustomResourceManifest(map[string]types.GomegaMatcher{})
					})

					It("can disable http gateway", func() {
						prepareMakefile(namespace, helmValues{
							valuesArgs: []string{"gatewayProxies.gatewayProxy.gatewaySettings.disableHttpGateway=true"},
						})
						assertCustomResourceManifest(map[string]types.GomegaMatcher{
							getSslGatewayName(defaults.GatewayProxyName): BeEquivalentTo(gwSsl),
						})
					})

					It("disabling http gateway disables corresponding service port", func() {
						prepareMakefile(namespace, helmValues{
							valuesArgs: []string{"gatewayProxies.gatewayProxy.gatewaySettings.disableHttpGateway=true"},
						})

						var gatewayProxyService *v1.Service

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
								Name:       "https",
								Protocol:   "TCP",
								Port:       443,
								TargetPort: intstr.IntOrString{IntVal: 8443},
							},
						}
						gatewayProxyService.Spec.Type = v1.ServiceTypeLoadBalancer

						testManifest.ExpectService(gatewayProxyService)
					})

					It("can disable https gateway", func() {
						prepareMakefile(namespace, helmValues{
							valuesArgs: []string{"gatewayProxies.gatewayProxy.gatewaySettings.disableHttpsGateway=true"},
						})
						assertCustomResourceManifest(map[string]types.GomegaMatcher{
							defaults.GatewayProxyName: BeEquivalentTo(gw),
						})
					})

					It("disabling https gateway disables corresponding service port", func() {
						prepareMakefile(namespace, helmValues{
							valuesArgs: []string{"gatewayProxies.gatewayProxy.gatewaySettings.disableHttpsGateway=true"},
						})

						var gatewayProxyService *v1.Service

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
						}
						gatewayProxyService.Spec.Type = v1.ServiceTypeLoadBalancer

						testManifest.ExpectService(gatewayProxyService)
					})

					It("sets http hybrid gateway", func() {
						gwHybrid := makeUnstructured(`
apiVersion: gateway.solo.io/v1
kind: Gateway
metadata:
  name: gateway-proxy
  namespace: gloo-system
  labels:
    app: gloo
spec:
  bindAddress: "::"
  bindPort: 8080
  hybridGateway:
    matchedGateways:
    - httpGateway:
        virtualServices:
        - name: default
          namespace: gloo-system
      matcher:
        sourcePrefixRanges:
        - addressPrefix: 0.0.0.0
          prefixLen: 1
    - httpGateway:
        virtualServices:
        - name: client-ip-reject
          namespace: gloo-system
      matcher: {}
  httpGateway: {}
  useProxyProto: false
  ssl: false
  proxyNames:
  - gateway-proxy
`)
						prepareMakefileFromValuesFile("values/val_gwp_http_hybrid_gateway.yaml")
						assertCustomResourceManifest(map[string]types.GomegaMatcher{
							"gateway-proxy":     BeEquivalentTo(gwHybrid),
							"gateway-proxy-ssl": BeEquivalentTo(gwSsl),
						})
					})

					It("sets https hybrid gateway", func() {
						gwHybrid := makeUnstructured(`
apiVersion: gateway.solo.io/v1
kind: Gateway
metadata:
  name: gateway-proxy-ssl
  namespace: gloo-system
  labels:
    app: gloo
spec:
  bindAddress: "::"
  bindPort: 8443
  hybridGateway:
    matchedGateways:
    - httpGateway:
        virtualServices:
        - name: default
          namespace: gloo-system
      matcher:
        sourcePrefixRanges:
        - addressPrefix: 0.0.0.0
          prefixLen: 1
        sslConfig:
          secretRef:
            name: gloo-cert
            namespace: gloo-system
    - httpGateway:
        virtualServices:
        - name: client-ip-reject
          namespace: gloo-system
      matcher:
        sslConfig:
          secretRef:
            name: gloo-cert
            namespace: gloo-system
  httpGateway: {}
  useProxyProto: false
  ssl: true
  proxyNames:
  - gateway-proxy
`)
						prepareMakefileFromValuesFile("values/val_gwp_https_hybrid_gateway.yaml")
						assertCustomResourceManifest(map[string]types.GomegaMatcher{
							"gateway-proxy":     BeEquivalentTo(gw),
							"gateway-proxy-ssl": BeEquivalentTo(gwHybrid),
						})
					})

					It("can set accessLoggingService", func() {
						gwAls := makeUnstructured(`
apiVersion: gateway.solo.io/v1
kind: Gateway
metadata:
  name: gateway-proxy
  namespace: gloo-system
  labels:
    app: gloo
spec:
  bindAddress: "::"
  bindPort: 8080
  httpGateway: {}
  options:
    accessLoggingService:
      accessLog:
      - fileSink:
          path: /dev/stdout
          stringFormat: ""
  useProxyProto: false
  ssl: false
  proxyNames:
  - gateway-proxy
`)

						gwAlsSsl := makeUnstructured(`
apiVersion: gateway.solo.io/v1
kind: Gateway
metadata:
  name: gateway-proxy-ssl
  namespace: gloo-system
  labels:
    app: gloo
spec:
  bindAddress: "::"
  bindPort: 8443
  httpGateway: {}
  options:
    accessLoggingService:
      accessLog:
      - fileSink:
          path: /dev/stdout
          stringFormat: ""
  useProxyProto: false
  ssl: true
  proxyNames:
  - gateway-proxy
`)

						prepareMakefileFromValuesFile("values/val_default_gateway_access_logging_service.yaml")
						assertCustomResourceManifest(map[string]types.GomegaMatcher{
							"gateway-proxy":     BeEquivalentTo(gwAls),
							"gateway-proxy-ssl": BeEquivalentTo(gwAlsSsl),
						})
					})

					It("can set tracing provider", func() {
						gwTracing := makeUnstructured(`
apiVersion: gateway.solo.io/v1
kind: Gateway
metadata:
  name: gateway-proxy
  namespace: gloo-system
  labels:
    app: gloo
spec:
  bindAddress: "::"
  bindPort: 8080
  httpGateway:
    options:
      httpConnectionManagerSettings:
        tracing:
          zipkinConfig:
            collector_cluster: zipkin
            collector_endpoint: /api/v2/spans
  useProxyProto: false
  ssl: false
  proxyNames:
  - gateway-proxy
`)

						gwTracingSsl := makeUnstructured(`
apiVersion: gateway.solo.io/v1
kind: Gateway
metadata:
  name: gateway-proxy-ssl
  namespace: gloo-system
  labels:
    app: gloo
spec:
  bindAddress: "::"
  bindPort: 8443
  httpGateway:
    options:
      httpConnectionManagerSettings:
        tracing:
          zipkinConfig:
            collector_cluster: zipkin
            collector_endpoint: /api/v2/spans
  useProxyProto: false
  ssl: true
  proxyNames:
  - gateway-proxy
`)
						prepareMakefileFromValuesFile("values/val_tracing_provider_cluster.yaml")
						assertCustomResourceManifest(map[string]types.GomegaMatcher{
							"gateway-proxy":     BeEquivalentTo(gwTracing),
							"gateway-proxy-ssl": BeEquivalentTo(gwTracingSsl),
						})
					})

					It("gwp hpa disabled by default", func() {
						prepareMakefile(namespace, helmValues{})

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

					It("can create gwp autoscaling/v2 hpa", func() {

						prepareMakefileFromValuesFile("values/val_gwp_hpa_v2.yaml")

						hpa := makeUnstructured(`
apiVersion: autoscaling/v2
kind: HorizontalPodAutoscaler
metadata:
  labels:
    app: gloo
    gateway-proxy-id: gateway-proxy
    gloo: gateway-proxy
  name: gateway-proxy-hpa
  namespace: gloo-system
spec:
  maxReplicas: 2
  metrics:
    - type: Resource
      resource:
        name: cpu
        target:
          type: Utilization
          averageUtilization: 75
  minReplicas: 1
  scaleTargetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: gateway-proxy
  behavior:
    scaleUp:
      stabilizationWindowSeconds: 0
      policies:
      - type: Percent
        value: 100
        periodSeconds: 15
      - type: Pods
        value: 4
        periodSeconds: 15
      selectPolicy: Max
    scaleDown:
      policies:
      - type: Percent
        value: 100
        periodSeconds: 15
      stabilizationWindowSeconds: 300
`)

						testManifest.ExpectUnstructured("HorizontalPodAutoscaler", namespace, defaults.GatewayProxyName+"-hpa").To(BeEquivalentTo(hpa))

					})

					It("gwp pdb disabled by default", func() {
						prepareMakefile(namespace, helmValues{})

						testManifest.ExpectUnstructured("PodDisruptionBudget", namespace, defaults.GatewayProxyName+"-pdb").To(BeNil())
					})

					It("can create gwp pdb with minAvailable", func() {

						prepareMakefile(namespace, helmValues{
							valuesArgs: []string{
								"gatewayProxies.gatewayProxy.podDisruptionBudget.minAvailable='2'",
							},
						})

						pdb := makeUnstructured(`
apiVersion: policy/v1
kind: PodDisruptionBudget
metadata:
  name: gateway-proxy-pdb
  namespace: gloo-system
spec:
  minAvailable: '2'
  selector:
    matchLabels:
      gateway-proxy-id: gateway-proxy
`)

						testManifest.ExpectUnstructured("PodDisruptionBudget", namespace, defaults.GatewayProxyName+"-pdb").To(BeEquivalentTo(pdb))
					})

					It("can create gwp pdb with maxUnavailable", func() {

						prepareMakefile(namespace, helmValues{
							valuesArgs: []string{
								"gatewayProxies.gatewayProxy.podDisruptionBudget.maxUnavailable='2'",
							},
						})

						pdb := makeUnstructured(`
apiVersion: policy/v1
kind: PodDisruptionBudget
metadata:
  name: gateway-proxy-pdb
  namespace: gloo-system
spec:
  maxUnavailable: '2'
  selector:
    matchLabels:
      gateway-proxy-id: gateway-proxy
`)

						testManifest.ExpectUnstructured("PodDisruptionBudget", namespace, defaults.GatewayProxyName+"-pdb").To(BeEquivalentTo(pdb))
					})

					It("can create gwp pdb for multiple gateways with unique selectors", func() {

						prepareMakefile(namespace, helmValues{
							valuesArgs: []string{
								"gatewayProxies.gatewayProxy.podDisruptionBudget.maxUnavailable='2'",
								"gatewayProxies.gatewayProxyTwo.podDisruptionBudget.maxUnavailable='2'",
							},
						})

						pdbFormat := `
apiVersion: policy/v1
kind: PodDisruptionBudget
metadata:
  name: %s-pdb
  namespace: gloo-system
spec:
  maxUnavailable: '2'
  selector:
    matchLabels:
      gateway-proxy-id: %s
`

						testManifest.ExpectUnstructured("PodDisruptionBudget", namespace, defaults.GatewayProxyName+"-pdb").To(BeEquivalentTo(makeUnstructured(fmt.Sprintf(pdbFormat, defaults.GatewayProxyName, defaults.GatewayProxyName))))
						testManifest.ExpectUnstructured("PodDisruptionBudget", namespace, defaults.GatewayProxyName+"-two-pdb").To(BeEquivalentTo(makeUnstructured(fmt.Sprintf(pdbFormat, defaults.GatewayProxyName+"-two", defaults.GatewayProxyName+"-two"))))
					})

					It("can render with custom listener yaml", func() {
						prepareMakefileFromValuesFile("values/val_custom_gateways.yaml")
						gws := []*unstructured.Unstructured{
							makeUnstructured(`
apiVersion: gateway.solo.io/v1
kind: Gateway
metadata:
  name: gateway-proxy
  namespace: gloo-system
  labels:
    app: gloo
spec:
  bindAddress: "::"
  bindPort: 8080
  httpGateway:
    virtualServices:
    - name: one
      namespace: one
  useProxyProto: true
  ssl: false
  proxyNames:
  - gateway-proxy
`),
							makeUnstructured(`
apiVersion: gateway.solo.io/v1
kind: Gateway
metadata:
  name: gateway-proxy-ssl
  namespace: gloo-system
  labels:
    app: gloo
spec:
  bindAddress: "::"
  bindPort: 8443
  httpGateway:
    virtualServices:
    - name: one
      namespace: one
  useProxyProto: true
  ssl: true
  proxyNames:
  - gateway-proxy
`),
							makeUnstructured(`
apiVersion: gateway.solo.io/v1
kind: Gateway
metadata:
  name: test-name
  namespace: gloo-system
  labels:
    app: gloo
spec:
  bindAddress: "::"
  bindPort: 8081
  httpGateway:
    virtualServices:
    - name: one
      namespace: one
  useProxyProto: true
  ssl: false
  proxyNames:
  - test-name
`),
							makeUnstructured(`
apiVersion: gateway.solo.io/v1
kind: Gateway
metadata:
  name: test-name-ssl
  namespace: gloo-system
  labels:
    app: gloo
spec:
  bindAddress: "::"
  bindPort: 8444
  httpGateway:
    virtualServices:
    - name: one
      namespace: one
  useProxyProto: true
  ssl: true
  proxyNames:
  - test-name
`),
						}
						assertCustomResourceManifest(map[string]types.GomegaMatcher{
							"gateway-proxy":     BeEquivalentTo(gws[0]),
							"gateway-proxy-ssl": BeEquivalentTo(gws[1]),
							"test-name":         BeEquivalentTo(gws[2]),
							"test-name-ssl":     BeEquivalentTo(gws[3]),
						})
					})
				})

				Context("Failover Gateway", func() {

					gwFailover := makeUnstructured(`
apiVersion: gateway.solo.io/v1
kind: Gateway
metadata:
  name: ` + getFailoverGatewayName(defaults.GatewayProxyName) + `
  namespace: ` + namespace + `
  labels:
    app: gloo
spec:
  bindAddress: "` + defaults.GatewayBindAddress + `"
  bindPort: 15444
  tcpGateway:
    tcpHosts:
    - name: failover
      sslConfig:
        secretRef:
          name: failover-downstream
          namespace: gloo-system
      destination:
        forwardSniClusterName: {}
  proxyNames:
  - ` + defaults.GatewayProxyName)

					It("renders with http/https gateways by default", func() {
						prepareMakefile(namespace, helmValues{
							valuesArgs: []string{
								"gatewayProxies.gatewayProxy.failover.enabled=true",
								"gatewayProxies.gatewayProxy.failover.port=15444",
							},
						})

						// expect the 2 default gateways + failover gateway
						assertCustomResourceManifest(map[string]types.GomegaMatcher{
							defaults.GatewayProxyName:                         Not(BeNil()),
							getSslGatewayName(defaults.GatewayProxyName):      Not(BeNil()),
							getFailoverGatewayName(defaults.GatewayProxyName): BeEquivalentTo(gwFailover),
						})
					})

					It("by default will not render failover gateway", func() {
						prepareMakefile(namespace, helmValues{})
						// expect only the 2 default gateways
						assertCustomResourceManifest(map[string]types.GomegaMatcher{
							defaults.GatewayProxyName:                    Not(BeNil()),
							getSslGatewayName(defaults.GatewayProxyName): Not(BeNil()),
						})
					})

				})

				Context("custom gateway", func() {

					Context("when the default values weren't overridden", func() {
						BeforeEach(func() {
							prepareMakefile(namespace, helmValues{
								valuesArgs: []string{
									"gatewayProxies.anotherGatewayProxy.gatewaySettings.options.socketOptions[0].description=enable keep-alive",
								},
							})
						})
						It("uses default values for the gateway", func() {
							anotherGw := makeUnstructured(`
apiVersion: gateway.solo.io/v1
kind: Gateway
metadata:
  labels:
    app: gloo
  name: another-gateway-proxy
  namespace: ` + namespace + `
spec:
  bindAddress: '::'
  bindPort: 8080
  httpGateway: {}
  options:
    socketOptions:
    - description: "enable keep-alive"
  proxyNames:
  - another-gateway-proxy
  ssl: false
  useProxyProto: false
`)
							// expect 2 default gateways + another-gateway-proxy + another-gateway-proxy-ssl
							assertCustomResourceManifest(map[string]types.GomegaMatcher{
								defaults.GatewayProxyName:                    Not(BeNil()),
								getSslGatewayName(defaults.GatewayProxyName): Not(BeNil()),
								"another-gateway-proxy":                      BeEquivalentTo(anotherGw),
								getSslGatewayName("another-gateway-proxy"):   Not(BeNil()),
							})
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
							configMap := getConfigMap(testManifest, namespace, "another-gateway-proxy-envoy-config")
							Expect(configMap.Data).ToNot(BeNil()) // Uses the default config data
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
							anotherGw := makeUnstructured(`
apiVersion: gateway.solo.io/v1
kind: Gateway
metadata:
  labels:
    app: gloo
  name: another-gateway-proxy
  namespace: ` + namespace + `
spec:
  bindAddress: '::'
  bindPort: 9999
  httpGateway: {}
  proxyNames:
  - another-gateway-proxy
  ssl: false
  useProxyProto: false
`)
							// expect 2 default gateways + another-gateway-proxy + another-gateway-proxy-ssl
							assertCustomResourceManifest(map[string]types.GomegaMatcher{
								defaults.GatewayProxyName:                    Not(BeNil()),
								getSslGatewayName(defaults.GatewayProxyName): Not(BeNil()),
								"another-gateway-proxy":                      BeEquivalentTo(anotherGw),
								getSslGatewayName("another-gateway-proxy"):   Not(BeNil()),
							})
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
							configMap := getConfigMap(testManifest, namespace, "another-gateway-proxy-envoy-config")
							Expect(configMap.Data).To(Equal(map[string]string{"customData": "someData"}))
						})
					})

					Context("when non-default values are overridden by custom gatewayproxy", func() {
						BeforeEach(func() {
							prepareMakefile(namespace, helmValues{
								valuesArgs: []string{
									"gatewayProxies.gatewayProxy.service.extraAnnotations.original=original",
									"gatewayProxies.anotherGatewayProxy.service.extraAnnotations.override=override",
								},
							})
						})

						It("does not merge extraAnnotations for service", func() {
							serviceUns := testManifest.ExpectCustomResource("Service", namespace, "another-gateway-proxy")
							service, err := kuberesource.ConvertUnstructured(serviceUns)
							Expect(err).NotTo(HaveOccurred())
							Expect(service).To(BeAssignableToTypeOf(&v1.Service{}))
							serviceStr := *service.(*v1.Service)
							Expect(serviceStr.ObjectMeta.Annotations).To(Equal(map[string]string{"override": "override"}))
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
						// only the first and second ssl gateways should be enabled
						firstGwSsl := makeUnstructured(`
apiVersion: gateway.solo.io/v1
kind: Gateway
metadata:
  name: first-gateway-proxy-ssl
  namespace: gloo-system
  labels:
    app: gloo
spec:
  bindAddress: "::"
  bindPort: 8443
  httpGateway:
    virtualServiceSelector:
      gateway: first
  useProxyProto: false
  ssl: true
  proxyNames:
  - first-gateway-proxy
`)
						secondGwSsl := makeUnstructured(`
apiVersion: gateway.solo.io/v1
kind: Gateway
metadata:
  name: second-gateway-proxy-ssl
  namespace: gloo-system
  labels:
    app: gloo
spec:
  bindAddress: "::"
  bindPort: 8443
  httpGateway:
    virtualServiceSelector:
      gateway: second
  useProxyProto: false
  ssl: true
  proxyNames:
  - second-gateway-proxy
`)
						assertCustomResourceManifest(map[string]types.GomegaMatcher{
							"first-gateway-proxy-ssl":  BeEquivalentTo(firstGwSsl),
							"second-gateway-proxy-ssl": BeEquivalentTo(secondGwSsl),
						})
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

					checkDiscoveryAddressEqual := func(expected string) {
						testManifest.SelectResources(func(resource *unstructured.Unstructured) bool {
							return resource.GetKind() == "Deployment" && resource.GetName() == "gateway-proxy"
						}).ExpectAll(func(deployment *unstructured.Unstructured) {
							deploymentObject, err := kuberesource.ConvertUnstructured(deployment)
							Expect(err).NotTo(HaveOccurred(), "Deployment should be able to convert from unstructured")
							structuredDeployment, ok := deploymentObject.(*appsv1.Deployment)
							Expect(ok).To(BeTrue(), "Deployment should be able to cast to a structured deployment")
							isProxyConfigSet := false

							var discoveryAddress interface{}
							for _, container := range structuredDeployment.Spec.Template.Spec.Containers {
								for _, env := range container.Env {
									if env.Name == "PROXY_CONFIG" {
										isProxyConfigSet = true
										var proxyConfigMap map[string]interface{}
										err := yaml.Unmarshal([]byte(env.Value), &proxyConfigMap)
										Expect(err).ToNot(HaveOccurred())
										discoveryAddress, ok = proxyConfigMap["discoveryAddress"]
										Expect(ok).To(BeTrue(), "discoveryAddress should be set in PROXY_CONFIG")
										Expect(discoveryAddress).To(Equal(expected), fmt.Sprintf("discovery address should be value: %v", expected))
										break
									}
								}
							}

							Expect(isProxyConfigSet).To(BeTrue(), "Istio's PROXY_CONFIG and discoveryAddress were not set")
						})
					}

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
						deploy.Spec.Template.Spec.Containers[0].Env = append(
							deploy.Spec.Template.Spec.Containers[0].Env,
							v1.EnvVar{
								Name:  "DISABLE_CORE_DUMPS",
								Value: "false",
							})
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

						It("can set the DISABLE_CORE_DUMPS environment variable", func() {
							prepareMakefile(namespace, helmValues{
								valuesArgs: []string{"gatewayProxies.gatewayProxy.disableCoreDumps=true"},
							})
							testManifest.SelectResources(func(resource *unstructured.Unstructured) bool {
								return resource.GetKind() == "Deployment" && resource.GetName() == "gateway-proxy"
							}).ExpectAll(func(deployment *unstructured.Unstructured) {
								deploymentObject, err := kuberesource.ConvertUnstructured(deployment)
								Expect(err).NotTo(HaveOccurred(), fmt.Sprintf("Deployment %+v should be able to convert from unstructured", deployment))
								structuredDeployment, ok := deploymentObject.(*appsv1.Deployment)
								Expect(ok).To(BeTrue(), fmt.Sprintf("Deployment %+v should be able to cast to a structured deployment", deployment))

								Expect(structuredDeployment.Spec.Template.Spec.Containers).To(HaveLen(1))
								for _, envvar := range structuredDeployment.Spec.Template.Spec.Containers[0].Env {
									if envvar.Name != "DISABLE_CORE_DUMPS" {
										continue
									}
									Expect(envvar.Value).To(Equal("true"))
								}
							})
						})

						It("can explicitly disable hostNetwork", func() {
							daemonSet.Spec.Template.Spec.HostNetwork = false
							prepareMakefile(namespace, helmValues{
								valuesArgs: []string{
									"gatewayProxies.gatewayProxy.kind.deployment=null",
									"gatewayProxies.gatewayProxy.kind.daemonSet.hostPort=true",
									"gatewayProxies.gatewayProxy.kind.daemonSet.hostNetwork=false",
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

					It("supports deploying the fips envoy image", func() {
						prepareMakefile(namespace, helmValues{
							valuesArgs: []string{
								"global.image.fips=true",
								"gatewayProxies.gatewayProxy.podTemplate.image.repository=gloo-ee-envoy-wrapper",
							},
						})
						gatewayProxyDeployment.Spec.Template.Spec.Containers[0].Image = "quay.io/solo-io/gloo-ee-envoy-wrapper-fips:" + version
						testManifest.ExpectDeploymentAppsV1(gatewayProxyDeployment)
					})

					It("doesn't break containers when enabling multiple containers", func() {
						prepareMakefile(namespace, helmValues{
							valuesArgs: []string{
								"global.glooMtls.enabled=true",
								"global.istioSDS.enabled=true",
							},
						})

						// Containers we expect to have
						expectedContainers := map[string]struct{}{
							"gateway-proxy": {},
							"istio-proxy":   {},
							"sds":           {},
						}

						testManifest.SelectResources(func(resource *unstructured.Unstructured) bool {
							return resource.GetKind() == "Deployment" && resource.GetName() == "gateway-proxy"
						}).ExpectAll(func(deployment *unstructured.Unstructured) {
							deploymentObject, err := kuberesource.ConvertUnstructured(deployment)
							Expect(err).NotTo(HaveOccurred(), fmt.Sprintf("Deployment %+v should be able to convert from unstructured", deployment))
							structuredDeployment, ok := deploymentObject.(*appsv1.Deployment)
							Expect(ok).To(BeTrue(), fmt.Sprintf("Deployment %+v should be able to cast to a structured deployment", deployment))

							for _, container := range structuredDeployment.Spec.Template.Spec.Containers {
								if _, ok := expectedContainers[container.Name]; ok {
									// delete found containers from our expectedContainers list
									delete(expectedContainers, container.Name)
								} else {
									Fail(fmt.Sprintf("Unexpected container found: %+v", container.Name))
								}
							}
						})

						// An expected container was not correctly set
						Expect(len(expectedContainers)).To(BeZero(), "all enabled containers must have been found")
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
								"gatewayProxies.gatewayProxy.kind.deployment.replicas=0",
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

					It("allows removing pod security context", func() {
						prepareMakefile(namespace, helmValues{
							valuesArgs: []string{"gatewayProxies.gatewayProxy.podTemplate.enablePodSecurityContext=false"},
						})
						gatewayProxyDeployment.Spec.Template.Spec.SecurityContext = nil
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
										{
											MatchExpressions: []v1.NodeSelectorRequirement{
												{
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

					It("sets topologySpreadConstraints", func() {

						gatewayProxyDeployment.Spec.Template.Spec.TopologySpreadConstraints = append(
							gatewayProxyDeployment.Spec.Template.Spec.TopologySpreadConstraints,
							v1.TopologySpreadConstraint{
								MaxSkew:           1,
								TopologyKey:       "zone",
								WhenUnsatisfiable: "ScheduleAnyway",
								LabelSelector: &metav1.LabelSelector{
									MatchLabels: map[string]string{"gloo": "gateway-proxy"},
								},
							})

						prepareMakefileFromValuesFile("values/val_gwp_topologyspreadconstraints.yaml")

						testManifest.ExpectDeploymentAppsV1(gatewayProxyDeployment)
					})

					It("enables probes", func() {
						prepareMakefile(namespace, helmValues{
							valuesArgs: []string{
								"gatewayProxies.gatewayProxy.podTemplate.probes=true",
								"gatewayProxies.gatewayProxy.podTemplate.livenessProbeEnabled=true",
							},
						})
						gatewayProxyDeployment.Spec.Template.Spec.Containers[0].ReadinessProbe = &v1.Probe{
							ProbeHandler: v1.ProbeHandler{
								Exec: &v1.ExecAction{
									Command: []string{
										"wget", "-O", "/dev/null", "127.0.0.1:19000/ready",
									},
								},
							},
							InitialDelaySeconds: 3,
							PeriodSeconds:       10,
							FailureThreshold:    3,
						}
						gatewayProxyDeployment.Spec.Template.Spec.Containers[0].LivenessProbe = &v1.Probe{
							ProbeHandler: v1.ProbeHandler{
								Exec: &v1.ExecAction{
									Command: []string{
										"wget", "-O", "/dev/null", "127.0.0.1:19000/server_info",
									},
								},
							},
							InitialDelaySeconds: 3,
							PeriodSeconds:       10,
							FailureThreshold:    3,
						}
						testManifest.ExpectDeploymentAppsV1(gatewayProxyDeployment)
					})

					It("supports custom readiness and liveness probe", func() {
						prepareMakefile(namespace, helmValues{
							valuesArgs: []string{
								"gatewayProxies.gatewayProxy.podTemplate.probes=true",
								"gatewayProxies.gatewayProxy.podTemplate.livenessProbeEnabled=true",
								"gatewayProxies.gatewayProxy.podTemplate.customReadinessProbe.initialDelaySeconds=3",
								"gatewayProxies.gatewayProxy.podTemplate.customReadinessProbe.failureThreshold=3",
								"gatewayProxies.gatewayProxy.podTemplate.customReadinessProbe.periodSeconds=10",
								"gatewayProxies.gatewayProxy.podTemplate.customReadinessProbe.httpGet.path=/ready",
								"gatewayProxies.gatewayProxy.podTemplate.customReadinessProbe.httpGet.port=19000",
								"gatewayProxies.gatewayProxy.podTemplate.customReadinessProbe.httpGet.scheme=HTTP",
								"gatewayProxies.gatewayProxy.podTemplate.customLivenessProbe.initialDelaySeconds=3",
								"gatewayProxies.gatewayProxy.podTemplate.customLivenessProbe.failureThreshold=3",
								"gatewayProxies.gatewayProxy.podTemplate.customLivenessProbe.periodSeconds=10",
								"gatewayProxies.gatewayProxy.podTemplate.customLivenessProbe.httpGet.path=/server_info",
								"gatewayProxies.gatewayProxy.podTemplate.customLivenessProbe.httpGet.port=19000",
								"gatewayProxies.gatewayProxy.podTemplate.customLivenessProbe.httpGet.scheme=HTTP",
							},
						})
						gatewayProxyDeployment.Spec.Template.Spec.Containers[0].ReadinessProbe = &v1.Probe{
							ProbeHandler: v1.ProbeHandler{
								HTTPGet: &v1.HTTPGetAction{
									Path:   "/ready",
									Port:   intstr.FromInt(19000),
									Scheme: "HTTP",
								},
							},
							InitialDelaySeconds: 3,
							PeriodSeconds:       10,
							FailureThreshold:    3,
						}
						gatewayProxyDeployment.Spec.Template.Spec.Containers[0].LivenessProbe = &v1.Probe{
							ProbeHandler: v1.ProbeHandler{
								HTTPGet: &v1.HTTPGetAction{
									Path:   "/server_info",
									Port:   intstr.FromInt(19000),
									Scheme: "HTTP",
								},
							},
							InitialDelaySeconds: 3,
							PeriodSeconds:       10,
							FailureThreshold:    3,
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
							PreStop: &v1.LifecycleHandler{
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
							PreStop: &v1.LifecycleHandler{
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
								"gatewayProxies.gatewayProxy.podTemplate.resources.limits.memory=2Mi",
								"gatewayProxies.gatewayProxy.podTemplate.resources.limits.cpu=3m",
								"gatewayProxies.gatewayProxy.podTemplate.resources.requests.memory=4Mi",
								"gatewayProxies.gatewayProxy.podTemplate.resources.requests.cpu=5m",
							},
						})

						// Add the limits we are testing:
						gatewayProxyDeployment.Spec.Template.Spec.Containers[0].Resources = v1.ResourceRequirements{
							Limits: v1.ResourceList{
								v1.ResourceMemory: resource.MustParse("2Mi"),
								v1.ResourceCPU:    resource.MustParse("3m"),
							},
							Requests: v1.ResourceList{
								v1.ResourceMemory: resource.MustParse("4Mi"),
								v1.ResourceCPU:    resource.MustParse("5m"),
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

					It("can render Extra Containers", func() {
						gatewayProxyDeployment.Spec.Template.Spec.Containers = append(gatewayProxyDeployment.Spec.Template.Spec.Containers, gatewayProxyDeployment.Spec.Template.Spec.Containers[0])
						gatewayProxyDeployment.Spec.Template.Spec.Containers[0] = v1.Container{
							Image: "gcr.io/solo-public",
							Name:  "podName",
							Command: []string{
								"sh",
								"-c",
							},
						}
						prepareMakefile(namespace, helmValues{
							valuesArgs: []string{
								"gatewayProxies.gatewayProxy.podTemplate.extraContainers[0].image=gcr.io/solo-public",
								"gatewayProxies.gatewayProxy.podTemplate.extraContainers[0].name=podName",
								"gatewayProxies.gatewayProxy.podTemplate.extraContainers[0].command[0]=sh",
								"gatewayProxies.gatewayProxy.podTemplate.extraContainers[0].command[1]=-c",
							},
						})
						testManifest.ExpectDeploymentAppsV1(gatewayProxyDeployment)
					})

					It("can render Extra Init Containers", func() {
						gatewayProxyDeployment.Spec.Template.Spec.InitContainers = append(gatewayProxyDeployment.Spec.Template.Spec.InitContainers, v1.Container{
							Image: "gcr.io/solo-public",
							Name:  "podName",
							Command: []string{
								"sh",
								"-c",
							},
						})
						prepareMakefile(namespace, helmValues{
							valuesArgs: []string{
								"gatewayProxies.gatewayProxy.podTemplate.extraInitContainers[0].image=gcr.io/solo-public",
								"gatewayProxies.gatewayProxy.podTemplate.extraInitContainers[0].name=podName",
								"gatewayProxies.gatewayProxy.podTemplate.extraInitContainers[0].command[0]=sh",
								"gatewayProxies.gatewayProxy.podTemplate.extraInitContainers[0].command[1]=-c",
							},
						})
						testManifest.ExpectDeploymentAppsV1(gatewayProxyDeployment)
					})

					It("can overwrite sds and istioProxy images", func() {
						prepareMakefile(namespace, helmValues{
							valuesArgs: []string{
								"global.glooMtls.enabled=true",
								"global.istioSDS.enabled=true",
								"global.glooMtls.sds.image.tag=my-sds-tag",
								"global.glooMtls.sds.image.repository=my-sds-repo",
								"global.glooMtls.sds.image.registry=my-sds-reg",
								"global.glooMtls.istioProxy.image.tag=my-istio-tag",
								"global.glooMtls.istioProxy.image.repository=my-istio-repo",
								"global.glooMtls.istioProxy.image.registry=my-istio-reg",
								"global.glooMtls.istioProxy.image.pullPolicy=Always",
							},
						})

						gwpUns := testManifest.ExpectCustomResource("Deployment", namespace, "gateway-proxy")
						gwpObj, err := kuberesource.ConvertUnstructured(gwpUns)
						Expect(err).NotTo(HaveOccurred())
						Expect(gwpObj).To(BeAssignableToTypeOf(&appsv1.Deployment{}))
						gwpDepl := *gwpObj.(*appsv1.Deployment)
						Expect(gwpDepl.Spec.Template.Spec.Containers).To(HaveLen(3))

						sdsContainer := gwpDepl.Spec.Template.Spec.Containers[1]
						Expect(sdsContainer.Name).To(Equal("sds"))
						Expect(sdsContainer.Image).To(Equal("my-sds-reg/my-sds-repo:my-sds-tag"))
						Expect(sdsContainer.ImagePullPolicy).To(Equal(v1.PullIfNotPresent))

						istioProxyContainer := gwpDepl.Spec.Template.Spec.Containers[2]
						Expect(istioProxyContainer.Name).To(Equal("istio-proxy"))
						Expect(istioProxyContainer.Image).To(Equal("my-istio-reg/my-istio-repo:my-istio-tag"))
						Expect(istioProxyContainer.ImagePullPolicy).To(Equal(v1.PullAlways))
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

							// get ISTIO_META_MESH_ID env var value
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

							// get ISTIO_META_MESH_ID env var value
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

							// get ISTIO_META_CLUSTER_ID env var value
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

							// get ISTIO_META_CLUSTER_ID env var value
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

					It("can set discoveryAddress value in PROXY_CONFIG env var", func() {
						val := "istiod-1-8-6.istio-system.svc:15012"

						prepareMakefile(namespace, helmValues{
							valuesArgs: []string{
								"global.glooMtls.enabled=true",
								"global.istioSDS.enabled=true",
								"gatewayProxies.gatewayProxy.istioDiscoveryAddress=" + val,
							},
						})

						checkDiscoveryAddressEqual(val)
					})

					It("istio's discoveryAddress default value set", func() {
						def := "istiod.istio-system.svc:15012"

						prepareMakefile(namespace, helmValues{
							valuesArgs: []string{
								"global.glooMtls.enabled=true",
								"global.istioSDS.enabled=true",
							},
						})

						checkDiscoveryAddressEqual(def)
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

					It("can set envoy log level", func() {
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
						glooService := makeUnstructured(`
---
# Source: gloo/templates/2-gloo-service.yaml
apiVersion: v1
kind: Service
metadata:
  labels:
    app: gloo
    gloo: gloo
  name: gloo
  namespace: ` + namespace + `
spec:
  ports:
  - name: grpc-xds
    port: 9977
    protocol: TCP
  - name: rest-xds
    port: 9976
    protocol: TCP
  - name: grpc-validation
    port: 9988
    protocol: TCP
  - name: grpc-proxydebug
    port: 9966
    protocol: TCP
  - name: wasm-cache
    port: 9979
    protocol: TCP
  - name: https
    port: 443
    protocol: TCP
    # this should map to projects/gateway/pkg/defaults.ValidationWebhookBindPort
    targetPort: 8443
  selector:
    gloo: gloo
`)

						prepareMakefile(namespace, helmValues{})
						testManifest.ExpectUnstructured(glooService.GetKind(), glooService.GetNamespace(), glooService.GetName()).To(BeEquivalentTo(glooService))

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

					It("correctly sets the gateway validation fields in the settings", func() {
						settings := makeUnstructureFromTemplateFile("fixtures/settings/gateway_validation.yaml", namespace)

						prepareMakefile(namespace, helmValues{
							valuesArgs: []string{
								"gateway.validation.disableTransformationValidation=true",
								"gateway.validation.warnRouteShortCircuiting=true",
							},
						})
						testManifest.ExpectUnstructured(settings.GetKind(), settings.GetNamespace(), settings.GetName()).To(BeEquivalentTo(settings))
					})
					It("correctly sets the gateway circuitBreakers fields in the settings", func() {
						settings := makeUnstructureFromTemplateFile("fixtures/settings/gateway_circuit_breakers.yaml", namespace)

						prepareMakefile(namespace, helmValues{
							valuesArgs: []string{
								"settings.circuitBreakers.maxConnections=1024",
								"settings.circuitBreakers.maxPendingRequests=1024",
								"settings.circuitBreakers.maxRequests=1024",
								"settings.circuitBreakers.maxRetries=3",
							},
						})
						testManifest.ExpectUnstructured(settings.GetKind(), settings.GetNamespace(), settings.GetName()).To(BeEquivalentTo(settings))
					})
					It("always enables persisting proxy specs when not in gateway mode", func() {
						settings := makeUnstructureFromTemplateFile("fixtures/settings/disabled_gateway.yaml", namespace)
						prepareMakefile(namespace, helmValues{
							valuesArgs: []string{
								"gateway.enabled=false",
							}})
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
					It("correctly allows setting compressedProxySpec field in the settings when validation disabled", func() {
						settings := makeUnstructureFromTemplateFile("fixtures/settings/compressed_proxy_spec.yaml", namespace)

						prepareMakefile(namespace, helmValues{
							valuesArgs: []string{
								"gateway.validation.enabled=false",
								"gateway.compressedProxySpec=true",
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
					It("correctly allows setting ratelimitServer field.", func() {
						settings := makeUnstructureFromTemplateFile("fixtures/settings/ratelimit_server.yaml", namespace)

						prepareMakefile(namespace, helmValues{
							valuesArgs: []string{
								"settings.ratelimitServer.ratelimitServerRef.name=ratelimit",
								"settings.ratelimitServer.ratelimitServerRef.namespace=ratelimitns",
								"settings.ratelimitServer.denyOnFail=true",
								"settings.ratelimitServer.rateLimitBeforeAuth=true",
								"settings.ratelimitServer.enableXRatelimitHeaders=true",
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

					It("correctly sets the `regexMaxProgramSize` field to the default of 1024 in the settings", func() {
						settings := makeUnstructureFromTemplateFile("fixtures/settings/set_regex_max_program_size_default.yaml", namespace)
						prepareMakefile(namespace, helmValues{
							valuesArgs: []string{},
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
					It("correctly sets the `gloo.enableRestEds` when glooMtls is enabled", func() {
						settings := makeUnstructureFromTemplateFile("fixtures/settings/enable_rest_eds_and_gloo_mtls.yaml", namespace)
						prepareMakefile(namespace, helmValues{
							valuesArgs: []string{
								"global.glooMtls.enabled=true",
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
					It("sets secretOptions in settings", func() {
						settings := makeUnstructureFromTemplateFile("fixtures/settings/set_secretSettings_in_settings.yaml", namespace)
						prepareMakefile(namespace, helmValues{
							valuesArgs: []string{
								"settings.secretOptions.sources[0].vault.address=http://vault-internal.vault:8200",
								"settings.secretOptions.sources[0].vault.aws.iamServerIdHeader=vault.gloo.example.com",
								"settings.secretOptions.sources[0].vault.aws.mountPath=aws",
								"settings.secretOptions.sources[0].vault.aws.region=us-east-1",
								"settings.secretOptions.sources[0].vault.pathPrefix=dev",
							},
						})
						testManifest.ExpectUnstructured(settings.GetKind(), settings.GetNamespace(), settings.GetName()).To(BeEquivalentTo(settings))
					})

					It("can enable isolateVirtualHostsBySslConfig", func() {
						settings := makeUnstructureFromTemplateFile("fixtures/settings/isolate_virtual_hosts_by_ssl_config.yaml", namespace)

						prepareMakefile(namespace, helmValues{
							valuesArgs: []string{
								"gateway.isolateVirtualHostsBySslConfig=true",
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
    regexMaxProgramSize: 1024
    xdsBindAddr: "0.0.0.0:9977"
    restXdsBindAddr: "0.0.0.0:9976"
    proxyDebugBindAddr: "0.0.0.0:9966"
    enableRestEds: false
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
    enableGatewayController: true
    isolateVirtualHostsBySslConfig: false
    validation:
      proxyValidationServerAddr: gloo:9988
      alwaysAccept: true
      allowWarnings: true
      serverEnabled: true
      disableTransformationValidation: false
      warnRouteShortCircuiting: false
      validationServerGrpcMaxSizeBytes: 104857600
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
									// Two deployments employ proxy containers requiring the envoySidecar resources config:
									// - gloo (whose sidecar container is named: "envoy-sidecar")
									// - gateway-proxy (named: "istio-proxy")
									if container.Name == "envoy-sidecar" || container.Name == "istio-proxy" {
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
								"settings.aws.propagateOriginalRouting=true",
							},
						})
						testManifest.ExpectUnstructured(settings.GetKind(), settings.GetNamespace(), settings.GetName()).To(BeEquivalentTo(settings))
					})

					It("adds the validation port and mounts the certgen secret to the gloo deployment", func() {
						glooDeployment := makeUnstructured(`
# Source: gloo/templates/1-gloo-deployment.yaml

spec:
  replicas: 1
  selector:
    matchLabels:
      gloo: gloo
  template:
    metadata:
      labels:
        gloo: gloo
      annotations:
        prometheus.io/path: /metrics
        prometheus.io/port: "9091"
        prometheus.io/scrape: "true"
        gloo.solo.io/oss-image-tag: "` + version + `"
    spec:
      serviceAccountName: gloo
      volumes:
      - name: labels-volume
        downwardAPI:
          items:
            - path: "labels"
              fieldRef:
                fieldPath: metadata.labels
      - name: validation-certs
        secret:
          secretName: gateway-validation-certs
          defaultMode: 420
      containers:
      - image: quay.io/solo-io/gloo:` + version + `
        imagePullPolicy: IfNotPresent
        name: gloo
        resources:
          requests:
            cpu: 500m
            memory: 256Mi
        securityContext:
          readOnlyRootFilesystem: true
          allowPrivilegeEscalation: false
          runAsNonRoot: true
          runAsUser: 10101
          capabilities:
            drop:
            - ALL
        ports:
        - containerPort: 9977
          name: grpc-xds
          protocol: TCP
        - containerPort: 9976
          protocol: TCP
          name: rest-xds
        - containerPort: 9988
          name: grpc-validation
          protocol: TCP
        - containerPort: 9966
          name: grpc-proxydebug
          protocol: TCP
        - containerPort: 9979
          name: wasm-cache
          protocol: TCP
        volumeMounts:
        - mountPath: /etc/gateway/validation-certs
          name: validation-certs
        - name: labels-volume
          mountPath: /etc/gloo
          readOnly: true
        env:
          - name: POD_NAMESPACE
            valueFrom:
              fieldRef:
                fieldPath: metadata.namespace
          - name: START_STATS_SERVER
            value: "true"
          - name: VALIDATION_MUST_START
            value: "true"
        readinessProbe:
          tcpSocket:
            port: 9977
          initialDelaySeconds: 3
          periodSeconds: 10
          failureThreshold: 3
apiVersion: apps/v1
kind: Deployment
metadata:
  labels:
    app: gloo
    gloo: gloo
  name: gloo
  namespace: ` + namespace)

						prepareMakefile(namespace, helmValues{
							valuesArgs: []string{
								"gateway.validation.livenessProbeEnabled=true",
							},
						})
						testManifest.ExpectUnstructured(glooDeployment.GetKind(), glooDeployment.GetNamespace(), glooDeployment.GetName()).To(BeEquivalentTo(glooDeployment))
					})

					Context("custom resource lifecycle", func() {
						It("creates migration, rollout, and cleanup jobs", func() {
							prepareMakefile(namespace, helmValues{valuesArgs: []string{}})

							// getJob will fail if the job doesn't exist
							_ = getJob(testManifest, namespace, "gloo-resource-migration")
							_ = getJob(testManifest, namespace, "gloo-resource-cleanup")

							// rollout job should wait for deployment
							rolloutJob := getJob(testManifest, namespace, "gloo-resource-rollout")
							Expect(rolloutJob.Spec.Template.Spec.Containers[0].Command[2]).To(ContainSubstring("kubectl rollout"))
						})

						It("does not wait for deployment when validation webhook is disabled", func() {
							prepareMakefile(namespace, helmValues{valuesArgs: []string{
								"gateway.validation.webhook.enabled=false",
							}})
							job := getJob(testManifest, namespace, "gloo-resource-rollout")
							Expect(job.Spec.Template.Spec.Containers[0].Command[2]).NotTo(ContainSubstring("kubectl rollout"))
						})

						It("configmap should contain gateway yaml when default gateways are enabled", func() {
							prepareMakefile(namespace, helmValues{valuesArgs: []string{
								"gatewayProxies.gatewayProxy.gatewaySettings.enabled=true",
							}})

							configMap := getConfigMap(testManifest, namespace, customResourceConfigMapName)
							Expect(configMap.Data).ToNot(BeNil())
							Expect(configMap.Data["custom-resources"]).To(ContainSubstring("kind: Gateway"))
							Expect(configMap.Data["has-custom-resources"]).To(Equal("true"))
						})

						It("configmap should not contain gateway yaml when default gateways are disabled", func() {
							prepareMakefile(namespace, helmValues{valuesArgs: []string{
								"gatewayProxies.gatewayProxy.gatewaySettings.enabled=false",
							}})

							configMap := getConfigMap(testManifest, namespace, customResourceConfigMapName)
							Expect(configMap.Data).ToNot(BeNil())
							Expect(configMap.Data["custom-resources"]).NotTo(ContainSubstring("kind:"))
							Expect(configMap.Data["has-custom-resources"]).To(Equal("false"))
						})

						It("can disable rollout, migration, and cleanup jobs", func() {
							prepareMakefile(namespace, helmValues{valuesArgs: []string{
								"gateway.rolloutJob.enabled=false",
								"gateway.cleanupJob.enabled=false",
							}})
							testManifest.Expect("Job", namespace, "gloo-resource-rollout").To(BeNil())
							testManifest.Expect("Job", namespace, "gloo-resource-migration").To(BeNil())
							testManifest.Expect("Job", namespace, "gloo-resource-cleanup").To(BeNil())
						})

						It("can set resource requirements for rollout, migration, and cleanup jobs", func() {
							prepareMakefile(namespace, helmValues{valuesArgs: []string{
								"gateway.rolloutJob.resources.requests.memory=100Mi",
								"gateway.rolloutJob.resources.requests.cpu=200m",
								"gateway.rolloutJob.resources.limits.memory=300Mi",
								"gateway.rolloutJob.resources.limits.cpu=400m",
								"gateway.cleanupJob.resources.requests.memory=150Mi",
								"gateway.cleanupJob.resources.requests.cpu=250m",
								"gateway.cleanupJob.resources.limits.memory=350Mi",
								"gateway.cleanupJob.resources.limits.cpu=450m",
							}})
							// rollout and migration jobs both use the gateway.rolloutJob helm values
							rolloutJob := getJob(testManifest, namespace, "gloo-resource-rollout")
							Expect(rolloutJob.Spec.Template.Spec.Containers[0].Resources.Requests.Memory().String()).To(Equal("100Mi"))
							Expect(rolloutJob.Spec.Template.Spec.Containers[0].Resources.Requests.Cpu().String()).To(Equal("200m"))
							Expect(rolloutJob.Spec.Template.Spec.Containers[0].Resources.Limits.Memory().String()).To(Equal("300Mi"))
							Expect(rolloutJob.Spec.Template.Spec.Containers[0].Resources.Limits.Cpu().String()).To(Equal("400m"))
							migrationJob := getJob(testManifest, namespace, "gloo-resource-migration")
							Expect(migrationJob.Spec.Template.Spec.Containers[0].Resources.Requests.Memory().String()).To(Equal("100Mi"))
							Expect(migrationJob.Spec.Template.Spec.Containers[0].Resources.Requests.Cpu().String()).To(Equal("200m"))
							Expect(migrationJob.Spec.Template.Spec.Containers[0].Resources.Limits.Memory().String()).To(Equal("300Mi"))
							Expect(migrationJob.Spec.Template.Spec.Containers[0].Resources.Limits.Cpu().String()).To(Equal("400m"))
							cleanupJob := getJob(testManifest, namespace, "gloo-resource-cleanup")
							Expect(cleanupJob.Spec.Template.Spec.Containers[0].Resources.Requests.Memory().String()).To(Equal("150Mi"))
							Expect(cleanupJob.Spec.Template.Spec.Containers[0].Resources.Requests.Cpu().String()).To(Equal("250m"))
							Expect(cleanupJob.Spec.Template.Spec.Containers[0].Resources.Limits.Memory().String()).To(Equal("350Mi"))
							Expect(cleanupJob.Spec.Template.Spec.Containers[0].Resources.Limits.Cpu().String()).To(Equal("450m"))
						})
					})

					It("creates the certgen job, rbac, and service account", func() {
						prepareMakefile(namespace, helmValues{valuesArgs: []string{
							"gateway.certGenJob.resources.requests.memory=64Mi",
							"gateway.certGenJob.resources.requests.cpu=250m",
							"gateway.certGenJob.resources.limits.memory=128Mi",
							"gateway.certGenJob.resources.limits.cpu=500m",
						}})
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
    "helm.sh/hook": pre-install,pre-upgrade
    "helm.sh/hook-delete-policy": before-hook-creation
    "helm.sh/hook-weight": "10"
spec:
  ttlSecondsAfterFinished: 60
  template:
    metadata:
      labels:
        sidecar.istio.io/inject: "false"
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
          resources:
            requests:
              cpu: 250m
              memory: 64Mi
            limits:
              cpu: 500m
              memory: 128Mi
          args:
            - "--secret-name=gateway-validation-certs"
            - "--svc-name=gloo"
            - "--validating-webhook-configuration-name=gloo-gateway-validation-webhook-` + namespace + `"
            - "--force-rotation=true"
      restartPolicy: OnFailure

`)
						testManifest.ExpectUnstructured(job.GetKind(), job.GetNamespace(), job.GetName()).To(BeEquivalentTo(job))

						clusterRole := makeUnstructured(`

# this role requires access to cluster-scoped resources
kind: ClusterRole
apiVersion: rbac.authorization.k8s.io/v1
metadata:
    name: gloo-gateway-secret-create-` + namespace + `
    labels:
        app: gloo
        gloo: rbac
    annotations:
      "helm.sh/hook": "pre-install,pre-upgrade"
      "helm.sh/hook-weight": "5"
rules:
- apiGroups: [""]
  resources: ["secrets"]
  verbs: ["create", "get", "update"]
`)
						testManifest.ExpectUnstructured(clusterRole.GetKind(), clusterRole.GetNamespace(), clusterRole.GetName()).To(BeEquivalentTo(clusterRole))

						clusterRoleBinding := makeUnstructured(`
# this role requires access to cluster-scoped resources
kind: ClusterRoleBinding
apiVersion: rbac.authorization.k8s.io/v1
metadata:
  name: gloo-gateway-secret-create-` + namespace + `
  labels:
    app: gloo
    gloo: rbac
  annotations:
    "helm.sh/hook": "pre-install,pre-upgrade"
    "helm.sh/hook-weight": "5"
subjects:
- kind: ServiceAccount
  name: certgen
  namespace: ` + namespace + `
roleRef:
  kind: ClusterRole
  name: gloo-gateway-secret-create-` + namespace + `
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
    "helm.sh/hook": "pre-install,pre-upgrade"
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
						format.MaxLength = 0
						labels = map[string]string{
							"gloo": "gloo",
							"app":  "gloo",
						}
						selector = map[string]string{
							"gloo": "gloo",
						}
						container := GetQuayContainerSpec("gloo", version, GetPodNamespaceEnvVar(), GetPodNamespaceStats(), GetValidationEnvVar())
						glooAnnotations := make(map[string]string)
						for k, v := range statsAnnotations {
							glooAnnotations[k] = v
						}
						glooAnnotations["gloo.solo.io/oss-image-tag"] = version
						rb := ResourceBuilder{
							Namespace:   namespace,
							Name:        "gloo",
							Labels:      labels,
							Annotations: glooAnnotations,
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
						},
							{
								Name: "validation-certs",
								VolumeSource: v1.VolumeSource{
									Secret: &v1.SecretVolumeSource{
										SecretName:  "gateway-validation-certs",
										DefaultMode: proto.Int(420),
									},
								},
							}}
						deploy.Spec.Template.Spec.Containers[0].VolumeMounts = []v1.VolumeMount{
							{
								Name:      "validation-certs",
								MountPath: "/etc/gateway/validation-certs",
								ReadOnly:  false,
							},
							{
								Name:      "labels-volume",
								MountPath: "/etc/gloo",
								ReadOnly:  true,
							},
						}

						deploy.Spec.Template.Spec.Containers[0].Ports = glooPorts
						deploy.Spec.Template.Spec.Containers[0].Resources = v1.ResourceRequirements{
							Requests: v1.ResourceList{
								v1.ResourceMemory: resource.MustParse("256Mi"),
								v1.ResourceCPU:    resource.MustParse("500m"),
							},
						}

						deploy.Spec.Template.Spec.Containers[0].ReadinessProbe = &v1.Probe{
							ProbeHandler: v1.ProbeHandler{
								TCPSocket: &v1.TCPSocketAction{
									Port: intstr.FromInt(9977),
								},
							},
							InitialDelaySeconds: 3,
							PeriodSeconds:       10,
							FailureThreshold:    3,
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

					It("has limits", func() {
						prepareMakefile(namespace, helmValues{
							valuesArgs: []string{
								"gloo.deployment.resources.limits.memory=2Mi",
								"gloo.deployment.resources.limits.cpu=3m",
								"gloo.deployment.resources.requests.memory=4Mi",
								"gloo.deployment.resources.requests.cpu=5m",
							},
						})

						// Add the limits we are testing:
						glooDeployment.Spec.Template.Spec.Containers[0].Resources = v1.ResourceRequirements{
							Limits: v1.ResourceList{
								v1.ResourceMemory: resource.MustParse("2Mi"),
								v1.ResourceCPU:    resource.MustParse("3m"),
							},
							Requests: v1.ResourceList{
								v1.ResourceMemory: resource.MustParse("4Mi"),
								v1.ResourceCPU:    resource.MustParse("5m"),
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
					It("can set disable leader election env var", func() {
						glooDeployment.Spec.Template.Spec.Containers[0].Env = append(
							glooDeployment.Spec.Template.Spec.Containers[0].Env,
							v1.EnvVar{
								Name:  "DISABLE_LEADER_ELECTION",
								Value: "true",
							})
						prepareMakefile(namespace, helmValues{
							valuesArgs: []string{"gloo.disableLeaderElection=true"},
						})
						testManifest.ExpectDeploymentAppsV1(glooDeployment)
					})
					It("can set secret namespace match env var", func() {
						glooDeployment.Spec.Template.Spec.Containers[0].Env = append(
							glooDeployment.Spec.Template.Spec.Containers[0].Env,
							v1.EnvVar{
								Name:  "HEADER_SECRET_REF_NS_MATCHES_US",
								Value: "true",
							})
						prepareMakefile(namespace, helmValues{
							valuesArgs: []string{"gloo.headerSecretRefNsMatchesUs=true"},
						})
						testManifest.ExpectDeploymentAppsV1(glooDeployment)
					})
					It("can disable validation", func() {
						glooDeployment.Spec.Template.Spec.Containers[0].Env = []v1.EnvVar{GetPodNamespaceEnvVar(), GetPodNamespaceStats()}
						glooDeployment.Spec.Template.Spec.Volumes = []v1.Volume{{
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
						},
						}
						glooDeployment.Spec.Template.Spec.Containers[0].VolumeMounts = []v1.VolumeMount{
							{Name: "labels-volume",
								MountPath: "/etc/gloo",
								ReadOnly:  true,
							}}
						prepareMakefile(namespace, helmValues{
							valuesArgs: []string{"gateway.validation.enabled=false"},
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

					It("is not created when service is disabled", func() {
						prepareMakefile(namespace, helmValues{
							valuesArgs: []string{
								"discovery.enabled=false",
							},
						})
						testManifest.ExpectUnstructured(
							discoveryServiceAccount.Kind,
							discoveryServiceAccount.Namespace,
							discoveryServiceAccount.Name).To(BeNil())
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

					It("has limits", func() {
						prepareMakefile(namespace, helmValues{
							valuesArgs: []string{
								"discovery.deployment.resources.limits.memory=2Mi",
								"discovery.deployment.resources.limits.cpu=3m",
								"discovery.deployment.resources.requests.memory=4Mi",
								"discovery.deployment.resources.requests.cpu=5m",
							},
						})

						// Add the limits we are testing:
						discoveryDeployment.Spec.Template.Spec.Containers[0].Resources = v1.ResourceRequirements{
							Limits: v1.ResourceList{
								v1.ResourceMemory: resource.MustParse("2Mi"),
								v1.ResourceCPU:    resource.MustParse("3m"),
							},
							Requests: v1.ResourceList{
								v1.ResourceMemory: resource.MustParse("4Mi"),
								v1.ResourceCPU:    resource.MustParse("5m"),
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

					It("allows removing pod security context", func() {
						prepareMakefile(namespace, helmValues{
							valuesArgs: []string{"discovery.deployment.enablePodSecurityContext=false"},
						})
						discoveryDeployment.Spec.Template.Spec.SecurityContext = nil
						testManifest.ExpectDeploymentAppsV1(discoveryDeployment)
					})

					It("allows disabling FDS GraphQL discovery", func() {
						settings := makeUnstructureFromTemplateFile("fixtures/settings/graphql_fds_disabled.yaml", namespace)
						prepareMakefile(namespace, helmValues{
							valuesArgs: []string{
								"discovery.fdsOptions.graphqlEnabled=false",
							},
						})
						testManifest.ExpectUnstructured(settings.GetKind(), settings.GetNamespace(), settings.GetName()).To(BeEquivalentTo(settings))
					})

					It("allows disabling upstream discovery", func() {
						settings := makeUnstructureFromTemplateFile("fixtures/settings/uds_disabled.yaml", namespace)
						prepareMakefile(namespace, helmValues{
							valuesArgs: []string{
								"discovery.udsOptions.enabled=false",
							},
						})
						testManifest.ExpectUnstructured(settings.GetKind(), settings.GetNamespace(), settings.GetName()).To(BeEquivalentTo(settings))
					})

					It("allows setting watchedLabels for discovery", func() {
						settings := makeUnstructureFromTemplateFile("fixtures/settings/watched_discovery_labels.yaml", namespace)
						prepareMakefile(namespace, helmValues{
							valuesArgs: []string{
								"discovery.udsOptions.watchLabels.A=B",
							},
						})
						testManifest.ExpectUnstructured(settings.GetKind(), settings.GetNamespace(), settings.GetName()).To(BeEquivalentTo(settings))
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

				It("should allow customising tcp keepalive on custom gateway proxies", func() {
					prepareMakefile(namespace, helmValues{
						valuesArgs: []string{"",
							"gatewayProxies.gatewayProxy.tcpKeepaliveTimeSeconds=30",
							"gatewayProxies.anotherGatewayProxy.tcpKeepaliveTimeSeconds=33",
						},
					})
					gatewayProxyKeepaliveSeen := false
					anotherGatewayProxyKeepaliveSeen := false
					testManifest.SelectResources(func(resource *unstructured.Unstructured) bool {
						return resource.GetKind() == "ConfigMap"
					}).ExpectAll(func(configMap *unstructured.Unstructured) {
						configMapObject, err := kuberesource.ConvertUnstructured(configMap)
						Expect(err).NotTo(HaveOccurred(), fmt.Sprintf("ConfigMap %+v should be able to convert from unstructured", configMap))
						structuredConfigMap, ok := configMapObject.(*v1.ConfigMap)
						Expect(ok).To(BeTrue(), fmt.Sprintf("configMap %+v should be able to cast to a structured configMap", configMap))
						configMapName := structuredConfigMap.GetName()
						envoyConfig := structuredConfigMap.Data["envoy.yaml"]
						if configMapName == "gateway-proxy-envoy-config" {
							gatewayProxyKeepaliveSeen = true
							Expect(envoyConfig).To(ContainSubstring("keepalive_time: 30"), "Should set custom value for gatewayProxy")
						} else if configMapName == "another-gateway-proxy-envoy-config" {
							anotherGatewayProxyKeepaliveSeen = true
							Expect(envoyConfig).To(ContainSubstring("keepalive_time: 33"), "Should set custom value for another custom gatewayProxy value")
						}
					})
					Expect(gatewayProxyKeepaliveSeen).To(BeTrue())
					Expect(anotherGatewayProxyKeepaliveSeen).To(BeTrue())
				})

				It("should allow customising tcp keepalive", func() {
					prepareMakefile(namespace, helmValues{
						valuesArgs: []string{"",
							"gatewayProxies.gatewayProxy.tcpKeepaliveTimeSeconds=30"},
					})
					byt, err := os.ReadFile("fixtures/envoy_config/tcp_keepalive.yaml")
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

					byt, err := os.ReadFile("fixtures/envoy_config/static_clusters.yaml")
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

				It("can create a gateway proxy with added overload manager config", func() {
					prepareMakefile(namespace, helmValues{
						valuesArgs: []string{
							"gatewayProxies.gatewayProxy.envoyOverloadManager.enabled=true",
							"gatewayProxies.gatewayProxy.envoyOverloadManager.refreshInterval=2s",
							"gatewayProxies.gatewayProxy.disabled=false"},
					})

					byt, err := os.ReadFile("fixtures/envoy_config/overload_manager.yaml")
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

					byt, err := os.ReadFile("fixtures/envoy_config/bootstrap_extensions.yaml")
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

					byt, err := os.ReadFile("fixtures/envoy_config/custom_static_bootstrap.yaml")
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

				It("can create arbitrary configmaps", func() {
					prepareMakefile(namespace, helmValues{
						valuesArgs: []string{"global.configMaps[0].name=my-config-map",
							"global.configMaps[0].namespace=my-ns",
							"global.configMaps[0].data.key1=value1",
							"global.configMaps[0].data.key2=value2"},
					})
					cmRb := ResourceBuilder{
						Namespace: "my-ns",
						Name:      "my-config-map",
						Labels:    map[string]string{"app": "gloo", "gloo": "gloo"},
						Data:      map[string]string{"key1": "value1", "key2": "value2"},
					}
					cm := cmRb.GetConfigMap()
					testManifest.ExpectConfigMapWithYamlData(cm)
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
							valuesArgs: []string{
								"gatewayProxies.gatewayProxy.readConfig=true",
								"gatewayProxies.gatewayProxy.readConfigMulticluster=true",
							},
						})
						serviceLabels := map[string]string{
							"app":              "gloo",
							"gloo":             "gateway-proxy",
							"gateway-proxy-id": "gateway-proxy",
						}
						serviceSelector := map[string]string{
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
						gatewayProxyConfigDumpService.Spec.Selector = serviceSelector
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
								"gatewayProxies.gatewayProxyInternal.kind.deployment.replicas=0",
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
					glooAnnotations := make(map[string]string)
					for k, v := range statsAnnotations {
						glooAnnotations[k] = v
					}
					glooAnnotations["gloo.solo.io/oss-image-tag"] = version
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
									Annotations: glooAnnotations,
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
									},
									},
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
													Name: statusutils.PodNamespaceEnvName,
													ValueFrom: &v1.EnvVarSource{
														FieldRef: &v1.ObjectFieldSelector{APIVersion: "", FieldPath: "metadata.namespace"},
													},
												},
												{
													Name:  "START_STATS_SERVER",
													Value: "true",
												},
												{
													Name:  "VALIDATION_MUST_START",
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
												ProbeHandler: v1.ProbeHandler{
													TCPSocket: &v1.TCPSocketAction{
														Port: intstr.FromInt(9977),
													},
												},
												InitialDelaySeconds: 3,
												PeriodSeconds:       10,
												FailureThreshold:    3,
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
													Name: statusutils.PodNamespaceEnvName,
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
				expectNonRoot := func(testManifest TestManifest) {
					deployments := testManifest.SelectResources(func(resource *unstructured.Unstructured) bool {
						return resource.GetKind() == "Deployment"
					})

					Expect(deployments.NumResources()).NotTo(BeZero())

					deployments.ExpectAll(func(resource *unstructured.Unstructured) {
						rawDeploy, err := resource.MarshalJSON()
						Expect(err).NotTo(HaveOccurred(), "resource.MarshalJSON error")

						deploy := appsv1.Deployment{}
						err = json.Unmarshal(rawDeploy, &deploy)
						Expect(err).NotTo(HaveOccurred(), "json.Unmarshall error")

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

			Describe("Standard k8s values", func() {
				DescribeTable("PodSpec affinity, tolerations, nodeName, hostAliases, nodeSelector, priorityClassName, restartPolicy, initContainers, on Deployments and Jobs",
					func(kind string, resourceName string, value string, extraArgs ...string) {
						prepareMakefile(namespace, helmValues{
							valuesArgs: append([]string{
								value + ".nodeSelector.label=someLabel",
								value + ".nodeName=someNodeName",
								value + ".tolerations[0].operator=someToleration",
								value + ".hostAliases[0]=someHostAlias",
								value + ".affinity.nodeAffinity=someNodeAffinity",
								value + ".restartPolicy=someRestartPolicy",
								value + ".priorityClassName=somePriorityClass",
								value + ".initContainers[0].image=gcr.io/solo-public",
								value + ".initContainers[0].name=containerName",
								value + ".initContainers[0].command[0]=sh",
								value + ".initContainers[0].command[1]=-c",
							}, extraArgs...),
						})
						resources := testManifest.SelectResources(func(u *unstructured.Unstructured) bool {
							var prefixPath []string
							if kind == "CronJob" {
								prefixPath = []string{"spec", "jobTemplate"}
							}
							if u.GetKind() == kind && u.GetName() == resourceName {
								a := getFieldFromUnstructured(u, append(prefixPath, "spec", "template", "spec", "nodeSelector")...)
								Expect(a).To(Equal(map[string]interface{}{"label": "someLabel"}))
								a = getFieldFromUnstructured(u, append(prefixPath, "spec", "template", "spec", "priorityClassName")...)
								Expect(a).To(Equal("somePriorityClass"))
								a = getFieldFromUnstructured(u, append(prefixPath, "spec", "template", "spec", "nodeName")...)
								Expect(a).To(Equal("someNodeName"))
								a = getFieldFromUnstructured(u, append(prefixPath, "spec", "template", "spec", "tolerations")...)
								Expect(a).To(Equal([]interface{}{map[string]interface{}{"operator": "someToleration"}}))
								a = getFieldFromUnstructured(u, append(prefixPath, "spec", "template", "spec", "hostAliases")...)
								Expect(a).To(Equal([]interface{}{"someHostAlias"}))
								a = getFieldFromUnstructured(u, append(prefixPath, "spec", "template", "spec", "affinity")...)
								Expect(a).To(Equal(map[string]interface{}{"nodeAffinity": "someNodeAffinity"}))
								a = getFieldFromUnstructured(u, append(prefixPath, "spec", "template", "spec", "restartPolicy")...)
								Expect(a).To(Equal("someRestartPolicy"))
								a = getFieldFromUnstructured(u, append(prefixPath, "spec", "template", "spec", "initContainers")...)
								Expect(a).To(Equal([]interface{}{map[string]interface{}{"image": "gcr.io/solo-public", "name": "containerName", "command": []interface{}{"sh", "-c"}}}))
								return true
							}
							return false
						})
						Expect(resources.NumResources()).To(Equal(1))
					},
					Entry("gloo deployment", "Deployment", "gloo", "gloo.deployment"),
					Entry("discovery deployment", "Deployment", "discovery", "discovery.deployment"),
					Entry("ingress deployment", "Deployment", "ingress", "ingress.deployment", "ingress.enabled=true"),
					Entry("cluster-ingress deployment", "Deployment", "clusteringress-proxy", "settings.integrations.knative.proxy", "settings.integrations.knative.version=0.7.0", "settings.integrations.knative.enabled=true"),
					Entry("knative external proxy deployment", "Deployment", "knative-external-proxy", "settings.integrations.knative.proxy", "settings.integrations.knative.version=0.9.0", "settings.integrations.knative.enabled=true"),
					Entry("knative internal proxy deployment", "Deployment", "knative-internal-proxy", "settings.integrations.knative.proxy", "settings.integrations.knative.version=0.9.0", "settings.integrations.knative.enabled=true"),
					Entry("gateway certgen job", "Job", "gateway-certgen", "gateway.certGenJob"),
					Entry("mtls certgen job", "Job", "gloo-mtls-certgen", "gateway.certGenJob", "global.glooMtls.enabled=true"),
					Entry("mtls certgen cronjob", "CronJob", "gloo-mtls-certgen-cronjob", "gateway.certGenJob", "global.glooMtls.enabled=true", "gateway.certGenJob.cron.enabled=true"),
					Entry("gateway certgen cronjob", "CronJob", "gateway-certgen-cronjob", "gateway.certGenJob", "gateway.enabled=true", "gateway.validation.enabled=true", "gateway.validation.webhook.enabled=true", "gateway.certGenJob.cron.enabled=true"),
					Entry("resource rollout job", "Job", "gloo-resource-rollout", "gateway.rolloutJob"),
					Entry("resource migration job", "Job", "gloo-resource-migration", "gateway.rolloutJob"),
					Entry("resource cleanup job", "Job", "gloo-resource-cleanup", "gateway.cleanupJob"),
				)

				const (
					ACTIVE_DEADLINE_SECONDS    = 123
					TTL_SECONDS_AFTER_FINISHED = 42
					BACKOFF_LIMIT              = 10
					COMPLETIONS                = 5
					MANUAL_SELECTOR            = true
					PARALLELISM                = 7
				)
				DescribeTable("can set JobSpec fields on Jobs",
					func(kind string, resourceName string, jobValuesPrefix string, extraArgs ...string) {
						prepareMakefile(namespace, helmValues{
							valuesArgs: append([]string{
								jobValuesPrefix + ".activeDeadlineSeconds=" + strconv.Itoa(ACTIVE_DEADLINE_SECONDS),
								jobValuesPrefix + ".ttlSecondsAfterFinished=" + strconv.Itoa(TTL_SECONDS_AFTER_FINISHED),
								jobValuesPrefix + ".backoffLimit=" + strconv.Itoa(BACKOFF_LIMIT),
								jobValuesPrefix + ".completions=" + strconv.Itoa(COMPLETIONS),
								jobValuesPrefix + ".manualSelector=" + strconv.FormatBool(MANUAL_SELECTOR),
								jobValuesPrefix + ".parallelism=" + strconv.Itoa(PARALLELISM),
							}, extraArgs...),
						})
						resources := testManifest.SelectResources(func(u *unstructured.Unstructured) bool {
							var prefixPath []string
							if kind == "CronJob" {
								prefixPath = []string{"spec", "jobTemplate"}
							}
							if u.GetKind() == kind && u.GetName() == resourceName {
								a := getFieldFromUnstructured(u, append(prefixPath, "spec", "activeDeadlineSeconds")...)
								Expect(a).To(Equal(int64(ACTIVE_DEADLINE_SECONDS)))
								a = getFieldFromUnstructured(u, append(prefixPath, "spec", "ttlSecondsAfterFinished")...)
								Expect(a).To(Equal(int64(TTL_SECONDS_AFTER_FINISHED)))
								a = getFieldFromUnstructured(u, append(prefixPath, "spec", "backoffLimit")...)
								Expect(a).To(Equal(int64(BACKOFF_LIMIT)))
								a = getFieldFromUnstructured(u, append(prefixPath, "spec", "completions")...)
								Expect(a).To(Equal(int64(COMPLETIONS)))
								a = getFieldFromUnstructured(u, append(prefixPath, "spec", "parallelism")...)
								Expect(a).To(Equal(int64(PARALLELISM)))
								a = getFieldFromUnstructured(u, append(prefixPath, "spec", "manualSelector")...)
								Expect(a).To(Equal(MANUAL_SELECTOR))
								return true
							}
							return false
						})
						Expect(resources.NumResources()).To(Equal(1))
					},
					Entry("gateway certgen job", "Job", "gateway-certgen", "gateway.certGenJob"),
					Entry("mtls certgen job", "Job", "gloo-mtls-certgen", "gateway.certGenJob", "global.glooMtls.enabled=true"),
					Entry("mtls certgen cronjob", "CronJob", "gloo-mtls-certgen-cronjob", "gateway.certGenJob", "global.glooMtls.enabled=true", "gateway.certGenJob.cron.enabled=true"),
					Entry("gateway certgen cronjob", "CronJob", "gateway-certgen-cronjob", "gateway.certGenJob", "gateway.enabled=true", "gateway.validation.enabled=true", "gateway.validation.webhook.enabled=true", "gateway.certGenJob.cron.enabled=true"),
					Entry("resource rollout job", "Job", "gloo-resource-rollout", "gateway.rolloutJob"),
					Entry("resource migration job", "Job", "gloo-resource-migration", "gateway.rolloutJob"),
					Entry("resource cleanup job", "Job", "gloo-resource-cleanup", "gateway.cleanupJob"),
				)

				DescribeTable("setTtlAfterFinished=false suppresses ttlSecondsAfterFinished on Jobs",
					func(kind string, resourceName string, jobValuesPrefix string, extraArgs ...string) {
						prepareMakefile(namespace, helmValues{
							valuesArgs: append([]string{
								jobValuesPrefix + ".setTtlAfterFinished=false",
								jobValuesPrefix + ".ttlSecondsAfterFinished=" + strconv.Itoa(TTL_SECONDS_AFTER_FINISHED),
							}, extraArgs...),
						})
						resources := testManifest.SelectResources(func(u *unstructured.Unstructured) bool {
							var prefixPath []string
							if kind == "CronJob" {
								prefixPath = []string{"spec", "jobTemplate"}
							}
							if u.GetKind() == kind && u.GetName() == resourceName {
								a := getFieldFromUnstructured(u, append(prefixPath, "spec", "ttlSecondsAfterFinished")...)
								Expect(a).To(BeNil())
								if kind == "Job" {
									helmHookDeletePolicy, ok := u.GetAnnotations()["helm.sh/hook-delete-policy"]
									Expect(ok).To(BeTrue())
									Expect(helmHookDeletePolicy).To(ContainSubstring("hook-succeeded"))
								}
								return true
							}
							return false
						})
						Expect(resources.NumResources()).To(Equal(1))
					},
					Entry("gateway certgen job", "Job", "gateway-certgen", "gateway.certGenJob"),
					Entry("mtls certgen job", "Job", "gloo-mtls-certgen", "gateway.certGenJob", "global.glooMtls.enabled=true"),
					Entry("mtls certgen cronjob", "CronJob", "gloo-mtls-certgen-cronjob", "gateway.certGenJob", "global.glooMtls.enabled=true", "gateway.certGenJob.cron.enabled=true"),
					Entry("gateway certgen cronjob", "CronJob", "gateway-certgen-cronjob", "gateway.certGenJob", "gateway.enabled=true", "gateway.validation.enabled=true", "gateway.validation.webhook.enabled=true", "gateway.certGenJob.cron.enabled=true"),
				)

				DescribeTable("Setting setTtlAfterFinished=true includes ttlSecondsAfterFinished on Jobs",
					func(kind string, resourceName string, jobValuesPrefix string, expectAnnotation bool, extraArgs ...string) {
						prepareMakefile(namespace, helmValues{
							valuesArgs: append([]string{
								jobValuesPrefix + ".setTtlAfterFinished=true",
								jobValuesPrefix + ".ttlSecondsAfterFinished=" + strconv.Itoa(TTL_SECONDS_AFTER_FINISHED),
							}, extraArgs...),
						})
						resources := testManifest.SelectResources(func(u *unstructured.Unstructured) bool {
							var prefixPath []string
							if kind == "CronJob" {
								prefixPath = []string{"spec", "jobTemplate"}
							}
							if u.GetKind() == kind && u.GetName() == resourceName {
								a := getFieldFromUnstructured(u, append(prefixPath, "spec", "ttlSecondsAfterFinished")...)
								Expect(a).To(Equal(int64(TTL_SECONDS_AFTER_FINISHED)))
								if kind == "Job" {
									helmHookDeletePolicy, ok := u.GetAnnotations()["helm.sh/hook-delete-policy"]
									if expectAnnotation {
										Expect(ok).To(BeTrue())
										Expect(helmHookDeletePolicy).NotTo(ContainSubstring("hook-succeeded"))
									} else {
										Expect(ok).To(BeFalse())
									}
								}
								return true
							}
							return false
						})
						Expect(resources.NumResources()).To(Equal(1))
					},
					Entry("gateway certgen job", "Job", "gateway-certgen", "gateway.certGenJob", true),
					Entry("mtls certgen job", "Job", "gloo-mtls-certgen", "gateway.certGenJob", false, "global.glooMtls.enabled=true"),
					Entry("mtls certgen cronjob", "CronJob", "gloo-mtls-certgen-cronjob", "gateway.certGenJob", false, "global.glooMtls.enabled=true", "gateway.certGenJob.cron.enabled=true"),
				)

				DescribeTable("When setTtlAfterFinished is unset, includes ttlSecondsAfterFinished on Jobs",
					func(kind string, resourceName string, jobValuesPrefix string, expectAnnotation bool, extraArgs ...string) {
						prepareMakefile(namespace, helmValues{
							valuesArgs: append([]string{
								jobValuesPrefix + ".ttlSecondsAfterFinished=" + strconv.Itoa(TTL_SECONDS_AFTER_FINISHED),
							}, extraArgs...),
						})
						resources := testManifest.SelectResources(func(u *unstructured.Unstructured) bool {
							var prefixPath []string
							if kind == "CronJob" {
								prefixPath = []string{"spec", "jobTemplate"}
							}
							if u.GetKind() == kind && u.GetName() == resourceName {
								a := getFieldFromUnstructured(u, append(prefixPath, "spec", "ttlSecondsAfterFinished")...)
								Expect(a).To(Equal(int64(TTL_SECONDS_AFTER_FINISHED)))
								if kind == "Job" {
									helmHookDeletePolicy, ok := u.GetAnnotations()["helm.sh/hook-delete-policy"]
									if expectAnnotation {
										Expect(ok).To(BeTrue())
										Expect(helmHookDeletePolicy).NotTo(ContainSubstring("hook-succeeded"))
									} else if resourceName == "gloo-mtls-certgen" {
										Expect(ok).To(BeFalse())
									}
								}
								return true
							}
							return false
						})
						Expect(resources.NumResources()).To(Equal(1))
					},
					Entry("gateway certgen job", "Job", "gateway-certgen", "gateway.certGenJob", true),
					Entry("mtls certgen job", "Job", "gloo-mtls-certgen", "gateway.certGenJob", false, "global.glooMtls.enabled=true"),
					Entry("mtls certgen cronjob", "CronJob", "gloo-mtls-certgen-cronjob", "gateway.certGenJob", false, "global.glooMtls.enabled=true", "gateway.certGenJob.cron.enabled=true"),
					Entry("gateway certgen cronjob", "CronJob", "gateway-certgen-cronjob", "gateway.certGenJob", false, "gateway.enabled=true", "gateway.validation.enabled=true", "gateway.validation.webhook.enabled=true", "gateway.certGenJob.cron.enabled=true"),
				)

				DescribeTable("by default, activeDeadlineSeconds is unset on Jobs",
					func(kind string, resourceName string, extraArgs ...string) {
						prepareMakefile(namespace, helmValues{
							valuesArgs: append([]string{}, extraArgs...),
						})
						resources := testManifest.SelectResources(func(u *unstructured.Unstructured) bool {
							var prefixPath []string
							if kind == "CronJob" {
								prefixPath = []string{"spec", "jobTemplate"}
							}
							if u.GetKind() == kind && u.GetName() == resourceName {
								a := getFieldFromUnstructured(u, append(prefixPath, "spec", "activeDeadlineSeconds")...)
								Expect(a).To(BeNil())
								return true
							}
							return false
						})
						Expect(resources.NumResources()).To(Equal(1))
					},
					Entry("gateway certgen job", "Job", "gateway-certgen"),
					Entry("mtls certgen job", "Job", "gloo-mtls-certgen", "global.glooMtls.enabled=true"),
					Entry("mtls certgen cronjob", "CronJob", "gloo-mtls-certgen-cronjob", "global.glooMtls.enabled=true", "gateway.certGenJob.cron.enabled=true"),
					Entry("gateway certgen cronjob", "CronJob", "gateway-certgen-cronjob", "gateway.enabled=true", "gateway.validation.enabled=true", "gateway.validation.webhook.enabled=true", "gateway.certGenJob.cron.enabled=true"),
					Entry("resource rollout job", "Job", "gloo-resource-rollout"),
					Entry("resource migration job", "Job", "gloo-resource-migration"),
					Entry("resource cleanup job", "Job", "gloo-resource-cleanup"),
				)

				DescribeTable("overrides resources for container security contexts", func(resourceName string, containerName string, securityRoot string, extraArgs ...string) {
					// Split the securityContext fields into two groups so each one gets tested as in and not in the values file
					helmValuesA := securityContextFieldsStripeGroupA(securityRoot, extraArgs...)

					// 2nd group of fields to test. Switch up the default 'no-merge' to the explict version
					extraArgs = append([]string{
						securityRoot + ".mergePolicy=no-merge",
					}, extraArgs...)
					helmValuesB := securityContextFieldsStripeGroupB(securityRoot, extraArgs...)

					// First Group of fields to test
					prepareMakefile(namespace, helmValuesA)

					container := getContainer(testManifest, "Deployment", resourceName, containerName)
					Expect(container.SecurityContext).To(Equal(
						&v1.SecurityContext{
							RunAsUser:                pointer.Int64(int64(1234)),
							AllowPrivilegeEscalation: pointer.Bool(true),
							ReadOnlyRootFilesystem:   pointer.Bool(true),
							RunAsNonRoot:             pointer.Bool(true),
							SELinuxOptions: &v1.SELinuxOptions{
								Level: "seLevel",
								Role:  "seRole",
								Type:  "seType",
								User:  "seUser",
							},
						},
					))

					prepareMakefile(namespace, helmValuesB)

					container = getContainer(testManifest, "Deployment", resourceName, containerName)
					Expect(container.SecurityContext).To(Equal(
						&v1.SecurityContext{
							Capabilities: &v1.Capabilities{
								Add:  []v1.Capability{"ADD"},
								Drop: []v1.Capability{"DROP"},
							},
							SeccompProfile: &v1.SeccompProfile{
								LocalhostProfile: pointer.String("seccompLHP"),
								Type:             "seccompType",
							},
							WindowsOptions: &v1.WindowsSecurityContextOptions{
								GMSACredentialSpecName: pointer.String("winGmsaCredSpecName"),
								GMSACredentialSpec:     pointer.String("winGmsaCredSpec"),
								RunAsUserName:          pointer.String("winUser"),
								HostProcess:            pointer.Bool(true),
							},
						},
					))
				},
					Entry("7-gateway-proxy-deployment-gateway-proxy", "gateway-proxy", "gateway-proxy", "gatewayProxies.gatewayProxy.podTemplate.glooContainerSecurityContext"),
					Entry("7-gateway-proxy-deployment-sds", "gateway-proxy", "sds", "global.glooMtls.sds.securityContext", "global.glooMtls.enabled=true"),
					Entry("7-gateway-proxy-deployment-istio-proxy", "gateway-proxy", "istio-proxy", "global.glooMtls.istioProxy.securityContext", "global.istioSDS.enabled=true"),
					Entry("1-gloo-deployment-gloo", "gloo", "gloo", "gloo.deployment.glooContainerSecurityContext", "global.glooMtls.enabled=true"),
					Entry("1-gloo-deployment-envoy-sidecar", "gloo", "envoy-sidecar", "global.glooMtls.envoy.securityContext", "global.glooMtls.enabled=true"),
					Entry("1-gloo-deployment-sds", "gloo", "sds", "global.glooMtls.sds.securityContext", "global.glooMtls.enabled=true"),
				)

				DescribeTable("merges resources for container security contexts", func(resourceName string, containerName string, securityRoot string, extraArgs ...string) {
					// First helm template generates the baseline
					// Run once "plain" to get the baseline as generated with no helm values passed
					prepareMakefile(namespace, helmValues{
						valuesArgs: extraArgs,
					})

					// Strip out the revelant security context
					container := getContainer(testManifest, "Deployment", resourceName, containerName)
					initialSecurityContext := container.SecurityContext
					if initialSecurityContext == nil {
						initialSecurityContext = &v1.SecurityContext{}
					}
					// Make a copy so we can restore it later
					var initialSecurityContextClean *v1.SecurityContext
					deepCopy(&initialSecurityContext, &initialSecurityContextClean)

					// add "mergePolicy" to the security context
					extraArgs = append([]string{
						securityRoot + ".mergePolicy=helm-merge",
					}, extraArgs...)

					// Split the securityContext fields into two groups so each one gets tested as in and not in the values file
					helmValuesA := securityContextFieldsStripeGroupA(securityRoot, extraArgs...)
					helmValuesB := securityContextFieldsStripeGroupB(securityRoot, extraArgs...)

					//
					// Run again with security context A helm values applied
					prepareMakefile(namespace, helmValuesA)
					container = getContainer(testManifest, "Deployment", resourceName, containerName)

					// Test that all the values are set
					Expect(container.SecurityContext.RunAsNonRoot).To(Equal(pointer.Bool(true)))
					initialSecurityContext.RunAsNonRoot = pointer.Bool(true)
					Expect(container.SecurityContext.RunAsUser).To(Equal(pointer.Int64(int64(1234))))
					initialSecurityContext.RunAsUser = pointer.Int64(int64(1234))
					Expect(container.SecurityContext.AllowPrivilegeEscalation).To(Equal(pointer.Bool(true)))
					initialSecurityContext.AllowPrivilegeEscalation = pointer.Bool(true)
					Expect(container.SecurityContext.ReadOnlyRootFilesystem).To(Equal(pointer.Bool(true)))
					initialSecurityContext.ReadOnlyRootFilesystem = pointer.Bool(true)
					expectedSELinuxOptions := &v1.SELinuxOptions{
						Level: "seLevel",
						Role:  "seRole",
						Type:  "seType",
						User:  "seUser",
					}
					Expect(container.SecurityContext.SELinuxOptions).To(Equal(expectedSELinuxOptions))
					initialSecurityContext.SELinuxOptions = expectedSELinuxOptions

					// We've test/updated all the fields we passed, so now we can compare the whole thing
					Expect(container.SecurityContext).To(Equal(initialSecurityContext))

					//
					// Run again with security context B
					prepareMakefile(namespace, helmValuesB)
					initialSecurityContext = &v1.SecurityContext{}
					deepCopy(&initialSecurityContextClean, &initialSecurityContext)
					container = getContainer(testManifest, "Deployment", resourceName, containerName)

					// Test that all the values are set
					expectedCapabilities := &v1.Capabilities{
						Add:  []v1.Capability{"ADD"},
						Drop: []v1.Capability{"DROP"},
					}
					Expect(container.SecurityContext.Capabilities).To(Equal(expectedCapabilities))
					initialSecurityContext.Capabilities = expectedCapabilities
					expectedSeccompProfile := &v1.SeccompProfile{
						LocalhostProfile: pointer.String("seccompLHP"),
						Type:             "seccompType",
					}
					Expect(container.SecurityContext.SeccompProfile).To(Equal(expectedSeccompProfile))
					initialSecurityContext.SeccompProfile = expectedSeccompProfile
					expectedWindowsOptions := &v1.WindowsSecurityContextOptions{
						GMSACredentialSpecName: pointer.String("winGmsaCredSpecName"),
						GMSACredentialSpec:     pointer.String("winGmsaCredSpec"),
						RunAsUserName:          pointer.String("winUser"),
						HostProcess:            pointer.Bool(true),
					}
					Expect(container.SecurityContext.WindowsOptions).To(Equal(expectedWindowsOptions))
					initialSecurityContext.WindowsOptions = expectedWindowsOptions

					Expect(container.SecurityContext).To(Equal(initialSecurityContext))
				},
					Entry("7-gateway-proxy-deployment-gateway-proxy", "gateway-proxy", "gateway-proxy", "gatewayProxies.gatewayProxy.podTemplate.glooContainerSecurityContext"),
					Entry("7-gateway-proxy-deployment-sds", "gateway-proxy", "sds", "global.glooMtls.sds.securityContext", "global.glooMtls.enabled=true"),
					Entry("7-gateway-proxy-deployment-istio-proxy", "gateway-proxy", "istio-proxy", "global.glooMtls.istioProxy.securityContext", "global.istioSDS.enabled=true"),
					Entry("1-gloo-deployment-gloo", "gloo", "gloo", "gloo.deployment.glooContainerSecurityContext", "global.glooMtls.enabled=true"),
					Entry("1-gloo-deployment-envoy-sidecar", "gloo", "envoy-sidecar", "global.glooMtls.envoy.securityContext", "global.glooMtls.enabled=true"),
					Entry("1-gloo-deployment-sds", "gloo", "sds", "global.glooMtls.sds.securityContext", "global.glooMtls.enabled=true"),
				)

				DescribeTable("overrides resources for pod security contexts", func(resourceName string, securityRoot string, extraArgs ...string) {
					helmValuesA := podSecurityContextFieldsStripeGroupA(securityRoot, extraArgs...)

					// 2nd group of fields to test. Switch up the default 'no-merge' to the explict version
					extraArgs = append([]string{
						securityRoot + ".mergePolicy=no-merge",
					}, extraArgs...)
					helmValuesB := podSecurityContextFieldsStripeGroupB(securityRoot, extraArgs...)

					prepareMakefile(namespace, helmValuesA)
					structuredDeployment := getStructuredDeployment(testManifest, resourceName)

					fsGroupChangePolicy := v1.PodFSGroupChangePolicy("fsGroupChangePolicyValue")
					Expect(structuredDeployment.Spec.Template.Spec.SecurityContext).To(Equal(
						&v1.PodSecurityContext{
							FSGroup:             pointer.Int64(int64(101010)),
							FSGroupChangePolicy: &fsGroupChangePolicy,
							RunAsGroup:          pointer.Int64(int64(202020)),
							RunAsNonRoot:        pointer.Bool(true),
							RunAsUser:           pointer.Int64(int64(303030)),
							SupplementalGroups:  []int64{int64(11), int64(22), int64(33)},
							SELinuxOptions: &v1.SELinuxOptions{
								Level: "seLevel",
								Role:  "seRole",
								Type:  "seType",
								User:  "seUser",
							},
						},
					))

					prepareMakefile(namespace, helmValuesB)
					structuredDeployment = getStructuredDeployment(testManifest, resourceName)

					Expect(structuredDeployment.Spec.Template.Spec.SecurityContext).To(Equal(
						&v1.PodSecurityContext{
							SeccompProfile: &v1.SeccompProfile{
								LocalhostProfile: pointer.String("seccompLHP"),
								Type:             "seccompType",
							},
							WindowsOptions: &v1.WindowsSecurityContextOptions{
								GMSACredentialSpecName: pointer.String("winGmsaCredSpecName"),
								GMSACredentialSpec:     pointer.String("winGmsaCredSpec"),
								RunAsUserName:          pointer.String("winUser"),
								HostProcess:            pointer.Bool(true),
							},
							Sysctls: []v1.Sysctl{
								{
									Name:  "sysctlName",
									Value: "sysctlValue",
								},
							},
						},
					))

				},
					Entry("7-gateway-proxy-deployment", "gateway-proxy", "gatewayProxies.gatewayProxy.podTemplate.podSecurityContext"),
					Entry("1-gloo-deployment", "gloo", "gloo.deployment.podSecurityContext"),
				)

				DescribeTable("merges resources for pod security contexts", func(resourceName string, securityRoot string, extraArgs ...string) {
					// First run with no extra helm security context values
					prepareMakefile(namespace, helmValues{
						valuesArgs: extraArgs,
					})

					structuredDeployment := getStructuredDeployment(testManifest, resourceName)

					initialSecurityContext := structuredDeployment.Spec.Template.Spec.SecurityContext
					if initialSecurityContext == nil {
						initialSecurityContext = &v1.PodSecurityContext{}
					}

					var initialSecurityContextClean *v1.PodSecurityContext
					deepCopy(&initialSecurityContext, &initialSecurityContextClean)

					extraArgs = append([]string{
						securityRoot + ".mergePolicy=helm-merge",
					}, extraArgs...)

					fsGroupChangePolicy := v1.PodFSGroupChangePolicy("fsGroupChangePolicyValue")
					// Test every field, so split it up so each is set in one test and not in the other
					helmValuesA := podSecurityContextFieldsStripeGroupA(securityRoot, extraArgs...)
					helmValuesB := podSecurityContextFieldsStripeGroupB(securityRoot, extraArgs...)

					//
					// Stripe group A
					//
					prepareMakefile(namespace, helmValuesA)
					structuredDeployment = getStructuredDeployment(testManifest, resourceName)
					// Find the relevant resource and test that its values were updated
					// Apply the new values to the initial security context and compare. They should be the same

					// Check the fields individually:
					Expect(structuredDeployment.Spec.Template.Spec.SecurityContext.FSGroup).To(Equal(pointer.Int64(int64(101010))))
					initialSecurityContext.FSGroup = pointer.Int64(int64(101010))

					Expect(structuredDeployment.Spec.Template.Spec.SecurityContext.FSGroupChangePolicy).To(Equal(&fsGroupChangePolicy))
					initialSecurityContext.FSGroupChangePolicy = &fsGroupChangePolicy

					Expect(structuredDeployment.Spec.Template.Spec.SecurityContext.RunAsGroup).To(Equal(pointer.Int64(int64(202020))))
					initialSecurityContext.RunAsGroup = pointer.Int64(int64(202020))

					Expect(structuredDeployment.Spec.Template.Spec.SecurityContext.RunAsNonRoot).To(Equal(pointer.Bool(true)))
					initialSecurityContext.RunAsNonRoot = pointer.Bool(true)

					Expect(structuredDeployment.Spec.Template.Spec.SecurityContext.RunAsUser).To(Equal(pointer.Int64(int64(303030))))
					initialSecurityContext.RunAsUser = pointer.Int64(int64(303030))

					Expect(structuredDeployment.Spec.Template.Spec.SecurityContext.SupplementalGroups).To(Equal(
						[]int64{11, 22, 33}))
					initialSecurityContext.SupplementalGroups = []int64{11, 22, 33}

					expectedSELinuxOptions := &v1.SELinuxOptions{
						Level: "seLevel",
						Role:  "seRole",
						Type:  "seType",
						User:  "seUser",
					}
					Expect(structuredDeployment.Spec.Template.Spec.SecurityContext.SELinuxOptions).To(Equal(expectedSELinuxOptions))
					initialSecurityContext.SELinuxOptions = expectedSELinuxOptions

					Expect(structuredDeployment.Spec.Template.Spec.SecurityContext).To(Equal(initialSecurityContext))

					//
					// Stripe Group B
					//
					initialSecurityContext = &v1.PodSecurityContext{}
					deepCopy(&initialSecurityContextClean, &initialSecurityContext)
					prepareMakefile(namespace, helmValuesB)

					// Find the relevant resource and test that its values were updated
					// Apply the new values to the initial security context and compare. They should be the same
					structuredDeployment = getStructuredDeployment(testManifest, resourceName)

					// Check the fields individually:
					expectedSeccompProfile := &v1.SeccompProfile{
						LocalhostProfile: pointer.String("seccompLHP"),
						Type:             "seccompType",
					}
					Expect(structuredDeployment.Spec.Template.Spec.SecurityContext.SeccompProfile).To(Equal(expectedSeccompProfile))
					initialSecurityContext.SeccompProfile = expectedSeccompProfile

					expectedWindowsOptions := &v1.WindowsSecurityContextOptions{
						GMSACredentialSpec:     pointer.String("winGmsaCredSpec"),
						GMSACredentialSpecName: pointer.String("winGmsaCredSpecName"),
						HostProcess:            pointer.Bool(true),
						RunAsUserName:          pointer.String("winUser"),
					}
					Expect(structuredDeployment.Spec.Template.Spec.SecurityContext.WindowsOptions).To(Equal(expectedWindowsOptions))
					initialSecurityContext.WindowsOptions = expectedWindowsOptions

					expectedSysctls := []v1.Sysctl{
						{
							Name:  "sysctlName",
							Value: "sysctlValue",
						},
					}

					Expect(structuredDeployment.Spec.Template.Spec.SecurityContext.Sysctls).To(Equal(expectedSysctls))
					initialSecurityContext.Sysctls = expectedSysctls

					Expect(structuredDeployment.Spec.Template.Spec.SecurityContext).To(Equal(initialSecurityContext))

				},
					Entry("7-gateway-proxy-deployment", "gateway-proxy", "gatewayProxies.gatewayProxy.podTemplate.podSecurityContext"),
					Entry("1-gloo-deployment", "gloo", "gloo.deployment.podSecurityContext"),
				)

				DescribeTable("floatingUserId is properly applied (container)", func(resourceName string, containerName string, fpuidRoot string, securityRoot string, extraArgs ...string) {
					// Pass 1 - get with no changes
					prepareMakefile(namespace, helmValues{
						valuesArgs: append([]string{
							fpuidRoot + ".floatingUserId=false",
							securityRoot + ".runAsUser=30303",
						}, extraArgs...),
					})

					container := getContainer(testManifest, "Deployment", resourceName, containerName)
					Expect(container.SecurityContext.RunAsUser).To(Equal(pointer.Int64(int64(30303))))

					// Pass 2: floatingUserId=true
					// runAsUser should not be defined
					prepareMakefile(namespace, helmValues{
						valuesArgs: append([]string{
							fpuidRoot + ".floatingUserId=true",
						}, extraArgs...),
					})

					container = getContainer(testManifest, "Deployment", resourceName, containerName)
					Expect(container.SecurityContext.RunAsUser).To(BeNil())

					// Pass 3: floatingUserId=true
					// BUT also pass in an override for the runAsUser
					prepareMakefile(namespace, helmValues{
						valuesArgs: append([]string{
							fpuidRoot + ".floatingUserId=true",
							securityRoot + ".runAsUser=20202",
							securityRoot + ".mergePolicy=helm-merge",
						}, extraArgs...),
					})

					container = getContainer(testManifest, "Deployment", resourceName, containerName)
					Expect(container.SecurityContext.RunAsUser).To(Equal(pointer.Int64(int64(20202))))

				},
					Entry("1-gloo-deployment-envoy-sidecar", "gloo", "envoy-sidecar", "gloo.deployment", "global.glooMtls.envoy.securityContext", "global.glooMtls.enabled=true"),
					Entry("1-gloo-deployment-sds", "gloo", "sds", "gloo.deployment", "global.glooMtls.sds.securityContext", "global.glooMtls.enabled=true"),
					Entry("7-gateway-proxy-deployment-gateway-proxy", "gateway-proxy", "gateway-proxy", "gatewayProxies.gatewayProxy.podTemplate", "gatewayProxies.gatewayProxy.podTemplate.glooContainerSecurityContext"),
				)

				DescribeTable("floatingUserId is properly applied (pod)", func(resourceName string, fpuidRoot string, securityRoot string, extraArgs ...string) {
					// Pass 1: floatingUserId=false
					// Also set the runAsUser to something other than the default
					prepareMakefile(namespace, helmValues{
						valuesArgs: append([]string{
							fpuidRoot + ".floatingUserId=false",
							securityRoot + ".runAsUser=30303",
						}, extraArgs...),
					})

					// The chart uses the passed user id
					structuredDeployment := getStructuredDeployment(testManifest, resourceName)
					Expect(structuredDeployment.Spec.Template.Spec.SecurityContext.RunAsUser).To(Equal(pointer.Int64(int64(30303))))

					// Pass 2: floatingUserId=true
					// The chart uses the default user id
					prepareMakefile(namespace, helmValues{
						valuesArgs: append([]string{
							fpuidRoot + ".floatingUserId=true",
						}, extraArgs...),
					})

					structuredDeployment = getStructuredDeployment(testManifest, resourceName)
					Expect(structuredDeployment.Spec.Template.Spec.SecurityContext.RunAsUser).To(BeNil())

					// // Pass 3: floatingUserId=true
					// // BUT also pass in an override for the runAsUser
					prepareMakefile(namespace, helmValues{
						valuesArgs: append([]string{
							fpuidRoot + ".floatingUserId=true",
							securityRoot + ".runAsUser=20202",
							securityRoot + ".mergePolicy=helm-merge",
						}, extraArgs...),
					})

					structuredDeployment = getStructuredDeployment(testManifest, resourceName)
					Expect(structuredDeployment.Spec.Template.Spec.SecurityContext.RunAsUser).To(Equal(pointer.Int64(int64(20202))))
				},
					Entry("7-gateway-proxy-deployment", "gateway-proxy", "gatewayProxies.gatewayProxy.podTemplate", "gatewayProxies.gatewayProxy.podTemplate.podSecurityContext"),
				)

				It("should error on an invalid mergePolicy", func() {
					values := helmValues{
						valuesArgs: []string{
							"gatewayProxies.gatewayProxy.podTemplate.podSecurityContext.mergePolicy=invalid",
						},
					}
					_, err := rendererTestCase.renderer.RenderManifest(namespace, values)
					Expect(err).To(HaveOccurred(), "should error on an invalid mergePolicy")
				})

			})

			Context("Kube resource overrides", func() {
				DescribeTable("overrides Yaml in generated resources", func(overrideProperty string, extraArgs ...string) {
					// Override property should be the path to `kubeResourceOverride`, like gloo.deployment.kubeResourceOverride
					valueArg := fmt.Sprintf("%s.metadata.labels.overriddenLabel=label", overrideProperty)
					prepareMakefile(namespace, helmValues{
						valuesArgs: append(extraArgs, valueArg),
					})
					// We are overriding the generated yaml by adding our own label to the metadata
					resources := testManifest.SelectResources(func(resource *unstructured.Unstructured) bool {
						return resource.GetLabels()["overriddenLabel"] == "label" && resource.GetKind() != ""
					})
					Expect(resources.NumResources()).To(Equal(1))
				},
					Entry("1-gloo-deployment", "gloo.deployment.kubeResourceOverride"),
					Entry("2-gloo-service", "gloo.service.kubeResourceOverride"),
					Entry("2-gloo-service-account", "gloo.serviceAccount.kubeResourceOverride"),
					Entry("3-discovery-deployment", "discovery.deployment.kubeResourceOverride"),
					Entry("3-discovery-service-account", "discovery.serviceAccount.kubeResourceOverride"),
					Entry("5-gateway-validation-webhook-configuration", "gateway.validation.webhook.kubeResourceOverride"),
					Entry("6-access-logger-deployment", "accessLogger.deployment.kubeResourceOverride", "accessLogger.enabled=true"),
					Entry("6-access-logger-service", "accessLogger.service.kubeResourceOverride", "accessLogger.enabled=true"),
					Entry("6.5-gateway-certgen-job", "gateway.certGenJob.kubeResourceOverride"),
					Entry("6.5-gateway-certgen-cronjob", "gateway.certGenJob.cron.validationWebhookKubeResourceOverride", "gateway.enabled=true", "gateway.validation.enabled=true", "gateway.validation.webhook.enabled=true", "gateway.certGenJob.cron.enabled=true"),
					Entry("8-gateway-proxy-service-account", "gateway.proxyServiceAccount.kubeResourceOverride"),
					Entry("10-ingress-deployment", "ingress.deployment.kubeResourceOverride", "ingress.enabled=true"),
					Entry("11-ingress-proxy-deployment", "ingressProxy.deployment.kubeResourceOverride", "ingress.enabled=true"),
					Entry("12-ingress-proxy-configmap", "ingressProxy.configMap.kubeResourceOverride", "ingress.enabled=true"),
					Entry("13-ingress-proxy-service", "ingressProxy.service.kubeResourceOverride", "ingress.enabled=true"),
					Entry("14-clusteringress-proxy-deployment", "settings.integrations.knative.proxy.deployment.kubeResourceOverride", "settings.integrations.knative.version=0.1.0", "settings.integrations.knative.enabled=true"),
					Entry("15-clusteringress-proxy-configmap", "settings.integrations.knative.proxy.configMap.kubeResourceOverride", "settings.integrations.knative.version=0.1.0", "settings.integrations.knative.enabled=true"),
					Entry("16-clusteringress-proxy-service", "settings.integrations.knative.proxy.service.kubeResourceOverride", "settings.integrations.knative.version=0.1.0", "settings.integrations.knative.enabled=true"),
					Entry("18-settings", "settings.kubeResourceOverride"),
					Entry("19-gloo-mtls-certgen-job", "gateway.certGenJob.mtlsKubeResourceOverride", "global.glooMtls.enabled=true"),
					Entry("19-gloo-mtls-certgen-cronjob", "gateway.certGenJob.cron.mtlsKubeResourceOverride", "global.glooMtls.enabled=true", "gateway.certGenJob.cron.enabled=true"),
					Entry("26-knative-external-proxy-deployment", "settings.integrations.knative.proxy.deployment.kubeResourceOverride", "settings.integrations.knative.version=0.8.0", "settings.integrations.knative.enabled=true"),
					Entry("27-knative-external-proxy-configmap", "settings.integrations.knative.proxy.configMap.kubeResourceOverride", "settings.integrations.knative.version=0.8.0", "settings.integrations.knative.enabled=true"),
					Entry("28-knative-external-proxy-service", "settings.integrations.knative.proxy.service.kubeResourceOverride", "settings.integrations.knative.version=0.8.0", "settings.integrations.knative.enabled=true"),
					Entry("29-knative-internal-proxy-deployment", "settings.integrations.knative.proxy.internal.deployment.kubeResourceOverride", "settings.integrations.knative.version=0.8.0", "settings.integrations.knative.enabled=true"),
					Entry("30-knative-internal-proxy-configmap", "settings.integrations.knative.proxy.internal.configMap.kubeResourceOverride", "settings.integrations.knative.version=0.8.0", "settings.integrations.knative.enabled=true"),
					Entry("31-knative-internal-proxy-service", "settings.integrations.knative.proxy.internal.service.kubeResourceOverride", "settings.integrations.knative.version=0.8.0", "settings.integrations.knative.enabled=true"),
					// todo: implement overrides for these if need arises
					// A named helm template will have to be created for each of the resources
					// generated in the following files.
					// Entry("19-gloo-mtls-configmap", ),
					// Entry("20-namespace-clusterrole-gateway", ""),
					// Entry("21-namespace-clusterrole-ingress", ""),
					// Entry("22-namespace-clusterrole-knative", ""),
					// Entry("23-namespace-clusterrolebinding-gateway", ""),
					// Entry("24-namespace-clusterrolebinding-ingress", ""),
					// Entry("25-namespace-clusterrolebinding-knative", ""),
				)

				DescribeTable("overrides Yaml in resources for each gateway proxy", func(proxyOverrideProperty string, argsPerProxy []string) {
					// Override property should be the path to `kubeResourceOverride`, like gloo.deployment.kubeResourceOverride
					proxies := []string{"gatewayProxy", "anotherProxy", "proxyThree"}
					var args []string
					var extraArgs []string
					for _, proxy := range proxies {
						args = append(args, fmt.Sprintf("gatewayProxies.%s.%s.metadata.labels.overriddenLabel=label", proxy, proxyOverrideProperty))
						for _, arg := range argsPerProxy {
							args = append(args, fmt.Sprintf("gatewayProxies.%s.%s", proxy, arg))
						}
					}

					prepareMakefile(namespace, helmValues{
						valuesArgs: append(args, extraArgs...),
					})

					// We are overriding the generated yaml by adding our own label to the metadata
					resources := testManifest.SelectResources(func(resource *unstructured.Unstructured) bool {
						return resource.GetLabels()["overriddenLabel"] == "label" && resource.GetKind() != ""
					})
					countFromResources := resources.NumResources()

					// gloo custom resources are stored as yaml in a configmap so they don't appear in the resources count.
					configMap := getConfigMap(testManifest, namespace, customResourceConfigMapName)
					Expect(configMap.Data).ToNot(BeNil())
					Expect(configMap.Data["custom-resources"]).NotTo(BeEmpty())
					countFromConfigMap := strings.Count(configMap.Data["custom-resources"], "overriddenLabel: label")

					Expect(countFromResources + countFromConfigMap).To(Equal(len(proxies)))
				},
					Entry("7-gateway-proxy-deployment", "kubeResourceOverride", nil),
					Entry("8-default-gateways httpGateway", "gatewaySettings.httpGatewayKubeOverride", nil),
					Entry("8-default-gateways httpsGateway", "gatewaySettings.httpsGatewayKubeOverride", nil),
					Entry("8-default-gateways failoverGateway", "failover.kubeResourceOverride", []string{"failover.enabled=true"}),
					Entry("8-gateway-proxy-horizontal-pod-autoscaler", "horizontalPodAutoscaler.kubeResourceOverride", []string{"kind.deployment.replicas=2", "horizontalPodAutoscaler.apiVersion=v2"}),
					Entry("8-gateway-proxy-pod-disruption-budget", "podDisruptionBudget.kubeResourceOverride", []string{"kind.deployment.replicas=2"}),
					Entry("8-gateway-proxy-service service", "service.kubeResourceOverride", nil),
					Entry("8-gateway-proxy-service config-dump-service", "service.configDumpService.kubeResourceOverride", []string{"readConfig=true", "readConfigMulticluster=true"}),
					Entry("9-gateway-proxy-configmap", "configMap.kubeResourceOverride", nil),
				)
			})

			Context("Kube GatewayKube overrides", func() {
				It("can override values on default Gateways", func() {
					prepareMakefile(namespace, helmValues{
						valuesArgs: []string{
							"gatewayProxies.gatewayProxy.gatewaySettings.httpGatewayKubeOverride.spec.bindPort=1234",
							"gatewayProxies.gatewayProxy.gatewaySettings.httpsGatewayKubeOverride.spec.ssl=false",
							"gatewayProxies.gatewayProxy.failover.enabled=true",
							"gatewayProxies.gatewayProxy.failover.kubeResourceOverride.spec.bindPort=5678",
						},
					})
					// expected gateways
					gw := makeUnstructuredGateway(namespace, defaults.GatewayProxyName, false)
					unstructured.SetNestedField(gw.Object,
						int64(1234),
						"spec", "bindPort")
					gwSsl := makeUnstructuredGateway(namespace, defaults.GatewayProxyName, true)
					unstructured.SetNestedField(gwSsl.Object,
						false,
						"spec", "ssl")
					gwFailover := makeUnstructuredFailoverGateway(namespace, defaults.GatewayProxyName)
					unstructured.SetNestedField(gwFailover.Object,
						int64(5678),
						"spec", "bindPort")

					assertCustomResourceManifest(map[string]types.GomegaMatcher{
						defaults.GatewayProxyName:                         BeEquivalentTo(gw),
						getSslGatewayName(defaults.GatewayProxyName):      BeEquivalentTo(gwSsl),
						getFailoverGatewayName(defaults.GatewayProxyName): BeEquivalentTo(gwFailover),
					})
				})

				It("can override values on custom Gateways", func() {
					prepareMakefile(namespace, helmValues{
						valuesArgs: []string{
							"gatewayProxies.gatewayProxy.disabled=true",
							"gatewayProxies.anotherProxy.gatewaySettings.httpGatewayKubeOverride.spec.bindAddress=something",
							"gatewayProxies.anotherProxy.gatewaySettings.httpsGatewayKubeOverride.spec.proxyNames[0]=new-proxy",
							"gatewayProxies.anotherProxy.failover.enabled=true",
							"gatewayProxies.anotherProxy.failover.kubeResourceOverride.spec.bindPort=5678",
						},
					})
					// expected gateways
					anotherGw := makeUnstructuredGateway(namespace, "another-proxy", false)
					unstructured.SetNestedField(anotherGw.Object,
						"something",
						"spec", "bindAddress")
					anotherGwSsl := makeUnstructuredGateway(namespace, "another-proxy", true)
					unstructured.SetNestedStringSlice(anotherGwSsl.Object,
						[]string{"new-proxy"},
						"spec", "proxyNames")
					anotherGwFailover := makeUnstructuredFailoverGateway(namespace, "another-proxy")
					unstructured.SetNestedField(anotherGwFailover.Object,
						int64(5678),
						"spec", "bindPort")

					assertCustomResourceManifest(map[string]types.GomegaMatcher{
						"another-proxy":                         BeEquivalentTo(anotherGw),
						getSslGatewayName("another-proxy"):      BeEquivalentTo(anotherGwSsl),
						getFailoverGatewayName("another-proxy"): BeEquivalentTo(anotherGwFailover),
					})
				})
			})

			It("test devMode", func() {
				prepareMakefile(namespace, helmValues{
					valuesArgs: append([]string{"settings.devMode=true"}),
				})

				resources := testManifest.SelectResources(func(u *unstructured.Unstructured) bool {
					// Check the gloo deployment container
					if u.GetKind() == "Deployment" && u.GetName() == "gloo" {
						deploymentObject, err := kuberesource.ConvertUnstructured(u)
						ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Failed to render manifest")
						structuredDeployment, ok := deploymentObject.(*appsv1.Deployment)
						Expect(ok).To(BeTrue(), fmt.Sprintf("Deployment %+v should be able to cast to a structured deployment", u))

						containers := structuredDeployment.Spec.Template.Spec.Containers
						// a := getFieldFromUnstructured(u, []string{"spec", "template", "spec", "containers"}...)
						Expect(containers).ToNot(BeNil())
						Expect(containers).To(HaveLen(1), "should have exactly 1 container")
						ports := containers[0].Ports

						foundDevModePort := false
						for _, port := range ports {
							if port.Name == "dev-admin" && port.ContainerPort == 10010 {
								foundDevModePort = true
							}
						}

						Expect(foundDevModePort).To(Equal(true), "should have found the dev mode port")
						return foundDevModePort
					}
					// Check the Settigns
					if u.GetKind() == "Settings" && u.GetName() == "default" {
						devMode := getFieldFromUnstructured(u, []string{"spec", "devMode"}...)
						devModeBool, ok := devMode.(bool)
						Expect(ok).To(BeTrue(), "devMode should be a bool")
						Expect(devModeBool).To(BeTrue(), "should have found the dev mode port")
						if devMode == true {
							return true
						}
					}

					return false
				})

				Expect(resources.NumResources()).To(Equal(2))
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

		// Lines ending with whitespace causes malformatted config map (https://github.com/solo-io/gloo/issues/4645)
		It("should not contain trailing whitespace", func() {
			out, err := exec.Command("helm", "template", "../helm/gloo").Output()
			Expect(err).NotTo(HaveOccurred(), "Helm template Generation error")

			lines := strings.Split(string(out), "\n")
			// more descriptive fail message that prints out the manifest that includes the trailing whitespace
			manifestStartingLine := 0
			skip := false
			for idx, line := range lines {
				if strings.Contains(line, "---") {
					manifestStartingLine = idx
					continue
				}
				// skip all the content within the custom resource configmap since there is extra whitespace
				// that can't be removed
				if strings.Contains(line, "custom-resources: |") {
					skip = true
					continue
				}
				if strings.Contains(line, "has-custom-resources:") {
					skip = false
				}
				if !skip && strings.TrimRightFunc(line, unicode.IsSpace) != line {
					Fail(strings.Join(lines[manifestStartingLine:idx+1], "\n") + "\n last line has whitespace")
				}
			}
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

// gets value of field nested within an Unstructured struct.
// fieldPath is the path to the value, so the value foo.bar.baz would be passed in as "foo", "bar, "baz"
func getFieldFromUnstructured(uns *unstructured.Unstructured, fieldPath ...string) interface{} {
	if len(fieldPath) < 1 {
		return nil
	}
	obj := uns.Object[fieldPath[0]]
	for _, field := range fieldPath[1:] {
		obj = obj.(map[string]interface{})[field]
	}
	return obj
}

func getJob(testManifest TestManifest, jobNamespace string, jobName string) *jobsv1.Job {
	jobUns := testManifest.ExpectCustomResource("Job", jobNamespace, jobName)
	jobObj, err := kuberesource.ConvertUnstructured(jobUns)
	Expect(err).NotTo(HaveOccurred())
	Expect(jobObj).To(BeAssignableToTypeOf(&jobsv1.Job{}))
	return jobObj.(*jobsv1.Job)
}

func getConfigMap(testManifest TestManifest, namespace string, name string) *v1.ConfigMap {
	configMapUns := testManifest.ExpectCustomResource("ConfigMap", namespace, name)
	configMapObj, err := kuberesource.ConvertUnstructured(configMapUns)
	Expect(err).NotTo(HaveOccurred())
	Expect(configMapObj).To(BeAssignableToTypeOf(&v1.ConfigMap{}))
	return configMapObj.(*v1.ConfigMap)
}

// deepCopy deepcopies a to b using json marshaling
// https://stackoverflow.com/questions/46790190/quicker-way-to-deepCopy-objects-in-golang-json-vs-gob
func deepCopy(a, b interface{}) {
	byt, _ := json.Marshal(a)
	json.Unmarshal(byt, b)
}

/*
podSecurityContextFieldsStripeGroupA/B are used to generate the values.yaml for the podSecurityContext tests.

	We use the stripe groups so that with two tests we can cover all the fields in the podSecurityContext struct both with and without
	overriding the default values. We do this in 2 places, to test the merge and the overwrite, so we'll just define them once here
*/
func podSecurityContextFieldsStripeGroupA(securityRoot string, extraArgs ...string) helmValues {
	return helmValues{
		valuesArgs: append([]string{
			securityRoot + ".fsGroup=101010",
			securityRoot + ".fsGroupChangePolicy=fsGroupChangePolicyValue",
			securityRoot + ".runAsGroup=202020",
			securityRoot + ".runAsNonRoot=true",
			securityRoot + ".runAsUser=303030",
			securityRoot + ".supplementalGroups={11,22,33}",
			securityRoot + ".seLinuxOptions.level=seLevel",
			securityRoot + ".seLinuxOptions.role=seRole",
			securityRoot + ".seLinuxOptions.type=seType",
			securityRoot + ".seLinuxOptions.user=seUser",
		}, extraArgs...),
	}
}

func podSecurityContextFieldsStripeGroupB(securityRoot string, extraArgs ...string) helmValues {
	return helmValues{
		valuesArgs: append([]string{
			securityRoot + ".seccompProfile.localhostProfile=seccompLHP",
			securityRoot + ".seccompProfile.type=seccompType",
			securityRoot + ".windowsOptions.gmsaCredentialSpec=winGmsaCredSpec",
			securityRoot + ".windowsOptions.gmsaCredentialSpecName=winGmsaCredSpecName",
			securityRoot + ".windowsOptions.hostProcess=true",
			securityRoot + ".windowsOptions.runAsUserName=winUser",
			securityRoot + ".sysctls[0].name=sysctlName",
			securityRoot + ".sysctls[0].value=sysctlValue",
		}, extraArgs...),
	}
}

func securityContextFieldsStripeGroupA(securityRoot string, extraArgs ...string) helmValues {
	return helmValues{
		valuesArgs: append([]string{
			securityRoot + ".runAsNonRoot=true",
			securityRoot + ".runAsUser=1234",
			securityRoot + ".allowPrivilegeEscalation=true",
			securityRoot + ".readOnlyRootFilesystem=true",
			securityRoot + ".seLinuxOptions.level=seLevel",
			securityRoot + ".seLinuxOptions.role=seRole",
			securityRoot + ".seLinuxOptions.type=seType",
			securityRoot + ".seLinuxOptions.user=seUser",
		}, extraArgs...),
	}
}

func securityContextFieldsStripeGroupB(securityRoot string, extraArgs ...string) helmValues {
	return helmValues{
		valuesArgs: append([]string{
			securityRoot + ".capabilities.add={ADD}",
			securityRoot + ".capabilities.drop={DROP}",
			securityRoot + ".seccompProfile.localhostProfile=seccompLHP",
			securityRoot + ".seccompProfile.type=seccompType",
			securityRoot + ".windowsOptions.gmsaCredentialSpec=winGmsaCredSpec",
			securityRoot + ".windowsOptions.gmsaCredentialSpecName=winGmsaCredSpecName",
			securityRoot + ".windowsOptions.hostProcess=true",
			securityRoot + ".windowsOptions.runAsUserName=winUser",
		}, extraArgs...),
	}
}

func getContainer(t TestManifest, kind string, resourceName string, containerName string) *v1.Container {
	resources := t.SelectResources(func(u *unstructured.Unstructured) bool {
		if u.GetKind() == kind && u.GetName() == resourceName {
			return true
		}
		return false
	})
	Expect(resources.NumResources()).To(Equal(1))
	var foundContainer v1.Container
	resources.ExpectAll(func(deployment *unstructured.Unstructured) {
		foundExpected := false
		deploymentObject, err := kuberesource.ConvertUnstructured(deployment)
		ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Failed to render manifest")
		structuredDeployment, ok := deploymentObject.(*appsv1.Deployment)
		Expect(ok).To(BeTrue(), fmt.Sprintf("Deployment %+v should be able to cast to a structured deployment", deployment))

		for _, container := range structuredDeployment.Spec.Template.Spec.Containers {
			if container.Name == containerName {
				foundExpected = true
				foundContainer = container
			}
		}

		Expect(foundExpected).To(Equal(true))
	})

	return &foundContainer
}

func getStructuredDeployment(t TestManifest, resourceName string) *appsv1.Deployment {

	structuredDeployment := &appsv1.Deployment{}

	resources := t.SelectResources(func(u *unstructured.Unstructured) bool {
		if u.GetKind() == "Deployment" && u.GetName() == resourceName {
			deploymentObject, err := kuberesource.ConvertUnstructured(u)
			ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Failed to render manifest")
			var ok bool
			structuredDeployment, ok = deploymentObject.(*appsv1.Deployment)
			Expect(ok).To(BeTrue(), fmt.Sprintf("Deployment %+v should be able to cast to a structured deployment", u))
			return true
		}
		return false
	})
	Expect(resources.NumResources()).To(Equal(1))

	return structuredDeployment
}

func makeUnstructuredGateway(namespace string, name string, ssl bool) *unstructured.Unstructured {
	port := "8080"
	gwName := name
	if ssl {
		port = "8443"
		gwName = getSslGatewayName(name)
	}

	return makeUnstructured(`
apiVersion: gateway.solo.io/v1
kind: Gateway
metadata:
  labels:
    app: gloo
  name: ` + gwName + `
  namespace: ` + namespace + `
spec:
  bindAddress: '::'
  bindPort: ` + port + `
  httpGateway: {}
  proxyNames:
  - ` + name + `
  ssl: ` + strconv.FormatBool(ssl) + `
  useProxyProto: false
`)
}

func makeUnstructuredFailoverGateway(namespace string, proxyName string) *unstructured.Unstructured {
	return makeUnstructured(`
apiVersion: gateway.solo.io/v1
kind: Gateway
metadata:
  labels:
    app: gloo
  name: ` + getFailoverGatewayName(proxyName) + `
  namespace: ` + namespace + `
spec:
  bindAddress: '::'
  bindPort: 15443
  proxyNames:
  - ` + proxyName + `
  tcpGateway:
    tcpHosts:
    - destination:
        forwardSniClusterName: {}
      name: failover
      sslConfig:
        secretRef:
          name: failover-downstream
          namespace: ` + namespace + `
`)
}

func getSslGatewayName(name string) string {
	return name + "-ssl"
}

func getFailoverGatewayName(name string) string {
	return name + "-failover"
}
