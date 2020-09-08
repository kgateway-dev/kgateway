---
title: Gloo and Istio mTLS
weight: 3
---

Serving as the Ingress for an Istio cluster -- without compromising on security -- means supporting mutual TLS (mTLS) communication between Gloo and the rest of the cluster. Mutual TLS means that the client proves its identity to the server (in addition to the server proving its identity to the client, which happens in regular TLS).

## Guide versions

### Istio versions

This guide was tested with Istio 1.6.6 and 1.7. For older versions of Istio, see [here]({{% versioned_link_path fromRoot="/guides/integrations/service_mesh/gloo_istio_mtls/older_istio_versions/" %}}).

### Gloo versions

This guide was tested with Gloo v1.5.0-beta23.

{{% notice warning %}}

The Gloo integration with Istio 1.6.x requires Gloo version 1.4.10, or 1.5.0-beta23 or higher.

{{% /notice %}}

### Kubernetes versions

This guide was tested with GKE v1.15.


{{% notice note %}}
Please note that if you are running Kubernetes > 1.12 in Minikube, you may run into several issues later on when installing Istio in SDS mode. This mode requires the projection of the istio-token service account tokens into volumes. We recommend installing Istio in a cluster which has this feature turned on by default (for example, GKE). See [token-authentication](https://kubernetes.io/docs/reference/access-authn-authz/authentication/#webhook-token-authentication) for more details.

For local development and testing, if you remove the istio-token mount then Istio will default to using the service account token instead of the projected token. You will also need to set the Environment variable JWT_POLICY to "first-party-jwt" in the istio sidecar container attached to gateway-proxy.
{{% /notice %}}

---

## Step 1 - Install Istio

### Download and install

To download and install the latest version of Istio, we will be following the installation instructions [here](https://istio.io/docs/setup/getting-started/).

```bash
curl -L https://istio.io/downloadIstio | ISTIO_VERSION=1.6.6 sh -
cd istio-1.6.6
istioctl install --set profile=demo
```

Use `kubectl get pods -n istio-system` to check the status on the Istio pods and wait until all the pods are **Running** or **Completed**.


## Step 2 - Install bookinfo

Before configuring Gloo, you'll need to install the bookinfo sample app to be consistent with this guide, or you can use your preferred Upstream. Either way, you'll need to enable istio-injection in the default namespace:

```bash
kubectl label namespace default istio-injection=enabled
```

To install the bookinfo sample app, cd into your downloaded Istio directory and run this command:
```bash
kubectl apply -f samples/bookinfo/platform/kube/bookinfo.yaml
```

---

## Step 3 - Configure Gloo

If necessary, install Gloo with either glooctl:
```
glooctl install gateway
```
or with helm:
```
kubectl create ns gloo-system; helm install --namespace gloo-system --version 1.5.0-beta23 gloo gloo/gloo
```
See the [quick start]({{% versioned_link_path fromRoot="/installation/gateway/kubernetes/" %}}) guide for more information.

Gloo is installed to the `gloo-system` namespace and should *not* be injected with the Istio sidecar. If you have automatic injection enabled for Istio, make sure the `istio-injection` label does *not* exist on the `gloo-system` namespace. See [the Istio docs on automatic sidecar injection](https://istio.io/docs/setup/kubernetes/additional-setup/sidecar-injection/#automatic-sidecar-injection) for more.

For Gloo to successfully send requests to an Istio Upstream with mTLS enabled, we need to add the Istio mTLS secret to the gateway-proxy pod. The secret allows Gloo to authenticate with the Upstream service. We will also add an SDS server container to the pod, to handle cert rotation when Istio updates its certs.

First, we will edit the gateway-proxy configmap to tell it to listen to the SDS server. You can edit it with this command:
```bash
kubectl edit configmaps -n gloo-system gateway-proxy-envoy-config
```

We need to add the gateway_proxy_sds cluster as follows:
{{< highlight yaml "hl_lines=110-121" >}}
apiVersion: v1
data:
  envoy.yaml: |
    layered_runtime:
      layers:
      - name: static_layer
        static_layer:
          overload:
            global_downstream_max_connections: 250000
      - name: admin_layer
        admin_layer: {}
    node:
      cluster: gateway
      id: "{{.PodName}}.{{.PodNamespace}}"
      metadata:
        role: "{{.PodNamespace}}~gateway-proxy"
    stats_sinks:
      - name: envoy.stat_sinks.metrics_service
        typed_config:
          "@type": type.googleapis.com/envoy.config.metrics.v3.MetricsServiceConfig
          grpc_service:
            envoy_grpc: {cluster_name: gloo.gloo-system.svc.cluster.local:9966}
    static_resources:
      listeners:
        - name: prometheus_listener
          address:
            socket_address:
              address: 0.0.0.0
              port_value: 8081
          filter_chains:
            - filters:
                - name: envoy.filters.network.http_connection_manager
                  typed_config:
                    "@type": type.googleapis.com/envoy.extensions.filters.network.http_connection_manager.v3.HttpConnectionManager
                    codec_type: AUTO
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
                      - name: envoy.filters.http.router
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
        respect_dns_ttl: true
      - name: rest_xds_cluster
        alt_stat_name: rest_xds_cluster
        connect_timeout: 5.000s
        load_assignment:
          cluster_name: rest_xds_cluster
          endpoints:
          - lb_endpoints:
            - endpoint:
                address:
                  socket_address:
                    address: gloo.gloo-system.svc.cluster.local
                    port_value: 9976
        upstream_connection_options:
          tcp_keepalive: {}
        type: STRICT_DNS
        respect_dns_ttl: true
      - name: wasm-cache
        connect_timeout: 5.000s
        load_assignment:
          cluster_name: wasm-cache
          endpoints:
          - lb_endpoints:
            - endpoint:
                address:
                  socket_address:
                    address: gloo.gloo-system.svc.cluster.local
                    port_value: 9979
        upstream_connection_options:
          tcp_keepalive: {}
        type: STRICT_DNS
        respect_dns_ttl: true
      - name: gateway_proxy_sds
        connect_timeout: 0.25s
        http2_protocol_options: {}
        load_assignment:
          cluster_name: gateway_proxy_sds
          endpoints:
          - lb_endpoints:
            - endpoint:
                address:
                  socket_address:
                    address: 127.0.0.1
                    port_value: 8234
      - name: gloo.gloo-system.svc.cluster.local:9966
        alt_stat_name: metrics_cluster
        connect_timeout: 5.000s
        load_assignment:
            cluster_name: gloo.gloo-system.svc.cluster.local:9966
            endpoints:
            - lb_endpoints:
              - endpoint:
                    address:
                        socket_address:
                            address: gloo.gloo-system.svc.cluster.local
                            port_value: 9966
        http2_protocol_options: {}
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
                    port_value: 19000

    dynamic_resources:
      ads_config:
        api_type: GRPC
        rate_limit_settings: {}
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
          port_value: 19000
kind: ConfigMap
metadata:
  labels:
    app: gloo
    gateway-proxy-id: gateway-proxy
    gloo: gateway-proxy
  name: gateway-proxy-envoy-config
  namespace: gloo-system
{{</highlight>}}


Edit the gateway-proxy with this command:
```bash
kubectl edit deploy/gateway-proxy -n gloo-system
```

We will update our gateway-proxy deployment as follows:

{{< highlight yaml "hl_lines=61-177 183-200" >}}
apiVersion: apps/v1
kind: Deployment
metadata:
  labels:
    app: gloo
    gateway-proxy-id: gateway-proxy
    gloo: gateway-proxy
  name: gateway-proxy
  namespace: gloo-system
spec:
  selector:
    matchLabels:
      gateway-proxy-id: gateway-proxy
      gloo: gateway-proxy
  template:
    metadata:
      annotations:
        prometheus.io/path: /metrics
        prometheus.io/port: "8081"
        prometheus.io/scrape: "true"
      labels:
        gateway-proxy: live
        gateway-proxy-id: gateway-proxy
        gloo: gateway-proxy
    spec:
      containers:
      - args:
        - --disable-hot-restart
        env:
        - name: POD_NAMESPACE
          valueFrom:
            fieldRef:
              apiVersion: v1
              fieldPath: metadata.namespace
        - name: POD_NAME
          valueFrom:
            fieldRef:
              apiVersion: v1
              fieldPath: metadata.name
        image: quay.io/solo-io/gloo-envoy-wrapper:1.5.0-beta23
        imagePullPolicy: IfNotPresent
        name: gateway-proxy
        ports:
        - containerPort: 8080
          name: http
          protocol: TCP
        - containerPort: 8443
          name: https
          protocol: TCP
        resources: {}
        securityContext:
          allowPrivilegeEscalation: false
          capabilities:
            add:
            - NET_BIND_SERVICE
            drop:
            - ALL
        volumeMounts:
        - mountPath: /etc/envoy
          name: envoy-config
      - name: cert-rotator
        image: quay.io/solo-io/sds:1.5.0-beta23
        imagePullPolicy: IfNotPresent
        ports:
        - containerPort: 8234
          name: sds
          protocol: TCP
        volumeMounts:
        - mountPath: /etc/istio-certs/
          name: istio-certs
        - mountPath: /etc/envoy
          name: envoy-config
        env:
          - name: POD_NAME
            valueFrom:
              fieldRef:
                fieldPath: metadata.name
          - name: POD_NAMESPACE
            valueFrom:
              fieldRef:
                fieldPath: metadata.namespace
          - name: ISTIO_MTLS_SDS_ENABLED
            value: "true"
      - name: istio-proxy
        image: docker.io/istio/proxyv2:1.6.6
        args:
        - proxy
        - sidecar
        - --domain
        - $(POD_NAMESPACE).svc.cluster.local
        - --configPath
        - /etc/istio/proxy
        - --binaryPath
        - /usr/local/bin/envoy
        - --serviceCluster
        - istio-proxy-prometheus
        - --drainDuration
        - 45s
        - --parentShutdownDuration
        - 1m0s
        - --discoveryAddress
        - istio-pilot.istio-system.svc:15012
        - --proxyLogLevel=warning
        - --proxyComponentLogLevel=misc:error
        - --connectTimeout
        - 10s
        - --proxyAdminPort
        - "15000"
        - --controlPlaneAuthPolicy
        - NONE
        - --dnsRefreshRate
        - 300s
        - --statusPort
        - "15021"
        - --trust-domain=cluster.local
        - --controlPlaneBootstrap=false
        env:
          - name: OUTPUT_CERTS
            value: "/etc/istio-certs"
          - name: JWT_POLICY
            value: third-party-jwt
          - name: PILOT_CERT_PROVIDER
            value: istiod
          - name: CA_ADDR
            value: istiod.istio-system.svc:15012
          - name: ISTIO_META_MESH_ID
            value: cluster.local
          - name: POD_NAME
            valueFrom:
              fieldRef:
                fieldPath: metadata.name
          - name: POD_NAMESPACE
            valueFrom:
              fieldRef:
                fieldPath: metadata.namespace
          - name: INSTANCE_IP
            valueFrom:
              fieldRef:
                fieldPath: status.podIP
          - name: SERVICE_ACCOUNT
            valueFrom:
              fieldRef:
                fieldPath: spec.serviceAccountName
          - name: HOST_IP
            valueFrom:
              fieldRef:
                fieldPath: status.hostIP
          - name: ISTIO_META_POD_NAME
            valueFrom:
              fieldRef:
                apiVersion: v1
                fieldPath: metadata.name
          - name: ISTIO_META_CONFIG_NAMESPACE
            valueFrom:
              fieldRef:
                apiVersion: v1
                fieldPath: metadata.namespace
        imagePullPolicy: IfNotPresent
        readinessProbe:
          failureThreshold: 30
          httpGet:
            path: /healthz/ready
            port: 15021
            scheme: HTTP
          initialDelaySeconds: 1
          periodSeconds: 2
          successThreshold: 1
          timeoutSeconds: 1
        volumeMounts:
        - mountPath: /var/run/secrets/istio
          name: istiod-ca-cert
        - mountPath: /etc/istio/proxy
          name: istio-envoy
        - mountPath: /etc/istio-certs/
          name: istio-certs
        - mountPath: /var/run/secrets/tokens
          name: istio-token
      volumes:
      - configMap:
          defaultMode: 420
          name: gateway-proxy-envoy-config
        name: envoy-config
      - name: istio-certs
        emptyDir:
          medium: Memory
      - name: istiod-ca-cert
        configMap:
          defaultMode: 420
          name: istio-ca-root-cert
      - emptyDir:
          medium: Memory
        name: istio-envoy
      - name: istio-token
        projected:
          defaultMode: 420
          sources:
          - serviceAccountToken:
              audience: istio-ca
              expirationSeconds: 43200
              path: istio-token
{{</highlight>}}

The last configuration step is to configure the relevant Gloo Upstreams with mTLS. We can be fine-grained about which Upstreams have these settings as not all Gloo Upstreams may need/want mTLS enabled. This gives us the flexibility to route to Upstreams both with and without mTLS enabled - a common occurrence in a brown-field environment or during a migration to Istio.

For Gloo versions 1.1.x and up, you must disable function discovery before editing the Upstream to prevent your change from being overwritten by Gloo:

```bash
kubectl label namespace default discovery.solo.io/function_discovery=disabled
```

Edit the Upstream with this command:
```bash
kubectl edit upstream default-productpage-9080 --namespace gloo-system
```

Here's an example of the edited Upstream for Istio 1.6.6:

{{< highlight yaml "hl_lines=17-23" >}}
apiVersion: gloo.solo.io/v1
kind: Upstream
metadata:
  labels:
    app: productpage
    service: productpage
  name: default-productpage-9080-sds
  namespace: gloo-system
spec:
  discoveryMetadata: {}
  kube:
    selector:
      app: productpage
    serviceName: productpage
    serviceNamespace: default
    servicePort: 9080
  sslConfig:
    alpn_protocols:
    - istio
    sds:
      targetUri: 127.0.0.1:8234
      certificatesSecretName: istio_server_cert
      validationContextName: istio_validation_context
{{</highlight>}}

Next we're going to lock down the mesh so that only mTLS traffic is allowed:
```bash
kubectl apply -n istio-system -f - <<EOF
apiVersion: "security.istio.io/v1beta1"
kind: "PeerAuthentication"
metadata:
  name: "default"
spec:
  mtls:
    mode: STRICT
EOF
```

Alternatively, if you're not ready to lock down your entire mesh, you can change settings on a namespace or workload level. To enable stict mTLS for just the productpage app we've configured, we can run:
```bash
cat <<EOF | kubectl apply -f -
apiVersion: "security.istio.io/v1beta1"
kind: "PeerAuthentication"
metadata:
  name: "productpage"
  namespace: "default"
spec:
  selector:
    matchLabels:
      app: productpage
  mtls:
    mode: STRICT
EOF
```

More details on configuring PeerAuthentication policies can be found [here](https://istio.io/latest/docs/tasks/security/authentication/authn-policy/).

To test this out, we need a route in Gloo:
```bash
glooctl add route --name prodpage --namespace gloo-system --path-prefix / --dest-name default-productpage-9080 --dest-namespace gloo-system
```

And we can curl it:

```bash
curl -v $(glooctl proxy url)/productpage
```

Or access it in the browser:
```bash
HTTP_GW=$(glooctl proxy url)
## Open the ingress url in the browser:
$([ "$(uname -s)" = "Linux" ] && echo xdg-open || echo open) $HTTP_GW/productpage
```
