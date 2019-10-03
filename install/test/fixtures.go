package test

var confWithoutTracing = `
admin:
  access_log_path: /dev/null
  address:
    socket_address:
      address: 127.0.0.1
      port_value: 19000
dynamic_resources:
  ads_config:
    api_type: GRPC
    grpc_services:
    - envoy_grpc:
        cluster_name: gloo.gloo-system.svc.cluster.local:9977
    rate_limit_settings: {}
  cds_config:
    ads: {}
  lds_config:
    ads: {}
node:
  cluster: gateway
  id: '{{.PodName}}.{{.PodNamespace}}'
  metadata:
    role: '{{.PodNamespace}}~gateway-proxy-v2'
static_resources:
  clusters:
  - alt_stat_name: xds_cluster
    connect_timeout: 5.000s
    http2_protocol_options: {}
    load_assignment:
      cluster_name: gloo.gloo-system.svc.cluster.local:9977
      endpoints:
      - lb_endpoints:
        - endpoint:
            address:
              socket_address:
                address: gloo.gloo-system.svc.cluster.local
                port_value: 9977
    name: gloo.gloo-system.svc.cluster.local:9977
    respect_dns_ttl: true
    type: STRICT_DNS
    upstream_connection_options:
      tcp_keepalive: {}
  - alt_stat_name: metrics_cluster
    connect_timeout: 5.000s
    http2_protocol_options: {}
    load_assignment:
      cluster_name: gloo.gloo-system.svc.cluster.local:9966
      endpoints:
      - lb_endpoints:
        - endpoint:
            address:
              socket_address:
                address: gloo.gloo-system.svc.cluster.local
                port_value: 9966
    name: gloo.gloo-system.svc.cluster.local:9966
    type: STRICT_DNS
  - connect_timeout: 5.000s
    lb_policy: ROUND_ROBIN
    load_assignment:
      cluster_name: admin_port_cluster
      endpoints:
      - lb_endpoints:
        - endpoint:
            address:
              socket_address:
                address: 127.0.0.1
                port_value: 19000
    name: admin_port_cluster
    type: STATIC
  listeners:
  - address:
      socket_address:
        address: 0.0.0.0
        port_value: 8081
    filter_chains:
    - filters:
      - config:
          codec_type: auto
          http_filters:
          - config: {}
            name: envoy.router
          route_config:
            name: prometheus_route
            virtual_hosts:
            - domains:
              - '*'
              name: prometheus_host
              routes:
              - match:
                  headers:
                  - exact_match: GET
                    name: :method
                  path: /ready
                route:
                  cluster: admin_port_cluster
              - match:
                  headers:
                  - exact_match: GET
                    name: :method
                  prefix: /metrics
                route:
                  cluster: admin_port_cluster
                  prefix_rewrite: /stats/prometheus
          stat_prefix: prometheus
        name: envoy.http_connection_manager
    name: prometheus_listener
stats_sinks:
- config:
    grpc_service:
      envoy_grpc:
        cluster_name: gloo.gloo-system.svc.cluster.local:9966
  name: envoy.metrics_service
`

var confWithTracingProvider = `
admin:
  access_log_path: /dev/null
  address:
    socket_address:
      address: 127.0.0.1
      port_value: 19000
dynamic_resources:
  ads_config:
    api_type: GRPC
    grpc_services:
    - envoy_grpc:
        cluster_name: gloo.gloo-system.svc.cluster.local:9977
    rate_limit_settings: {}
  cds_config:
    ads: {}
  lds_config:
    ads: {}
node:
  cluster: gateway
  id: '{{.PodName}}.{{.PodNamespace}}'
  metadata:
    role: '{{.PodNamespace}}~gateway-proxy-v2'
static_resources:
  clusters:
  - alt_stat_name: xds_cluster
    connect_timeout: 5.000s
    http2_protocol_options: {}
    load_assignment:
      cluster_name: gloo.gloo-system.svc.cluster.local:9977
      endpoints:
      - lb_endpoints:
        - endpoint:
            address:
              socket_address:
                address: gloo.gloo-system.svc.cluster.local
                port_value: 9977
    name: gloo.gloo-system.svc.cluster.local:9977
    respect_dns_ttl: true
    type: STRICT_DNS
    upstream_connection_options:
      tcp_keepalive: {}
  - alt_stat_name: metrics_cluster
    connect_timeout: 5.000s
    http2_protocol_options: {}
    load_assignment:
      cluster_name: gloo.gloo-system.svc.cluster.local:9966
      endpoints:
      - lb_endpoints:
        - endpoint:
            address:
              socket_address:
                address: gloo.gloo-system.svc.cluster.local
                port_value: 9966
    name: gloo.gloo-system.svc.cluster.local:9966
    type: STRICT_DNS
  - connect_timeout: 5.000s
    lb_policy: ROUND_ROBIN
    load_assignment:
      cluster_name: admin_port_cluster
      endpoints:
      - lb_endpoints:
        - endpoint:
            address:
              socket_address:
                address: 127.0.0.1
                port_value: 19000
    name: admin_port_cluster
    type: STATIC
  listeners:
  - address:
      socket_address:
        address: 0.0.0.0
        port_value: 8081
    filter_chains:
    - filters:
      - config:
          codec_type: auto
          http_filters:
          - config: {}
            name: envoy.router
          route_config:
            name: prometheus_route
            virtual_hosts:
            - domains:
              - '*'
              name: prometheus_host
              routes:
              - match:
                  headers:
                  - exact_match: GET
                    name: :method
                  path: /ready
                route:
                  cluster: admin_port_cluster
              - match:
                  headers:
                  - exact_match: GET
                    name: :method
                  prefix: /metrics
                route:
                  cluster: admin_port_cluster
                  prefix_rewrite: /stats/prometheus
          stat_prefix: prometheus
        name: envoy.http_connection_manager
    name: prometheus_listener
stats_sinks:
- config:
    grpc_service:
      envoy_grpc:
        cluster_name: gloo.gloo-system.svc.cluster.local:9966
  name: envoy.metrics_service
tracing:
  http:
    another: line
    trace: spec
`

var confWithTracingProviderCluster = `
admin:
  access_log_path: /dev/null
  address:
    socket_address:
      address: 127.0.0.1
      port_value: 19000
dynamic_resources:
  ads_config:
    api_type: GRPC
    grpc_services:
    - envoy_grpc:
        cluster_name: gloo.gloo-system.svc.cluster.local:9977
    rate_limit_settings: {}
  cds_config:
    ads: {}
  lds_config:
    ads: {}
node:
  cluster: gateway
  id: '{{.PodName}}.{{.PodNamespace}}'
  metadata:
    role: '{{.PodNamespace}}~gateway-proxy-v2'
static_resources:
  clusters:
  - alt_stat_name: xds_cluster
    connect_timeout: 5.000s
    http2_protocol_options: {}
    load_assignment:
      cluster_name: gloo.gloo-system.svc.cluster.local:9977
      endpoints:
      - lb_endpoints:
        - endpoint:
            address:
              socket_address:
                address: gloo.gloo-system.svc.cluster.local
                port_value: 9977
    name: gloo.gloo-system.svc.cluster.local:9977
    respect_dns_ttl: true
    type: STRICT_DNS
    upstream_connection_options:
      tcp_keepalive: {}
  - alt_stat_name: metrics_cluster
    connect_timeout: 5.000s
    http2_protocol_options: {}
    load_assignment:
      cluster_name: gloo.gloo-system.svc.cluster.local:9966
      endpoints:
      - lb_endpoints:
        - endpoint:
            address:
              socket_address:
                address: gloo.gloo-system.svc.cluster.local
                port_value: 9966
    name: gloo.gloo-system.svc.cluster.local:9966
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
    respect_dns_ttl: true
    type: STRICT_DNS
  - connect_timeout: 5.000s
    lb_policy: ROUND_ROBIN
    load_assignment:
      cluster_name: admin_port_cluster
      endpoints:
      - lb_endpoints:
        - endpoint:
            address:
              socket_address:
                address: 127.0.0.1
                port_value: 19000
    name: admin_port_cluster
    type: STATIC
  listeners:
  - address:
      socket_address:
        address: 0.0.0.0
        port_value: 8081
    filter_chains:
    - filters:
      - config:
          codec_type: auto
          http_filters:
          - config: {}
            name: envoy.router
          route_config:
            name: prometheus_route
            virtual_hosts:
            - domains:
              - '*'
              name: prometheus_host
              routes:
              - match:
                  headers:
                  - exact_match: GET
                    name: :method
                  path: /ready
                route:
                  cluster: admin_port_cluster
              - match:
                  headers:
                  - exact_match: GET
                    name: :method
                  prefix: /metrics
                route:
                  cluster: admin_port_cluster
                  prefix_rewrite: /stats/prometheus
          stat_prefix: prometheus
        name: envoy.http_connection_manager
    name: prometheus_listener
stats_sinks:
- config:
    grpc_service:
      envoy_grpc:
        cluster_name: gloo.gloo-system.svc.cluster.local:9966
  name: envoy.metrics_service
tracing:
  http:
    typed_config:
      '@type': type.googleapis.com/envoy.config.trace.v2.ZipkinConfig
      collector_cluster: zipkin
      collector_endpoint: /api/v1/spans
`

var confWithReadConfig = `
admin:
  access_log_path: /dev/null
  address:
    socket_address:
      address: 127.0.0.1
      port_value: 19000
dynamic_resources:
  ads_config:
    api_type: GRPC
    grpc_services:
    - envoy_grpc:
        cluster_name: gloo.gloo-system.svc.cluster.local:9977
    rate_limit_settings: {}
  cds_config:
    ads: {}
  lds_config:
    ads: {}
node:
  cluster: gateway
  id: '{{.PodName}}.{{.PodNamespace}}'
  metadata:
    role: '{{.PodNamespace}}~gateway-proxy-v2'
static_resources:
  clusters:
  - alt_stat_name: xds_cluster
    connect_timeout: 5.000s
    http2_protocol_options: {}
    load_assignment:
      cluster_name: gloo.gloo-system.svc.cluster.local:9977
      endpoints:
      - lb_endpoints:
        - endpoint:
            address:
              socket_address:
                address: gloo.gloo-system.svc.cluster.local
                port_value: 9977
    name: gloo.gloo-system.svc.cluster.local:9977
    respect_dns_ttl: true
    type: STRICT_DNS
    upstream_connection_options:
      tcp_keepalive: {}
  - alt_stat_name: metrics_cluster
    connect_timeout: 5.000s
    http2_protocol_options: {}
    load_assignment:
      cluster_name: gloo.gloo-system.svc.cluster.local:9966
      endpoints:
      - lb_endpoints:
        - endpoint:
            address:
              socket_address:
                address: gloo.gloo-system.svc.cluster.local
                port_value: 9966
    name: gloo.gloo-system.svc.cluster.local:9966
    type: STRICT_DNS
  - connect_timeout: 5.000s
    lb_policy: ROUND_ROBIN
    load_assignment:
      cluster_name: admin_port_cluster
      endpoints:
      - lb_endpoints:
        - endpoint:
            address:
              socket_address:
                address: 127.0.0.1
                port_value: 19000
    name: admin_port_cluster
    type: STATIC
  listeners:
  - address:
      socket_address:
        address: 0.0.0.0
        port_value: 8081
    filter_chains:
    - filters:
      - config:
          codec_type: auto
          http_filters:
          - config: {}
            name: envoy.router
          route_config:
            name: prometheus_route
            virtual_hosts:
            - domains:
              - '*'
              name: prometheus_host
              routes:
              - match:
                  headers:
                  - exact_match: GET
                    name: :method
                  path: /ready
                route:
                  cluster: admin_port_cluster
              - match:
                  headers:
                  - exact_match: GET
                    name: :method
                  prefix: /metrics
                route:
                  cluster: admin_port_cluster
                  prefix_rewrite: /stats/prometheus
          stat_prefix: prometheus
        name: envoy.http_connection_manager
    name: prometheus_listener
  - address:
      socket_address:
        address: 0.0.0.0
        port_value: 8082
    filter_chains:
    - filters:
      - config:
          codec_type: auto
          http_filters:
          - config: {}
            name: envoy.router
          route_config:
            name: read_config_route
            virtual_hosts:
            - domains:
              - '*'
              name: read_config_host
              routes:
              - match:
                  headers:
                  - exact_match: GET
                    name: :method
                  path: /ready
                route:
                  cluster: admin_port_cluster
              - match:
                  headers:
                  - exact_match: GET
                    name: :method
                  prefix: /stats
                route:
                  cluster: admin_port_cluster
              - match:
                  headers:
                  - exact_match: GET
                    name: :method
                  prefix: /config_dump
                route:
                  cluster: admin_port_cluster
          stat_prefix: read_config
        name: envoy.http_connection_manager
    name: read_config_listener
stats_sinks:
- config:
    grpc_service:
      envoy_grpc:
        cluster_name: gloo.gloo-system.svc.cluster.local:9966
  name: envoy.metrics_service
`

var confWithAccessLogger = `
admin:
  access_log_path: /dev/null
  address:
    socket_address:
      address: 127.0.0.1
      port_value: 19000
dynamic_resources:
  ads_config:
    api_type: GRPC
    grpc_services:
    - envoy_grpc:
        cluster_name: gloo.gloo-system.svc.cluster.local:9977
    rate_limit_settings: {}
  cds_config:
    ads: {}
  lds_config:
    ads: {}
node:
  cluster: gateway
  id: '{{.PodName}}.{{.PodNamespace}}'
  metadata:
    role: '{{.PodNamespace}}~gateway-proxy-v2'
static_resources:
  clusters:
  - alt_stat_name: xds_cluster
    connect_timeout: 5.000s
    http2_protocol_options: {}
    load_assignment:
      cluster_name: gloo.gloo-system.svc.cluster.local:9977
      endpoints:
      - lb_endpoints:
        - endpoint:
            address:
              socket_address:
                address: gloo.gloo-system.svc.cluster.local
                port_value: 9977
    name: gloo.gloo-system.svc.cluster.local:9977
    respect_dns_ttl: true
    type: STRICT_DNS
    upstream_connection_options:
      tcp_keepalive: {}
  - alt_stat_name: metrics_cluster
    connect_timeout: 5.000s
    http2_protocol_options: {}
    load_assignment:
      cluster_name: gloo.gloo-system.svc.cluster.local:9966
      endpoints:
      - lb_endpoints:
        - endpoint:
            address:
              socket_address:
                address: gloo.gloo-system.svc.cluster.local
                port_value: 9966
    name: gloo.gloo-system.svc.cluster.local:9966
    type: STRICT_DNS
  - connect_timeout: 5.000s
    http2_protocol_options: {}
    load_assignment:
      cluster_name: access_log_cluster
      endpoints:
      - lb_endpoints:
        - endpoint:
            address:
              socket_address:
                address: gateway-proxy-v2-access-logger.gloo-system.svc.cluster.local
                port_value: 8083
    name: access_log_cluster
    type: STRICT_DNS
  - connect_timeout: 5.000s
    lb_policy: ROUND_ROBIN
    load_assignment:
      cluster_name: admin_port_cluster
      endpoints:
      - lb_endpoints:
        - endpoint:
            address:
              socket_address:
                address: 127.0.0.1
                port_value: 19000
    name: admin_port_cluster
    type: STATIC
  listeners:
  - address:
      socket_address:
        address: 0.0.0.0
        port_value: 8081
    filter_chains:
    - filters:
      - config:
          codec_type: auto
          http_filters:
          - config: {}
            name: envoy.router
          route_config:
            name: prometheus_route
            virtual_hosts:
            - domains:
              - '*'
              name: prometheus_host
              routes:
              - match:
                  headers:
                  - exact_match: GET
                    name: :method
                  path: /ready
                route:
                  cluster: admin_port_cluster
              - match:
                  headers:
                  - exact_match: GET
                    name: :method
                  prefix: /metrics
                route:
                  cluster: admin_port_cluster
                  prefix_rewrite: /stats/prometheus
          stat_prefix: prometheus
        name: envoy.http_connection_manager
    name: prometheus_listener
stats_sinks:
- config:
    grpc_service:
      envoy_grpc:
        cluster_name: gloo.gloo-system.svc.cluster.local:9966
  name: envoy.metrics_service
`
