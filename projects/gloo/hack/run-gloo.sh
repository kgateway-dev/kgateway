#!/usr/bin/env bash

echo "Starting envoy"
#GLOO_IP=$(docker inspect gloo -f '{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}')
GLOO_IP=127.0.0.1
cat > ./envoy.yaml <<EOF
node:
  cluster: ingress
  id: ingress~1

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
                    address: ${GLOO_IP}
                    port_value: 8080
    http2_protocol_options: {}
    type: STATIC

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

./envoy -c ./envoy.yaml