---
title: Tracing Setup
weight: 4
description: Configure Gloo Edge for tracing
---

## Tracing

Gloo Edge makes it easy to implement tracing on your system through [Envoy's tracing capabilities](https://www.envoyproxy.io/docs/envoy/latest/intro/arch_overview/observability/tracing.html).   
Following list of tracing systems are currently supported in Gloo Edge:
* Zipkin
* Jaeger
* Datadog

#### Usage

*If you have not yet enabled tracing, please see the [configuration](#configuration) details below.*

- Produce a trace by passing the header: `x-client-trace-id`
  - This id provides a means of associating the spans produced during a trace. The value must be unique, a uuid4 is [recommended](https://www.envoyproxy.io/docs/envoy/v1.9.0/configuration/http_conn_man/headers#config-http-conn-man-headers-x-client-trace-id).
- Optionally annotate your trace with the `x-envoy-decorator-operation` header.
  - This will be emitted with the resulting trace and can be a means of identifying the origin of a given trace. Note that it will override any pre-specified route decorator. Additional details can be found [here](https://www.envoyproxy.io/docs/envoy/v1.11.2/configuration/http_filters/router_filter#config-http-filters-router-x-envoy-decorator-operation).

#### Configuration

There are a few steps to make tracing available through Gloo Edge:
1. Configure a tracing cluster
1. Configure a tracing provider
1. (Optional) Annotate routes with descriptors

##### 1. Configure a tracing cluster

Tracing requires a cluster that will collect the traces. For example, Zipkin requires a `collector_cluster` to be specified in the bootstrap config. If your provider requires a cluster to be specified, you can provide it in the config, as shown below.

The bootstrap config is the portion of Envoy's config that is applied when an Envoy process is initialized.
That means that you must either apply this configuration through Helm values during installation or that you must edit the proxy's config map and restart the pod.
We describe both methods below.

{{< tabs >}}
{{< tab name="helm">}}
{{< highlight yaml "hl_lines=4-16" >}}
gatewayProxies:
  gatewayProxy:
    tracing:
      cluster:
        - name: zipkin
          connect_timeout: 1s
          type: STRICT_DNS
          load_assignment:
            cluster_name: zipkin
            endpoints:
            - lb_endpoints:
              - endpoint:
                  address:
                    socket_address:
                      address: zipkin
                      port_value: 9411
{{< /highlight >}}

When you install Gloo Edge using these Helm values, Envoy will be configured with the tracing cluster you specified.
{{< /tab >}}

{{< tab name="configmap">}}

First, edit the config map pertaining to your proxy. This should be `gateway-proxy-envoy-config` in the `gloo-system` namespace.

```bash
kubectl edit configmap -n gloo-system gateway-proxy-envoy-config
```
Apply the tracing cluster changes. 

A sample Zipkin configuration is shown here:

{{< highlight yaml "hl_lines=25-36">}}
apiVersion: v1
kind: ConfigMap
data:
  envoy.yaml:
    node:
      cluster: gateway
      id: "{{.PodName}}{{.PodNamespace}}"
      metadata:
        role: "{{.PodNamespace}}~gateway-proxy"
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
                      address: gloo
                      port_value: 9977
          http2_protocol_options: {}
          type: STRICT_DNS
        - name: zipkin
          connect_timeout: 1s
          type: STRICT_DNS
          load_assignment:
            cluster_name: zipkin
            endpoints:
            - lb_endpoints:
              - endpoint:
                  address:
                    socket_address:
                      address: zipkin
                      port_value: 9411
{{< /highlight >}}

To apply the bootstrap config to Envoy we need to restart the process. An easy way to do this is with `kubectl rollout restart`.

```bash
kubectl rollout restart deployment [deployment_name]
```

When the `gateway-proxy` pod restarts it should have the new trace cluster config.

{{< /tab >}}
{{< /tabs >}}

##### 2. Configure a tracing provider

For a list of supported tracing providers, and the configuration that they expect, please see Envoy's documentation on [trace provider configuration](https://www.envoyproxy.io/docs/envoy/v1.13.1/api-v2/config/trace/v2/trace.proto#config-trace-v2-tracing-http).
For demonstration purposes, we show how to configure a *zipkin* trace provider below.


{{< tabs >}}
{{< tab name="(Preferred) Dynamic Listener">}}

You can enable tracing on a listener-by-listener basis. Please see [the tracing listener docs]({{% versioned_link_path fromRoot="/guides/traffic_management/listener_configuration/http_connection_manager/#tracing" %}}) for details on how to enable tracing on a listener. Note that we have configured a _cluster_ in step 1 which we will refer to by `clusterName`.

{{< /tab >}}
{{< tab name="configmap">}}

First, edit the config map pertaining to your proxy. This should be `gateway-proxy-envoy-config` in the `gloo-system` namespace.

```bash
kubectl edit configmap -n gloo-system gateway-proxy-envoy-config
```
Apply the tracing provider changes. A sample Zipkin configuration is shown below.

{{< highlight yaml "hl_lines=27-34">}}
apiVersion: v1
kind: ConfigMap
data:
  envoy.yaml:
    node:
      cluster: gateway
      id: "{{.PodName}}{{.PodNamespace}}"
      metadata:
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
                - name: envoy.filters.network.http_connection_manager
                  typed_config:
                    "@type": type.googleapis.com/envoy.extensions.filters.network.http_connection_manager.v3.HttpConnectionManager
                    codec_type: AUTO
                    stat_prefix: prometheus
                    route_config: # collapsed for brevity
                    http_filters:
                      - name: envoy.filters.http.router
                    tracing:
                      provider:
                        name: envoy.tracers.zipkin
                        typed_config:
                          "@type": "type.googleapis.com/envoy.config.trace.v2.ZipkinConfig"
                          collector_cluster: zipkin
                          collector_endpoint: "/api/v2/spans"
                          collector_endpoint_version: HTTP_JSON
{{< /highlight >}}


To apply the bootstrap config to Envoy we need to restart the process. An easy way to do this is with `kubectl rollout restart`.

```bash
kubectl rollout restart deployment [deployment_name]
```

When the `gateway-proxy` pod restarts it should have the new trace provider config.

{{% notice note %}}
This provider configuration will only be applied to the static listeners that are defined in the bootstrap config. If you need to support tracing on dynamically created listeners, follow the steps in the "Dynamic Listener" tab.
{{% /notice %}}
{{< /tab >}}
{{< /tabs >}}

##### 3. (Optional) Annotate routes with descriptors

In order to associate a trace with a route, it can be helpful to annotate your routes with a descriptive name. This can be applied to the route, via a route plugin, or provided through a header `x-envoy-decorator-operation`.
If both means are used, the header's value will override the routes's value.

You can set a route descriptor with `kubectl edit virtualservice -n gloo-system [name-of-vs]`.
Edit your virtual service as shown below.

{{< highlight yaml "hl_lines=17-18" >}}
apiVersion: gateway.solo.io/v1
kind: VirtualService
metadata: # collapsed for brevity
spec:
  virtualHost:
    domains:
    - '*'
    routes:
    - matchers:
      - exact: /abc
      routeAction:
        single:
          upstream:
            name: my-upstream
            namespace: gloo-system
      options:
        tracing:
          routeDescriptor: my-route-from-abc-jan-01
        prefixRewrite: /
status: # collapsed for brevity
{{< /highlight >}}

##### 4. (Optional) Setting up zipkin locally
Set up Zipkin tracing in a [local Kind cluster]({{< versioned_link_path fromRoot="/installation/platform_configuration/cluster_setup/#kind" >}}) for local troubleshooting and experimentation. 
1. Run Zipkin.
    ```shell
    docker run --network=kind -itd --name zipkin -p 9411:9411 openzipkin/zipkin
    ```

2. Verify that both `zipkin` and `zipkin-tracing-control-plane` are in your local Kind cluster network.
     ```shell
     docker network inspect kind
     ```
{{< highlight json "hl_lines=35">}}
[
    {
        "Name": "kind",
        "Id": "6a37a4ebb2d0e7dcbabe50dc8b1a519b431f054aebb822ed85e00abde99fd4d3",
        "Created": "2021-09-16T09:28:49.88165506-04:00",
        "Scope": "local",
        "Driver": "bridge",
        "EnableIPv6": true,
        "IPAM": {
            "Driver": "default",
            "Options": {},
            "Config": [
                {
                    "Subnet": "172.18.0.0/16",
                    "Gateway": "172.18.0.1"
                },
                {
                    "Subnet": "fc00:f853:ccd:e793::/64",
                    "Gateway": "fc00:f853:ccd:e793::1"
                }
            ]
        },
        "Internal": false,
        "Attachable": false,
        "Ingress": false,
        "ConfigFrom": {
            "Network": ""
        },
        "ConfigOnly": false,
        "Containers": {
            "3431770d0c41bfbc8eceac4c806605286f5dac81820599f828dcb250037a2f48": {
                "Name": "zipkin-tracing-control-plane",
                "EndpointID": "3e48e18bc7b259ca9d597a594ee3d5205c8339e8ecd9f8f274a178d07f395b78",
                "MacAddress": "02:42:ac:12:00:03",
                "IPv4Address": "172.18.0.3/16",
                "IPv6Address": "fc00:f853:ccd:e793::3/64"
            },
            "84dadbd86f113c7104eca23d3d78e9dec997a47666c1ba4eed2ae7a5ad8eb20d": {
                "Name": "zipkin",
                "EndpointID": "09e07c8ac6b1cd912c325962586d9497520e216a4ec357384c663594248fc104",
                "MacAddress": "02:42:ac:12:00:02",
                "IPv4Address": "172.18.0.2/16",
                "IPv6Address": "fc00:f853:ccd:e793::2/64"
            }
        },
        "Options": {
            "com.docker.network.bridge.enable_ip_masquerade": "true",
            "com.docker.network.driver.mtu": "1500"
        },
        "Labels": {}
    }
]
{{< /highlight >}}

3. Configure a [tracing cluster]({{< versioned_link_path fromRoot="/guides/observability/tracing/#1-configure-a-tracing-cluster" >}}) with the IP address of Zipkin, such as `172.18.0.2` in this example.

4. Navigate to the zipkin interface at http://localhost:9411 to visualize traces:

![Zipkin UI]({{% versioned_link_path fromRoot="/img/zipkin.png" %}})