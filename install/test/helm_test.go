package test

import (
	"fmt"
	"os"

	"k8s.io/utils/pointer"

	. "github.com/onsi/ginkgo"
	"github.com/solo-io/gloo/projects/gateway/pkg/translator"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	. "github.com/solo-io/go-utils/manifesttestutils"
)

var _ = Describe("Helm Test", func() {

	Describe("gateway proxy extra annotations and crds", func() {
		var (
			labels        map[string]string
			selector      map[string]string
			getPullPolicy func() v1.PullPolicy
		)

		BeforeEach(func() {
			version = os.Getenv("TAGGED_VERSION")
			if version == "" {
				version = "dev"
				getPullPolicy = func() v1.PullPolicy { return v1.PullAlways }
			} else {
				version = version[1:]
				getPullPolicy = func() v1.PullPolicy { return v1.PullIfNotPresent }
			}
		})

		prepareMakefile := func(helmFlags string) {
			makefileSerializer.Lock()
			defer makefileSerializer.Unlock()
			MustMake(".", "-C", "../..", "install/gloo-gateway.yaml", "HELMFLAGS="+helmFlags)
			testManifest = NewTestManifest("../gloo-gateway.yaml")
		}

		Context("gateway", func() {
			BeforeEach(func() {
				labels = map[string]string{
					"gloo": translator.GatewayProxyName,
					"app":  "gloo",
				}
				selector = map[string]string{
					"gateway-proxy": "live",
				}
			})

			It("has a namespace", func() {
				helmFlags := "--namespace " + namespace + " --set namespace.create=true  --set gatewayProxies.gatewayProxyV2.service.extraAnnotations.test=test"
				prepareMakefile(helmFlags)
				rb := ResourceBuilder{
					Namespace: namespace,
					Name:      translator.GatewayProxyName,
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

			Context("gateway-proxy deployment", func() {
				var (
					gatewayProxyDeployment *appsv1.Deployment
				)

				BeforeEach(func() {
					selector = map[string]string{
						"gloo": "gateway-proxy-v2",
					}
					podLabels := map[string]string{
						"gloo":          "gateway-proxy-v2",
						"gateway-proxy": "live",
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
					container.Name = "gateway-proxy-v2"
					container.Args = []string{"--disable-hot-restart"}

					rb := ResourceBuilder{
						Namespace:  namespace,
						Name:       "gateway-proxy-v2",
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
									Name: "gateway-proxy-v2-envoy-config",
								},
							},
						},
					}}
					deploy.Spec.Template.Spec.Containers[0].ImagePullPolicy = getPullPolicy()
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
					deploy.Spec.Template.Spec.Containers[0].SecurityContext = &v1.SecurityContext{
						Capabilities: &v1.Capabilities{
							Add:  []v1.Capability{"NET_BIND_SERVICE"},
							Drop: []v1.Capability{"ALL"},
						},
						ReadOnlyRootFilesystem:   &truez,
						AllowPrivilegeEscalation: &falsez,
					}

					gatewayProxyDeployment = deploy
				})

				It("creates a deployment", func() {
					helmFlags := "--namespace " + namespace + " --set namespace.create=true"
					prepareMakefile(helmFlags)
					testManifest.ExpectDeploymentAppsV1(gatewayProxyDeployment)
				})

				It("enables probes", func() {
					helmFlags := "--namespace " + namespace + " --set namespace.create=true --set gatewayProxies.gatewayProxyV2.podTemplate.probes=true"

					gatewayProxyDeployment.Spec.Template.Spec.Containers[0].ReadinessProbe = &v1.Probe{
						Handler: v1.Handler{
							Exec: &v1.ExecAction{
								Command: []string{
									"wget", "-O", "/dev/null", "localhost:19000/ready",
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
									"wget", "-O", "/dev/null", "localhost:19000/server_info",
								},
							},
						},
						InitialDelaySeconds: 1,
						PeriodSeconds:       10,
						FailureThreshold:    10,
					}
					prepareMakefile(helmFlags)
					testManifest.ExpectDeploymentAppsV1(gatewayProxyDeployment)
				})

				It("has limits", func() {
					helmFlags := "--namespace " + namespace + " --set namespace.create=true --set gatewayProxies.gatewayProxyV2.podTemplate.resources.limits.memory=2  --set gatewayProxies.gatewayProxyV2.podTemplate.resources.limits.cpu=3 --set gatewayProxies.gatewayProxyV2.podTemplate.resources.requests.memory=4  --set gatewayProxies.gatewayProxyV2.podTemplate.resources.requests.cpu=5"
					prepareMakefile(helmFlags)

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
					helmFlags := "--namespace " + namespace + " --set namespace.create=true --set gatewayProxies.gatewayProxyV2.podTemplate.image.pullPolicy=Always --set gatewayProxies.gatewayProxyV2.podTemplate.image.registry=gcr.io/solo-public"
					prepareMakefile(helmFlags)

					testManifest.ExpectDeploymentAppsV1(gatewayProxyDeployment)
				})
			})
		})
		Context("control plane deployments", func() {
			updateDeployment := func(deploy *appsv1.Deployment) {
				deploy.Spec.Selector = &metav1.LabelSelector{
					MatchLabels: selector,
				}
				deploy.Spec.Template.ObjectMeta.Labels = selector

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
				deploy.Spec.Template.Spec.Containers[0].ImagePullPolicy = getPullPolicy()
			}
			Context("gloo deployment", func() {
				var (
					glooDeployment *appsv1.Deployment
				)
				BeforeEach(func() {
					labels = map[string]string{
						"gloo": "gloo",
						"app":  "gloo",
					}
					selector = map[string]string{
						"gloo": "gloo",
					}
					container := GetQuayContainerSpec("gloo", version, GetPodNamespaceEnvVar())

					rb := ResourceBuilder{
						Namespace:  namespace,
						Name:       "gloo",
						Labels:     labels,
						Containers: []ContainerSpec{container},
					}
					deploy := rb.GetDeploymentAppsv1()
					updateDeployment(deploy)
					deploy.Spec.Template.Spec.Containers[0].Ports = []v1.ContainerPort{
						{Name: "grpc", ContainerPort: 9977, Protocol: "TCP"},
					}

					deploy.Spec.Template.Spec.Containers[0].Resources = v1.ResourceRequirements{
						Requests: v1.ResourceList{
							v1.ResourceMemory: resource.MustParse("256Mi"),
							v1.ResourceCPU:    resource.MustParse("500m"),
						},
					}
					glooDeployment = deploy
				})

				It("has a creates a deployment", func() {
					helmFlags := "--namespace " + namespace + " --set namespace.create=true"
					prepareMakefile(helmFlags)
					testManifest.ExpectDeploymentAppsV1(glooDeployment)
				})

				It("has limits", func() {
					helmFlags := "--namespace " + namespace + " --set namespace.create=true --set gloo.deployment.resources.limits.memory=2  --set gloo.deployment.resources.limits.cpu=3 --set gloo.deployment.resources.requests.memory=4  --set gloo.deployment.resources.requests.cpu=5"
					prepareMakefile(helmFlags)

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
					container := GetContainerSpec("gcr.io/solo-public", "gloo", version, GetPodNamespaceEnvVar())
					container.PullPolicy = "Always"
					rb := ResourceBuilder{
						Namespace:  namespace,
						Name:       "gloo",
						Labels:     labels,
						Containers: []ContainerSpec{container},
					}
					deploy := rb.GetDeploymentAppsv1()
					updateDeployment(deploy)
					deploy.Spec.Template.Spec.Containers[0].Ports = []v1.ContainerPort{
						{Name: "grpc", ContainerPort: 9977, Protocol: "TCP"},
					}

					glooDeployment = deploy
					helmFlags := "--namespace " + namespace + " --set namespace.create=true --set gloo.deployment.image.pullPolicy=Always --set gloo.deployment.image.registry=gcr.io/solo-public"
					prepareMakefile(helmFlags)

				})
			})

			Context("gateway deployment", func() {
				var (
					gatewayDeployment *appsv1.Deployment
				)
				BeforeEach(func() {
					labels = map[string]string{
						"gloo": "gateway",
						"app":  "gloo",
					}
					selector = map[string]string{
						"gloo": "gateway",
					}
					container := GetQuayContainerSpec("gateway", version, GetPodNamespaceEnvVar())

					rb := ResourceBuilder{
						Namespace:  namespace,
						Name:       "gateway-v2",
						Labels:     labels,
						Containers: []ContainerSpec{container},
					}
					deploy := rb.GetDeploymentAppsv1()
					updateDeployment(deploy)
					gatewayDeployment = deploy
				})

				It("has a creates a deployment", func() {
					helmFlags := "--namespace " + namespace + " --set namespace.create=true"
					prepareMakefile(helmFlags)
					testManifest.ExpectDeploymentAppsV1(gatewayDeployment)
				})

				It("disables probes", func() {
					helmFlags := "--namespace " + namespace + " --set namespace.create=true --set gateway.deployment.probes=false"
					prepareMakefile(helmFlags)
					gatewayDeployment.Spec.Template.Spec.Containers[0].ReadinessProbe = nil
					gatewayDeployment.Spec.Template.Spec.Containers[0].LivenessProbe = nil
					testManifest.ExpectDeploymentAppsV1(gatewayDeployment)
				})

				It("has limits", func() {
					helmFlags := "--namespace " + namespace + " --set namespace.create=true --set gateway.deployment.resources.limits.memory=2  --set gateway.deployment.resources.limits.cpu=3 --set gateway.deployment.resources.requests.memory=4  --set gateway.deployment.resources.requests.cpu=5"
					prepareMakefile(helmFlags)

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
					container := GetContainerSpec("gcr.io/solo-public", "gateway", version, GetPodNamespaceEnvVar())
					container.PullPolicy = "Always"
					rb := ResourceBuilder{
						Namespace:  namespace,
						Name:       "gateway",
						Labels:     labels,
						Containers: []ContainerSpec{container},
					}
					deploy := rb.GetDeploymentAppsv1()
					updateDeployment(deploy)

					gatewayDeployment = deploy
					helmFlags := "--namespace " + namespace + " --set namespace.create=true --set gateway.deployment.image.pullPolicy=Always --set gateway.deployment.image.registry=gcr.io/solo-public"
					prepareMakefile(helmFlags)

				})
			})

			Context("discovery deployment", func() {
				var (
					discoveryDeployment *appsv1.Deployment
				)
				BeforeEach(func() {
					labels = map[string]string{
						"gloo": "discovery",
						"app":  "gloo",
					}
					selector = map[string]string{
						"gloo": "discovery",
					}
					container := GetQuayContainerSpec("discovery", version, GetPodNamespaceEnvVar())

					rb := ResourceBuilder{
						Namespace:  namespace,
						Name:       "discovery",
						Labels:     labels,
						Containers: []ContainerSpec{container},
					}
					deploy := rb.GetDeploymentAppsv1()
					updateDeployment(deploy)
					discoveryDeployment = deploy
				})

				It("has a creates a deployment", func() {
					helmFlags := "--namespace " + namespace + " --set namespace.create=true"
					prepareMakefile(helmFlags)
					testManifest.ExpectDeploymentAppsV1(discoveryDeployment)
				})

				It("disables probes", func() {
					helmFlags := "--namespace " + namespace + " --set namespace.create=true --set discovery.deployment.probes=false"
					prepareMakefile(helmFlags)
					discoveryDeployment.Spec.Template.Spec.Containers[0].ReadinessProbe = nil
					discoveryDeployment.Spec.Template.Spec.Containers[0].LivenessProbe = nil
					testManifest.ExpectDeploymentAppsV1(discoveryDeployment)
				})

				It("has limits", func() {
					helmFlags := "--namespace " + namespace + " --set namespace.create=true --set discovery.deployment.resources.limits.memory=2  --set discovery.deployment.resources.limits.cpu=3 --set discovery.deployment.resources.requests.memory=4  --set discovery.deployment.resources.requests.cpu=5"
					prepareMakefile(helmFlags)

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
					container := GetContainerSpec("gcr.io/solo-public", "discovery", version, GetPodNamespaceEnvVar())
					container.PullPolicy = "Always"
					rb := ResourceBuilder{
						Namespace:  namespace,
						Name:       "discovery",
						Labels:     labels,
						Containers: []ContainerSpec{container},
					}
					deploy := rb.GetDeploymentAppsv1()
					updateDeployment(deploy)

					discoveryDeployment = deploy
					helmFlags := "--namespace " + namespace + " --set namespace.create=true --set discovery.deployment.image.pullPolicy=Always --set discovery.deployment.image.registry=gcr.io/solo-public"
					prepareMakefile(helmFlags)

				})
			})

		})

		Describe("gateway proxy - tracing config", func() {
			var (
				glooConfigMapName = "gateway-proxy-v2-envoy-config"
			)

			labels := map[string]string{
				"gloo": translator.GatewayProxyName,
				"app":  "gloo",
			}

			// helper for passing a values file
			prepareMakefileFromValuesFile := func(valuesFile string) {
				helmFlags := "--namespace " + namespace +
					" --set namespace.create=true" +
					" --set gatewayProxies.gatewayProxyV2.service.extraAnnotations.test=test" +
					" --values " + valuesFile
				prepareMakefile(helmFlags)
			}

			It("has a proxy without tracing", func() {
				helmFlags := "--namespace " + namespace + " --set namespace.create=true  --set gatewayProxies.gatewayProxyV2.service.extraAnnotations.test=test"
				prepareMakefile(helmFlags)
				proxySpec := make(map[string]string)
				proxySpec["envoy.yaml"] = confWithoutTracing
				cmRb := ResourceBuilder{
					Namespace: namespace,
					Name:      glooConfigMapName,
					Labels:    labels,
					Data:      proxySpec,
				}
				proxy := cmRb.GetConfigMap()
				testManifest.ExpectConfigMapWithYamlData(proxy)
			})

			It("has a proxy with tracing provider", func() {
				prepareMakefileFromValuesFile("install/test/val_tracing_provider.yaml")
				proxySpec := make(map[string]string)
				proxySpec["envoy.yaml"] = confWithTracingProvider
				cmRb := ResourceBuilder{
					Namespace: namespace,
					Name:      glooConfigMapName,
					Labels:    labels,
					Data:      proxySpec,
				}
				proxy := cmRb.GetConfigMap()
				testManifest.ExpectConfigMapWithYamlData(proxy)
			})

			It("has a proxy with tracing provider and cluster", func() {
				prepareMakefileFromValuesFile("install/test/val_tracing_provider_cluster.yaml")
				proxySpec := make(map[string]string)
				proxySpec["envoy.yaml"] = confWithTracingProviderCluster
				cmRb := ResourceBuilder{
					Namespace: namespace,
					Name:      glooConfigMapName,
					Labels:    labels,
					Data:      proxySpec,
				}
				proxy := cmRb.GetConfigMap()
				testManifest.ExpectConfigMapWithYamlData(proxy)
			})
		})

		FDescribe("merge ingress and gateway", func() {

			// helper for passing a values file
			prepareMakefileFromValuesFile := func(valuesFile string) {
				helmFlags := "--namespace " + namespace +
					" -f " + valuesFile
				prepareMakefile(helmFlags)
			}

			It("merges the config correctly", func() {
				prepareMakefileFromValuesFile("install/test/merge_ingress_values.yaml")
				cmRb := ResourceBuilder{
					Namespace: namespace,
					Name:      "gloo",
				}
				dep := cmRb.GetDeployment()
				fmt.Println(dep)
				testManifest.ExpectDeploymentAppsV1(d2)
			})

		})

	})
})

// These are large, so get them out of the way to help readability of test coverage

var d2 = &appsv1.Deployment{
	TypeMeta: metav1.TypeMeta{
		Kind:       "Deployment",
		APIVersion: "apps/v1",
	},
	ObjectMeta: metav1.ObjectMeta{
		Name:      "gloo",
		Namespace: "gloo-system",
		Labels: map[string]string{
			"app": "gloo", "gloo": "gloo"},
	},
	Spec: appsv1.DeploymentSpec{
		Replicas: pointer.Int32Ptr(1),
		Selector: &metav1.LabelSelector{MatchLabels: map[string]string{
			"gloo": "gloo"},
		},
		Template: v1.PodTemplateSpec{
			ObjectMeta: metav1.ObjectMeta{
				Labels: map[string]string{
					"gloo": "gloo"},
			},
			Spec: v1.PodSpec{
				Volumes:        nil,
				InitContainers: nil,
				Containers: []v1.Container{
					{
						Name: "gloo",
						// expecting ilackarms?
						//Image:      "gcr.io/solo-gloo-knative-e2e/gloo:test-ilackarms",
						Image: "quay.io/solo-io/gloo:dev",
						Ports: []v1.ContainerPort{
							{Name: "grpc", HostPort: 0, ContainerPort: 9977, Protocol: "TCP", HostIP: ""},
						},
						Env: []v1.EnvVar{
							{
								Name:  "POD_NAMESPACE",
								Value: "",
								ValueFrom: &v1.EnvVarSource{
									FieldRef:         &v1.ObjectFieldSelector{APIVersion: "", FieldPath: "metadata.namespace"},
									ResourceFieldRef: nil,
									ConfigMapKeyRef:  nil,
									SecretKeyRef:     nil,
								},
							},
						},
						Resources: v1.ResourceRequirements{
							Limits: nil,
							Requests: v1.ResourceList{
								v1.ResourceMemory: resource.MustParse("256Mi"), // 256M
								v1.ResourceCPU:    resource.MustParse("500m"),
							},
						},
						ImagePullPolicy: "Always",
						SecurityContext: &v1.SecurityContext{
							Capabilities:             &v1.Capabilities{Add: nil, Drop: []v1.Capability{"ALL"}},
							RunAsUser:                pointer.Int64Ptr(10101),
							RunAsNonRoot:             pointer.BoolPtr(true),
							ReadOnlyRootFilesystem:   pointer.BoolPtr(true),
							AllowPrivilegeEscalation: pointer.BoolPtr(false),
						},
					},
				},
			},
		},
	},
}

var confWithoutTracing = `
node:
  cluster: gateway
  id: "{{.PodName}}.{{.PodNamespace}}"
  metadata:
    # role's value is the key for the in-memory xds cache (projects/gloo/pkg/xds/envoy.go)
    role: "{{.PodNamespace}}~gateway-proxy-v2"
static_resources:
  listeners:
    - name: prometheus_listener
      address:
        socket_address:
          address: 0.0.0.0
          port_value: 8081
      filter_chains:
        - filters:
            - name: envoy.http_connection_manager
              config:
                codec_type: auto
                stat_prefix: prometheus
                route_config:
                  name: prometheus_route
                  virtual_hosts:
                    - name: prometheus_host
                      domains:
                        - "*"
                      routes:
                        - match:
                            path: "/ready"
                            headers:
                            - name: ":method"
                              exact_match: GET
                          route:
                            cluster: admin_port_cluster
                        - match:
                            prefix: "/metrics"
                            headers:
                            - name: ":method"
                              exact_match: GET
                          route:
                            prefix_rewrite: "/stats/prometheus"
                            cluster: admin_port_cluster
                http_filters:
                  - name: envoy.router
                    config: {} # if $spec.podTemplate.stats # if $spec.tracing


  clusters:
  - name: gloo.gloo-system.svc.cluster.local:9977
    alt_stat_name: xds_cluster
    connect_timeout: 5.000s
    load_assignment:
      cluster_name: gloo.gloo-system.svc.cluster.local:9977
      endpoints:
      - lb_endpoints:
        - endpoint:
            address:
              socket_address:
                address: gloo.gloo-system.svc.cluster.local
                port_value: 9977
    http2_protocol_options: {}
    upstream_connection_options:
      tcp_keepalive: {}
    type: STRICT_DNS
  - name: admin_port_cluster
    connect_timeout: 5.000s
    type: STATIC
    lb_policy: ROUND_ROBIN
    load_assignment:
      cluster_name: admin_port_cluster
      endpoints:
      - lb_endpoints:
        - endpoint:
            address:
              socket_address:
                address: 127.0.0.1
                port_value: 19000 # if $spec.podTemplate.stats

dynamic_resources:
  ads_config:
    api_type: GRPC
    grpc_services:
    - envoy_grpc: {cluster_name: gloo.gloo-system.svc.cluster.local:9977}
  cds_config:
    ads: {}
  lds_config:
    ads: {}
admin:
  access_log_path: /dev/null
  address:
    socket_address:
      address: 127.0.0.1
      port_value: 19000 # if (empty $spec.configMap.data) ## allows full custom # range $name, $spec := .Values.gatewayProxies# if .Values.gateway.enabled
`

var confWithTracingProvider = `
node:
  cluster: gateway
  id: "{{.PodName}}.{{.PodNamespace}}"
  metadata:
    # role's value is the key for the in-memory xds cache (projects/gloo/pkg/xds/envoy.go)
    role: "{{.PodNamespace}}~gateway-proxy-v2"
static_resources:
  listeners:
    - name: prometheus_listener
      address:
        socket_address:
          address: 0.0.0.0
          port_value: 8081
      filter_chains:
        - filters:
            - name: envoy.http_connection_manager
              config:
                codec_type: auto
                stat_prefix: prometheus
                route_config:
                  name: prometheus_route
                  virtual_hosts:
                    - name: prometheus_host
                      domains:
                        - "*"
                      routes:
                        - match:
                            path: "/ready"
                            headers:
                            - name: ":method"
                              exact_match: GET
                          route:
                            cluster: admin_port_cluster
                        - match:
                            prefix: "/metrics"
                            headers:
                            - name: ":method"
                              exact_match: GET
                          route:
                            prefix_rewrite: "/stats/prometheus"
                            cluster: admin_port_cluster
                http_filters:
                  - name: envoy.router
                    config: {} # if $spec.podTemplate.stats
  clusters:
  - name: gloo.gloo-system.svc.cluster.local:9977
    alt_stat_name: xds_cluster
    connect_timeout: 5.000s
    load_assignment:
      cluster_name: gloo.gloo-system.svc.cluster.local:9977
      endpoints:
      - lb_endpoints:
        - endpoint:
            address:
              socket_address:
                address: gloo.gloo-system.svc.cluster.local
                port_value: 9977
    http2_protocol_options: {}
    upstream_connection_options:
      tcp_keepalive: {}
    type: STRICT_DNS # if $spec.tracing.cluster # if $spec.tracing
  - name: admin_port_cluster
    connect_timeout: 5.000s
    type: STATIC
    lb_policy: ROUND_ROBIN
    load_assignment:
      cluster_name: admin_port_cluster
      endpoints:
      - lb_endpoints:
        - endpoint:
            address:
              socket_address:
                address: 127.0.0.1
                port_value: 19000 # if $spec.podTemplate.stats
tracing:
  http:
    another: line
    trace: spec
     # if $spec.tracing.provider # if $spec.tracing
dynamic_resources:
  ads_config:
    api_type: GRPC
    grpc_services:
    - envoy_grpc: {cluster_name: gloo.gloo-system.svc.cluster.local:9977}
  cds_config:
    ads: {}
  lds_config:
    ads: {}
admin:
  access_log_path: /dev/null
  address:
    socket_address:
      address: 127.0.0.1
      port_value: 19000 # if (empty $spec.configMap.data) ## allows full custom # range $name, $spec := .Values.gatewayProxies# if .Values.gateway.enabled
`

var confWithTracingProviderCluster = `
node:
  cluster: gateway
  id: "{{.PodName}}.{{.PodNamespace}}"
  metadata:
    # role's value is the key for the in-memory xds cache (projects/gloo/pkg/xds/envoy.go)
    role: "{{.PodNamespace}}~gateway-proxy-v2"
static_resources:
  listeners:
    - name: prometheus_listener
      address:
        socket_address:
          address: 0.0.0.0
          port_value: 8081
      filter_chains:
        - filters:
            - name: envoy.http_connection_manager
              config:
                codec_type: auto
                stat_prefix: prometheus
                route_config:
                  name: prometheus_route
                  virtual_hosts:
                    - name: prometheus_host
                      domains:
                        - "*"
                      routes:
                        - match:
                            path: "/ready"
                            headers:
                            - name: ":method"
                              exact_match: GET
                          route:
                            cluster: admin_port_cluster
                        - match:
                            prefix: "/metrics"
                            headers:
                            - name: ":method"
                              exact_match: GET
                          route:
                            prefix_rewrite: "/stats/prometheus"
                            cluster: admin_port_cluster
                http_filters:
                  - name: envoy.router
                    config: {} # if $spec.podTemplate.stats
  clusters:
  - name: gloo.gloo-system.svc.cluster.local:9977
    alt_stat_name: xds_cluster
    connect_timeout: 5.000s
    load_assignment:
      cluster_name: gloo.gloo-system.svc.cluster.local:9977
      endpoints:
      - lb_endpoints:
        - endpoint:
            address:
              socket_address:
                address: gloo.gloo-system.svc.cluster.local
                port_value: 9977
    http2_protocol_options: {}
    upstream_connection_options:
      tcp_keepalive: {}
    type: STRICT_DNS
  - connect_timeout: 1s
    lb_policy: round_robin
    load_assignment:
      cluster_name: zipkin
      endpoints:
      - lb_endpoints:
        - endpoint:
            address:
              socket_address:
                address: zipkin
                port_value: 1234
    name: zipkin
    type: strict_dns
   # if $spec.tracing.cluster # if $spec.tracing
  - name: admin_port_cluster
    connect_timeout: 5.000s
    type: STATIC
    lb_policy: ROUND_ROBIN
    load_assignment:
      cluster_name: admin_port_cluster
      endpoints:
      - lb_endpoints:
        - endpoint:
            address:
              socket_address:
                address: 127.0.0.1
                port_value: 19000 # if $spec.podTemplate.stats
tracing:
  http:
    typed_config:
      '@type': type.googleapis.com/envoy.config.trace.v2.ZipkinConfig
      collector_cluster: zipkin
      collector_endpoint: /api/v1/spans
     # if $spec.tracing.provider # if $spec.tracing
dynamic_resources:
  ads_config:
    api_type: GRPC
    grpc_services:
    - envoy_grpc: {cluster_name: gloo.gloo-system.svc.cluster.local:9977}
  cds_config:
    ads: {}
  lds_config:
    ads: {}
admin:
  access_log_path: /dev/null
  address:
    socket_address:
      address: 127.0.0.1
      port_value: 19000 # if (empty $spec.configMap.data) ## allows full custom # range $name, $spec := .Values.gatewayProxies# if .Values.gateway.enabled`
