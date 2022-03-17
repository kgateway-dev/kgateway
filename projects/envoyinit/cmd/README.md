# Test with:

```
cat <<EOF > /tmp/envoy.yaml
node:
  cluster: doesntmatter
  id: imspecial
  metadata:
    role: "gloo-system~gateway-proxy"

static_resources:
  clusters:
  - name: xds_cluster
    connect_timeout: 5.000s
    http2_protocol_options: {}
    type: STRICT_DNS
    load_assignment:
      cluster_name: xds_cluster
      endpoints:
      - lb_endpoints:
        - endpoint:
            address:
              socket_address:
                address: 127.0.0.1
                port_value: 8080

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
  access_log_path: /dev/stdout
  address:
    socket_address:
      address: 0.0.0.0
      port_value: 19000

EOF
```

```
docker run --rm -ti --network=host -v /tmp/envoy.yaml:/etc/envoy/envoy.yaml:ro quay.io/solo-io/gloo-envoy-wrapper:1.3.20
```