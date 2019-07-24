package test

import (
	"os"

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
			labels   map[string]string
			selector map[string]string
		)

		BeforeEach(func() {
			version = os.Getenv("TAGGED_VERSION")
			if version == "" {
				version = "dev"
			} else {
				version = version[1:]
			}
			labels = map[string]string{
				"gloo": translator.GatewayProxyName,
				"app":  "gloo",
			}
			selector = map[string]string{
				"gloo": translator.GatewayProxyName,
			}
		})

		prepareMakefile := func(helmFlags string) {
			makefileSerializer.Lock()
			defer makefileSerializer.Unlock()
			MustMake(".", "-C", "../..", "install/gloo-gateway.yaml", "HELMFLAGS="+helmFlags)
			testManifest = NewTestManifest("../gloo-gateway.yaml")
		}

		It("has a namespace", func() {
			helmFlags := "--namespace " + namespace + " --set namespace.create=true  --set gatewayProxies.gatewayProxy.service.extraAnnotations.test=test"
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

		It("has a proxy without tracing", func() {
			helmFlags := "--namespace " + namespace + " --set namespace.create=true  --set gatewayProxies.gatewayProxy.service.extraAnnotations.test=test"
			prepareMakefile(helmFlags)
			proxySpec := make(map[string]string)
			proxySpec["envoy.yaml"] = `
node:
  cluster: gateway
  id: "{{.PodName}}.{{.PodNamespace}}"
  metadata:
    # role's value is the key for the in-memory xds cache (projects/gloo/pkg/xds/envoy.go)
    role: "{{.PodNamespace}}~gateway-proxy"
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
                            path: "/server_info"
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
			cmName := "gateway-proxy-envoy-config"
			cmRb := ResourceBuilder{
				Namespace: namespace,
				Name:      cmName,
				Labels:    labels,
				Data:      proxySpec,
			}
			proxy := cmRb.GetConfigMap()
			testManifest.ExpectConfigMapWithYamlData(proxy)
		})

		It("has a proxy with tracing", func() {
			helmFlags := "--namespace " + namespace + " --set namespace.create=true  --set gatewayProxies.gatewayProxy.service.extraAnnotations.test=test --values install/test/test_values.yaml"
			prepareMakefile(helmFlags)
			proxySpec := make(map[string]string)
			proxySpec["envoy.yaml"] = `
node:
  cluster: gateway
  id: "{{.PodName}}.{{.PodNamespace}}"
  metadata:
    # role's value is the key for the in-memory xds cache (projects/gloo/pkg/xds/envoy.go)
    role: "{{.PodNamespace}}~gateway-proxy"
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
                            path: "/server_info"
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
  tracing:
    trace: spec
    another: line
      # if $spec.tracing


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
			cmName := "gateway-proxy-envoy-config"
			cmRb := ResourceBuilder{
				Namespace: namespace,
				Name:      cmName,
				Labels:    labels,
				Data:      proxySpec,
			}
			proxy := cmRb.GetConfigMap()
			testManifest.ExpectConfigMapWithYamlData(proxy)
		})

		Context("gateway-proxy deployment", func() {
			var (
				gatewayProxyDeployment *appsv1.Deployment
			)
			BeforeEach(func() {
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
					Name:       translator.GatewayProxyName,
					Labels:     labels,
					Containers: []ContainerSpec{container},
				}
				deploy := rb.GetDeploymentAppsv1()
				deploy.Spec.Selector = &metav1.LabelSelector{
					MatchLabels: selector,
				}
				deploy.Spec.Template.ObjectMeta.Labels = selector
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

				readyProb := v1.Probe{
					Handler: v1.Handler{
						HTTPGet: &v1.HTTPGetAction{
							Path: "/ready",
							Port: intstr.FromInt(8081),
						},
					},
					InitialDelaySeconds: 1,
					PeriodSeconds:       10,
					FailureThreshold:    10,
				}
				liveProb := v1.Probe{
					Handler: v1.Handler{
						HTTPGet: &v1.HTTPGetAction{
							Path: "/server_info",
							Port: intstr.FromInt(8081),
						},
					},
					InitialDelaySeconds: 1,
					PeriodSeconds:       10,
					FailureThreshold:    10,
				}

				deploy.Spec.Template.Spec.Containers[0].ReadinessProbe = &readyProb
				deploy.Spec.Template.Spec.Containers[0].LivenessProbe = &liveProb

				gatewayProxyDeployment = deploy
			})

			It("has a creates a deployment", func() {
				helmFlags := "--namespace " + namespace + " --set namespace.create=true --values install/test/test_values.yaml"
				prepareMakefile(helmFlags)
				testManifest.ExpectDeploymentAppsV1(gatewayProxyDeployment)
			})

			It("has limits", func() {
				helmFlags := "--namespace " + namespace + " --set namespace.create=true --set gatewayProxies.gatewayProxy.podTemplate.resources.limits.memory=2  --set gatewayProxies.gatewayProxy.podTemplate.resources.limits.cpu=3 --set gatewayProxies.gatewayProxy.podTemplate.resources.requests.memory=4  --set gatewayProxies.gatewayProxy.podTemplate.resources.requests.cpu=5 --values install/test/test_values.yaml"
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
					helmFlags := "--namespace " + namespace + " --set namespace.create=true --values install/test/test_values.yaml"
					prepareMakefile(helmFlags)
					testManifest.ExpectDeploymentAppsV1(glooDeployment)
				})

				It("has limits", func() {
					helmFlags := "--namespace " + namespace + " --set namespace.create=true --set gloo.deployment.resources.limits.memory=2  --set gloo.deployment.resources.limits.cpu=3 --set gloo.deployment.resources.requests.memory=4  --set gloo.deployment.resources.requests.cpu=5 --values install/test/test_values.yaml"
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
						Name:       "gateway",
						Labels:     labels,
						Containers: []ContainerSpec{container},
					}
					deploy := rb.GetDeploymentAppsv1()
					updateDeployment(deploy)
					gatewayDeployment = deploy
				})

				It("has a creates a deployment", func() {
					helmFlags := "--namespace " + namespace + " --set namespace.create=true --values install/test/test_values.yaml"
					prepareMakefile(helmFlags)
					testManifest.ExpectDeploymentAppsV1(gatewayDeployment)
				})

				It("has limits", func() {
					helmFlags := "--namespace " + namespace + " --set namespace.create=true --set gateway.deployment.resources.limits.memory=2  --set gateway.deployment.resources.limits.cpu=3 --set gateway.deployment.resources.requests.memory=4  --set gateway.deployment.resources.requests.cpu=5 --values install/test/test_values.yaml"
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
					helmFlags := "--namespace " + namespace + " --set namespace.create=true --values install/test/test_values.yaml"
					prepareMakefile(helmFlags)
					testManifest.ExpectDeploymentAppsV1(discoveryDeployment)
				})

				It("has limits", func() {
					helmFlags := "--namespace " + namespace + " --set namespace.create=true --set discovery.deployment.resources.limits.memory=2  --set discovery.deployment.resources.limits.cpu=3 --set discovery.deployment.resources.requests.memory=4  --set discovery.deployment.resources.requests.cpu=5 --values install/test/test_values.yaml"
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
			})

		})
	})
})
