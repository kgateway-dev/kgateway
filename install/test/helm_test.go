package test

import (
	"os"

	. "github.com/onsi/ginkgo"
	"github.com/solo-io/gloo/projects/gateway/pkg/translator"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	. "github.com/solo-io/go-utils/manifesttestutils"
)

var _ = Describe("Helm Test", func() {

	Describe("gateway proxy extra annotations and crds", func() {
		labels := map[string]string{
			"gloo": translator.GatewayProxyName,
			"app":  "gloo",
		}
		selector := map[string]string{
			"gloo": translator.GatewayProxyName,
		}

		prepareMakefile := func(helmFlags string) {
			makefileSerializer.Lock()
			defer makefileSerializer.Unlock()
			MustMake(".", "-C", "../..", "install/gloo-gateway.yaml", "HELMFLAGS="+helmFlags)
			testManifest = NewTestManifest("../gloo-gateway.yaml")
			version = os.Getenv("TAGGED_VERSION")
			if version == "" {
				version = "dev"
			} else {
				version = version[1:]
			}
		}

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

		It("has a proxy without tracing", func() {
			helmFlags := "--namespace " + namespace + " --set namespace.create=true  --set gatewayProxies.gatewayProxyV2.service.extraAnnotations.test=test"
			prepareMakefile(helmFlags)
			proxySpec := make(map[string]string)
			proxySpec["envoy.yaml"] = `
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
			cmName := "gateway-proxy-v2-envoy-config"
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
			helmFlags := "--namespace " + namespace + " --set namespace.create=true  --set gatewayProxies.gatewayProxyV2.service.extraAnnotations.test=test --values install/test/test_values.yaml"
			prepareMakefile(helmFlags)
			proxySpec := make(map[string]string)
			proxySpec["envoy.yaml"] = `
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
			cmName := "gateway-proxy-v2-envoy-config"
			cmRb := ResourceBuilder{
				Namespace: namespace,
				Name:      cmName,
				Labels:    labels,
				Data:      proxySpec,
			}
			proxy := cmRb.GetConfigMap()
			testManifest.ExpectConfigMapWithYamlData(proxy)
		})
	})
})
