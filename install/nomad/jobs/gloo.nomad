job "gloo" {

  datacenters = ["[[.datacenter]]"]
  type        = "service"

  update {
    max_parallel = 1
    min_healthy_time = "10s"
    healthy_deadline = "3m"
    auto_revert = false
    canary = 0
  }

  migrate {
    max_parallel = 1
    health_check = "checks"
    min_healthy_time = "10s"
    healthy_deadline = "5m"
  }

  group "gloo" {
    count = [[.gloo.replicas]]

    restart {
      attempts = 2
      interval = "30m"
      delay = "15s"
      mode = "fail"
    }

    task "gloo" {

      driver = "docker"
      config {
        image = "[[.gloo.image.registry]]/[[.gloo.image.repository]]:[[.gloo.image.tag]]"
        port_map {
          xds = [[.gloo.xdsPort]]
        }
        args = [
          "--namespace=[[.config.namespace]]",
          "--dir=${NOMAD_TASK_DIR}/settings",
        ]

        [[ if .dockerNetwork ]]
        # Use overlay network
        network_mode = "[[.dockerNetwork]]"
        [[ end ]]
      }

      template {
        data = <<EOF
gloo:
  xdsBindAddr: 0.0.0.0:[[.gloo.xdsPort]]
consul:
  address: [[.consul.address]]
  serviceDiscovery: {}
consulKvSource: {}
consulKvArtifactSource: {}
discoveryNamespace: [[.config.namespace]]
metadata:
  name: default
  namespace: [[.config.namespace]]
refreshRate: [[.config.refreshRate]]
vaultSecretSource:
  address: [[.vault.address]]
  token: [[.vault.token]]
EOF

        destination = "${NOMAD_TASK_DIR}/settings/[[.config.namespace]]/default.yaml"
      }

      resources {
        # cpu required in MHz
        cpu = [[.gloo.cpuLimit]]

        # memory required in MB
        memory = [[.gloo.memLimit]]

        network {
          # bandwidth required in MBits
          mbits = [[.gloo.bandwidthLimit]]
          port "xds" {}
        }
      }

      service {
        name = "gloo-xds"
        tags = ["gloo", "xds", "grpc"]
        port = "xds"
        check {
          name = "alive"
          type = "tcp"
          interval = "10s"
          timeout = "2s"
        }
      }

      vault {
        change_mode = "restart"
        policies = ["gloo"]
      }
    }

  }

  group "discovery" {

    restart {
      attempts = 2
      interval = "30m"
      delay = "15s"
      mode = "fail"
    }

    task "discovery" {
      driver = "docker"
      config {
        image = "[[.discovery.image.registry]]/[[.discovery.image.repository]]:[[.discovery.image.tag]]"
        args = [
          "--namespace=[[.config.namespace]]",
          "--dir=${NOMAD_TASK_DIR}/settings/",
        ]

        [[ if .dockerNetwork ]]
        # Use overlay network
        network_mode = "[[.dockerNetwork]]"
        [[ end ]]
      }

      template {
        data = <<EOF
gloo:
  xdsBindAddr: 0.0.0.0:[[.gloo.xdsPort]]
consul:
  address: [[.consul.address]]
  serviceDiscovery: {}
consulKvSource: {}
consulKvArtifactSource: {}
discoveryNamespace: [[.config.namespace]]
metadata:
  name: default
  namespace: [[.config.namespace]]
refreshRate: [[.config.refreshRate]]
vaultSecretSource:
  address: [[.vault.address]]
  token: [[.vault.token]]
EOF

        destination = "${NOMAD_TASK_DIR}/settings/[[.config.namespace]]/default.yaml"
      }

      resources {
        # cpu required in MHz
        cpu = [[.discovery.cpuLimit]]

        # memory required in MB
        memory = [[.discovery.memLimit]]

        network {
          # bandwidth required in MBits
          mbits = [[.discovery.bandwidthLimit]]
        }
      }

      vault {
        change_mode = "restart"
        policies = ["gloo"]
      }
    }
}

  group "gateway" {

    restart {
      attempts = 2
      interval = "30m"
      delay = "15s"
      mode = "fail"
    }

    task "gateway" {
    driver = "docker"
    config {
      image = "[[.gateway.image.registry]]/[[.gateway.image.repository]]:[[.gateway.image.tag]]"
      args = [
        "--namespace=[[.config.namespace]]",
        "--dir=${NOMAD_TASK_DIR}/settings/",
      ]

      [[ if .dockerNetwork ]]
      # Use overlay network
      network_mode = "[[.dockerNetwork]]"
      [[ end ]]
    }

    template {
      data = <<EOF
gloo:
  xdsBindAddr: 0.0.0.0:[[.gloo.xdsPort]]
consul:
  address: [[.consul.address]]
  serviceDiscovery: {}
consulKvSource: {}
consulKvArtifactSource: {}
discoveryNamespace: [[.config.namespace]]
metadata:
  name: default
  namespace: [[.config.namespace]]
refreshRate: [[.config.refreshRate]]
vaultSecretSource:
  address: [[.vault.address]]
  token: [[.vault.token]]
EOF

      destination = "${NOMAD_TASK_DIR}/settings/[[.config.namespace]]/default.yaml"
    }

    resources {
      # cpu required in MHz
      cpu = [[.gateway.cpuLimit]]

      # memory required in MB
      memory = [[.gateway.memLimit]]

      network {
        # bandwidth required in MBits
        mbits = [[.gateway.bandwidthLimit]]
      }
    }

  }

  }

  group "gateway-proxy" {
    count = [[.gatewayProxy.replicas]]

    restart {
      attempts = 2
      interval = "30m"
      delay = "15s"
      mode = "fail"
    }

    task "gateway-proxy" {
    driver = "docker"
    config {
      image = "[[.gatewayProxy.image.registry]]/[[.gatewayProxy.image.repository]]:[[.gatewayProxy.image.tag]]"
      port_map {
        http = [[.gatewayProxy.httpPort]]
        https = [[.gatewayProxy.httpsPort]]
        admin = [[.gatewayProxy.adminPort]]
      }
      entrypoint = ["envoy"]
      args = [
        "-c",
        "${NOMAD_TASK_DIR}/envoy.yaml",
        "--disable-hot-restart",
        "-l debug",
      ]

      [[ if .dockerNetwork ]]
      # Use overlay network
      network_mode = "[[.dockerNetwork]]"
      [[ end ]]
    }

    template {
      data = <<EOF
node:
  cluster: gateway
  id: gateway~{{ env "NOMAD_ALLOC_ID" }}
  metadata:
    # this line must match !
    role: "[[.config.namespace]]~gateway-proxy-v2"

static_resources:
  clusters:
  - name: xds_cluster
    connect_timeout: 5.000s
    load_assignment:
      cluster_name: xds_cluster
      endpoints:
      - lb_endpoints:
{{- range service "gloo-xds" }}
        - endpoint:
            address:
              socket_address:
                address: {{ .Address }}
                port_value: {{ .Port }}
{{- end }}
    http2_protocol_options: {}
    type: STATIC

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
                port_value: [[.gatewayProxy.adminPort]]

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
                    config: {}

dynamic_resources:
  ads_config:
    api_type: GRPC
    grpc_services:
    - envoy_grpc: {cluster_name: xds_cluster}
  cds_config:
    ads: {}
  lds_config:
    ads: {}

admin:
  access_log_path: /dev/null
  address:
    socket_address:
      address: 0.0.0.0
      port_value: 19000
EOF

      destination = "${NOMAD_TASK_DIR}/envoy.yaml"
    }

    resources {
      # cpu required in MHz
      cpu = [[.gatewayProxy.cpuLimit]]

      # memory required in MB
      memory = [[.gatewayProxy.memLimit]]

      network {
        # bandwidth required in MBits
        mbits = [[.gatewayProxy.bandwidthLimit]]

        port "http" {
          [[- if .gatewayProxy.exposePorts]]
          static = [[.gatewayProxy.httpPort]]
          [[- end ]]
        }
        port "https" {
          [[- if .gatewayProxy.exposePorts]]
          static = [[.gatewayProxy.httpsPort]]
          [[- end ]]
        }
        port "admin" {}
        port "stats" {}

      }
    }

    service {
      name = "gateway-proxy"
      tags = [
        "gloo",
        "http",
      ]
      port = "http"
      check {
        name = "alive"
        type = "tcp"
        interval = "10s"
        timeout = "2s"
      }
    }

    service {
      name = "gateway-proxy"
      tags = [
        "gloo",
        "https",
      ]
      port = "https"
      check {
        name = "alive"
        type = "tcp"
        interval = "10s"
        timeout = "2s"
      }
    }

    service {
      name = "gateway-proxy"
      tags = [
        "gloo",
        "admin",
      ]
      port = "admin"
      check {
        name = "alive"
        type = "tcp"
        interval = "10s"
        timeout = "2s"
      }
    }
  }

  }

 }
