---
title: Tracing Setup
weight: 4
description: Configure Gloo Edge for tracing
---

## Tracing

Gloo Edge makes it easy to implement tracing on your system through [Envoy's tracing capabilities](https://www.envoyproxy.io/docs/envoy/latest/intro/arch_overview/observability/tracing.html).

#### Usage

*If you have not yet enabled tracing, please see the [configuration](#configuration) details below.*

- Produce a trace by passing the header: `x-client-trace-id`
  - This id provides a means of associating the spans produced during a trace. The value must be unique, a uuid4 is [recommended](https://www.envoyproxy.io/docs/envoy/v1.9.0/configuration/http_conn_man/headers#config-http-conn-man-headers-x-client-trace-id).
- Optionally annotate your trace with the `x-envoy-decorator-operation` header.
  - This will be emitted with the resulting trace and can be a means of identifying the origin of a given trace. Note that it will override any pre-specified route decorator. Additional details can be found [here](https://www.envoyproxy.io/docs/envoy/v1.11.2/configuration/http_filters/router_filter#config-http-filters-router-x-envoy-decorator-operation).

#### Configuration

There are two steps to make tracing available through Gloo Edge:
1. Specify a trace provider in the bootstrap config
1. Enable tracing on the listener
1. (Optional) Annotate routes with descriptors

##### 1. Specify a tracing provider in the bootstrap config

The bootstrap config is the portion of Envoy's config that is applied when an Envoy process in intialized.
That means that you must either apply this configuration through Helm values during installation or that you must edit the proxy's config map and restart the pod.
We describe both methods below.

Several tracing providers are supported.
You can choose any that is supported by Envoy.
For a list of supported tracing providers and the configuration that they expect, please see Envoy's documentation on [trace provider configuration](https://www.envoyproxy.io/docs/envoy/v1.13.1/api-v2/config/trace/v2/trace.proto#config-trace-v2-tracing-http).
For demonstration purposes, we show how to specify the helm values for a *zipkin* trace provider below.

Note: some tracing providers, such as Zipkin, require a `collector_cluster` (the cluster which collects the traces) to be specified in the bootstrap config. If your provider requires a cluster to be specified, you can provide it in the config, as shown below. If your provider does not require a cluster you should omit that field. 

**Option 1: Set the trace provider through helm values:**

{{< highlight yaml "hl_lines=3-23" >}}
gatewayProxies:
  gatewayProxy:
    tracing:
      provider:
        name: envoy.tracers.zipkin
        typed_config:
          "@type": "type.googleapis.com/envoy.config.trace.v2.ZipkinConfig"
          collector_cluster: zipkin
          collector_endpoint: "/api/v2/spans"
          collector_endpoint_version: HTTP_JSON
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
                      port_value: 1234
{{< /highlight >}}

When you install Gloo Edge using these Helm values, Envoy will be configured with the tracing provider you specified.

**Option 2: Set the trace provider by editing the config map:**

First, edit the config map pertaining to your proxy. This should be `gateway-proxy-envoy-config` in the `gloo-system` namespace.

```bash
kubectl edit configmap -n gloo-system gateway-proxy-envoy-config
```
Apply the tracing provider changes. A sample Zipkin configuration is shown below.

{{< highlight yaml "hl_lines=27-34 49-60">}}
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
                      port_value: 1234
{{< /highlight >}}


To apply the bootstrap config to Envoy we need to restart the process. An easy way to do this is with `kubectl delete pod`.

```bash
kubectl delete pod -n gloo-system gateway-proxy-[suffix]
```

When the `gateway-proxy` pod restarts it should have the new trace provider config.

##### 2. Enable tracing on the listener

After you have installed Gloo Edge with a tracing provider, you can enable tracing on a listener-by-listener basis. Gloo Edge exposes this feature through a listener plugin. Please see [the tracing listener plugin docs]({{% versioned_link_path fromRoot="/guides/traffic_management/listener_configuration/http_connection_manager/#tracing" %}}) for details on how to enable tracing on a listener.

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
