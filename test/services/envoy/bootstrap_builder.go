package envoy

import (
	"bytes"
	"os"
	"text/template"
)

type BootstrapBuilder interface {
	Build(ei *Instance) string
}

type templateBootstrapBuilder struct {
	template *template.Template
}

func (tbb *templateBootstrapBuilder) Build(ei *Instance) string {
	var b bytes.Buffer
	if err := tbb.template.Execute(&b, ei); err != nil {
		panic(err)
	}
	return b.String()
}

type fileBootstrapBuilder struct {
	file string
}

func (fbb *fileBootstrapBuilder) Build(ei *Instance) string {
	templateBytes, err := os.ReadFile(fbb.file)
	if err != nil {
		panic(err)
	}

	parsedTemplate := template.Must(template.New(fbb.file).Parse(string(templateBytes)))

	var b bytes.Buffer
	if err := parsedTemplate.Execute(&b, ei); err != nil {
		panic(err)
	}
	return b.String()
}

const boostrapText = `
layered_runtime:
  layers:
  - name: static_layer
    static_layer:
      upstream:
        healthy_panic_threshold:
          value: 0
  - name: admin_layer
    admin_layer: {}
node:
 cluster: ingress
 id: {{.ID}}
 metadata:
  role: {{.Role}}

static_resources:
  clusters:
  - name: xds_cluster
    connect_timeout: 5.000s
    load_assignment:
      cluster_name: xds_cluster
      endpoints:
        - lb_endpoints:
            - endpoint:
                address:
                  socket_address:
                    address: {{.GlooAddr}}
                    port_value: {{.Port}}
    http2_protocol_options: {}
    type: STATIC
  - name: rest_xds_cluster
    connect_timeout: 5.000s
    load_assignment:
      cluster_name: rest_xds_cluster
      endpoints:
        - lb_endpoints:
            - endpoint:
                address:
                  socket_address:
                    address: {{.GlooAddr}}
                    port_value: {{.RestXdsPort}}
    upstream_connection_options:
      tcp_keepalive: {}
    type: STRICT_DNS
    respect_dns_ttl: true
{{if .RatelimitAddr}}
  - name: ratelimit_cluster
    connect_timeout: 5.000s
    load_assignment:
      cluster_name: ratelimit_cluster
      endpoints:
        - lb_endpoints:
            - endpoint:
                address:
                  socket_address:
                    address: {{.RatelimitAddr}}
                    port_value: {{.RatelimitPort}}
    http2_protocol_options: {}
    type: STATIC
{{end}}
{{if .AccessLogAddr}}
  - name: access_log_cluster
    connect_timeout: 5.000s
    load_assignment:
      cluster_name: access_log_cluster
      endpoints:
        - lb_endpoints:
            - endpoint:
                address:
                  socket_address:
                    address: {{.AccessLogAddr}}
                    port_value: {{.AccessLogPort}}
    http2_protocol_options: {}
    type: STATIC
{{end}}
  - name: aws_sts_cluster
    connect_timeout: 5.000s
    type: LOGICAL_DNS
    lb_policy: ROUND_ROBIN
    load_assignment:
      cluster_name: aws_sts_cluster
      endpoints:
      - lb_endpoints:
        - endpoint:
            address:
              socket_address:
                port_value: 443
                address: sts.amazonaws.com
    transport_socket:
      name: envoy.transport_sockets.tls
      typed_config:
        "@type": type.googleapis.com/envoy.extensions.transport_sockets.tls.v3.UpstreamTlsContext
        sni: sts.amazonaws.com
dynamic_resources:
  ads_config:
    transport_api_version: {{ .ApiVersion }}
    api_type: GRPC
    grpc_services:
    - envoy_grpc: {cluster_name: xds_cluster}
  cds_config:
    resource_api_version: {{ .ApiVersion }}
    ads: {}
  lds_config:
    resource_api_version: {{ .ApiVersion }}
    ads: {}

admin:
  access_log_path: /dev/null
  address:
    socket_address:
      address: 0.0.0.0
      port_value: {{.AdminPort}}

`

var bootstrapTemplate = template.Must(template.New("bootstrap").Parse(boostrapText))
