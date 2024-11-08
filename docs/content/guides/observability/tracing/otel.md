---
title: OpenTelemetry tracing
weight: 1
description: Leverage OpenTelementry tracing capabilities in Gloo Gateway.
---

Enable [OpenTelemetry](https://opentelemetry.io/) (OTel) tracing capabilities to obtain visibility and track requests as they pass through your API gateway to distributed backends.

OTel provides a standardized protocol for reporting traces, and a standardized collector through which to recieve metrics. Additionally, OTel supports exporting metrics to several types of distributed tracing platforms. For the full list of supported platforms, see the [OTel GitHub respository](https://github.com/open-telemetry/opentelemetry-collector-contrib/tree/main/exporter).

## Set up OpenTelemetry tracing

To get started, deploy an OTel collector and agents to your Gloo Gateway cluster to trace requests, and modify your gateway proxy with the OTel tracing configuration. Then, use a tracing provider to collect and visualize the sampled spans.

{{% notice note %}}
The OTel integration is supported as a beta feature in Gloo Gateway 1.13.0 and later.
</br></br>
This guide uses the Zipkin tracing platform as an example to show how to set up tracing with OTel in Gloo Gateway. To set up other tracing platforms, refer to the platform-specific documentation.
{{% /notice %}}

**Before you begin**: Create or update your Gloo Gateway installation to version 1.13.0 or later.

1. Download the [otel-config.yaml](../otel-config.yaml) file, which contains the configmaps, daemonset, deployment, and service for the OTel collector and agents. You can optionally check out the contents to see the OTel collector configuration.
   * For example, in the `otel-collector-conf` configmap that begins on line 92, the `data.otel-agent-config.receivers` section enables gRPC and HTTP protocols for data collection. The `data.otel-agent-config.exporters` section enables logging data to Zipkin for tracing and to the Gloo Gateway console for debugging.
   * In the `otel-collector` deployment, you can comment out the ports that begin on line 194 so that only the tracing platform you want to use is enabled, such as Zipkin for this guide.
   * For more information about this configuration, see the [OTel documentation](https://opentelemetry.io/docs/collector/configuration/). For more information and examples about the exporters you can configure, see the [OTel GitHub repo](https://github.com/open-telemetry/opentelemetry-collector-contrib/tree/main/exporter).
   ```sh
   cd ~/Downloads
   open otel-config.yaml
   ```

2. Install the OTel collector and agents into your cluster.
   ```
   kubectl apply -n gloo-system -f otel-config.yaml
   ```

3. Verify that the OTel collector and agents are deployed in your cluster. Because the agents are deployed as a daemonset, the number of agent pods equals the number of worker nodes in your cluster.
   ```
   kubectl get pods -n gloo-system
   ```
   Example output:
   ```
   NAME                              READY   STATUS    RESTARTS   AGE
   discovery-db9fbdd-4wsg8           1/1     Running   0          3h
   gateway-proxy-b5995db59-bvl9d     1/1     Running   0          3h
   gloo-56c78bb857-6v7vz             1/1     Running   0          3h
   otel-agent-dpdmd                  3/3     Running   0          35s
   otel-collector-64d8c966c5-ptpfn   1/1     Running   0          35s
   ```

4. Install Zipkin, which receives tracing data from the Zipkin exporter in your OTel setup.
   ```
   kubectl -n gloo-system create deployment --image openzipkin/zipkin zipkin
   kubectl -n gloo-system expose deployments/zipkin --port 9411 --target-port 9411
   ```

5. Create the following Gloo Gateway `Upstream`, `Gateway`, and `VirtualService` custom resources. 
   * The `Upstream` defines the OTel network address and port that Envoy reports data to.
   * The `Gateway` resource modifies your default HTTP gateway proxy with the OTel tracing configuration, which references the OTel upstream.
   * The `VirtualService` defines a direct response action so that requests to the `/` path respond with `hello world` for testing purposes.
   ```yaml
   kubectl apply -f- <<EOF
   apiVersion: gloo.solo.io/v1
   kind: Upstream
   metadata:
     name: "opentelemetry-collector"
     namespace: gloo-system
   spec:
     # REQUIRED FOR OPENTELEMETRY COLLECTION
     useHttp2: true
     static:
       hosts:
         - addr: "otel-collector"
           port: 4317
   ---
   apiVersion: gateway.solo.io/v1
   kind: Gateway
   metadata:
     labels:
       app: gloo
     name: gateway-proxy
     namespace: gloo-system
   spec:
     bindAddress: '::'
     bindPort: 8080
     httpGateway:
       options:
         httpConnectionManagerSettings:
           tracing:
             openTelemetryConfig:
               collectorUpstreamRef:
                 namespace: "gloo-system"
                 name: "opentelemetry-collector"
   ---
   apiVersion: gateway.solo.io/v1
   kind: VirtualService
   metadata:
     name: default
     namespace: gloo-system
   spec:
     virtualHost:
       domains:
         - '*'
       routes:
         - matchers:
            - prefix: /
           directResponseAction:
             status: 200
             body: 'hello world'
   EOF
   ```

6. In three separate terminals, port-forward and view logs for the deployed services.
   1. Port-forward the gateway proxy on port 8080.
      ```
      kubectl -n gloo-system port-forward deployments/gateway-proxy 8080
      ```
   2. Port-forward the Zipkin service on port 9411.
      ```
      kubectl -n gloo-system port-forward deployments/zipkin 9411
      ```
   3. Open the logs for the OTel collector.
      ```sh
      kubectl -n gloo-system logs deployments/otel-collector -f
      ```

7. In your original terminal, send a request to `http://localhost:8080`.
   ```sh
   curl http://localhost:8080
   ```

8. In the OTel collector logs, notice the trace for your request that was printed to the log, such as the following example.
   ```
   2023-01-13T17:20:20.907Z	info	TracesExporter	{"kind": "exporter", "data_type": "traces", "name": "logging", "#spans": 1}
   2023-01-13T17:20:20.907Z	info	ResourceSpans #0
   Resource SchemaURL: 
   ScopeSpans #0
   ScopeSpans SchemaURL: 
   InstrumentationScope  
   Span #0
       Trace ID       : 64dbb2328b98cc8d74dcc9be575ff8cb
       Parent ID      : 
       ID             : 53e50df3bd752d67
       Name           : ingress
       Kind           : Server
       Start time     : 2023-01-13 17:20:20.479033 +0000 UTC
       End time       : 2023-01-13 17:20:20.479216 +0000 UTC
       Status code    : Unset
       Status message : 
   Attributes:
        -> node_id: Str(gateway-proxy-b5995db59-bvl9d.gloo-system)
        -> zone: Str()
        -> guid:x-request-id: Str(b843b297-848a-95f0-b824-099a521f5c84)
        -> http.url: Str(http://localhost:8080/)
        -> http.method: Str(GET)
        -> downstream_cluster: Str(-)
        -> user_agent: Str(curl/7.79.1)
        -> http.protocol: Str(HTTP/1.1)
        -> peer.address: Str(127.0.0.1)
        -> request_size: Str(0)
        -> response_size: Str(11)
        -> component: Str(proxy)
        -> http.status_code: Str(200)
        -> response_flags: Str(-)
   	{"kind": "exporter", "data_type": "traces", "name": "logging"}
    ```

    Note that the `Status code` in the `Span` of the above trace is `Unset`. This is the recommended value in accordance with the OpenTelemetry [semantic conventions](https://opentelemetry.io/docs/specs/semconv/http/http-spans/), which state:

    > Span Status MUST be left unset if HTTP status code was in the 1xx, 2xx or 3xx ranges, unless there was another error (e.g., network error receiving the response body; or 3xx codes with max redirects exceeded), in which case status MUST be set to Error.
    > ... For HTTP status codes in the 5xx range, as well as any other code the client failed to interpret, span status MUST be set to Error.

    To observe this behavior, update the `status` that is returned by the `directResponseAction` to `500`. Subsequent requests that are sent to this endpoint receive a `Status code: Error` instead of `Status code: Unset`.

9. [Open the Zipkin web interface.](http://localhost:9411/zipkin/)

10. In the Zipkin web interface, click **Run query** to view traces for your requests, and click **Show** to review the details of the trace.

## Customize the span name

Gloo supports changing the default span name by using the transformation filter. The following steps show an example name change.

1. Change the span name by modifying your Virtual Service. The following sample configuration in a Virtual Service changes the name to the host header in the `text: '{{header("Host")}}'` field. Note because the `spanTransformer.name` field is an Inja template, you can use header values and any other macro logic that is supported in transformations. For more information, see the [transformation documentation]({{< versioned_link_path fromRoot="/guides/traffic_management/request_processing/transformations/" >}}).
   {{< highlight yaml "hl_lines=16-24" >}}
   apiVersion: gateway.solo.io/v1
   kind: VirtualService
   metadata:
     name: default
     namespace: gloo-system
   spec:
     virtualHost:
       domains:
         - '*'
       routes:
         - matchers:
            - prefix: /
           directResponseAction:
             status: 200
             body: 'hello world'
       options:
         stagedTransformations:
           regular:
             requestTransforms:
               - requestTransformation:
                   transformationTemplate:
                     spanTransformer:
                       name:
                         text: '{{header("Host")}}'
   {{< /highlight >}}

2. After you apply the updated Virtual Service, you can verify that the span name in the OpenTelemetry collector now is set to the value of the `Host` header by sending a curl request.
   ```sh
   curl -H "Host: my-hostname" http://localhost:8080
   ```
   In the output, verify that the following span appears:
   ```
   2024-11-04T20:18:08.656Z	info	TracesExporter	{"kind": "exporter", "data_type": "traces", "name": "logging", "#spans": 1}
   2024-11-04T20:18:08.656Z	info	ResourceSpans #0
   Resource SchemaURL: 
   Resource attributes:
        -> service.name: Str(gateway-proxy)
   ScopeSpans #0
   ScopeSpans SchemaURL: 
   InstrumentationScope  
   Span #0
       Trace ID       : e42f4fa3ba873eefeb26b3b16112dc18
       Parent ID      : 
       ID             : 21547202617791ee
       Name           : my-hostname
       Kind           : Server
       Start time     : 2024-11-04 20:18:06.697813 +0000 UTC
       End time       : 2024-11-04 20:18:06.699024 +0000 UTC
       Status code    : Unset
       Status message : 
   Attributes:
        -> node_id: Str(gateway-proxy-577544cdcd-c6rvm.gloo-system)
        -> zone: Str()
        -> guid:x-request-id: Str(d1c5b217-e87b-95ae-b449-56c40eaa879c)
        -> http.url: Str(http://echo-server/)
        -> http.method: Str(GET)
        -> downstream_cluster: Str(-)
        -> user_agent: Str(curl/7.88.1)
        -> http.protocol: Str(HTTP/1.1)
        -> peer.address: Str(127.0.0.1)
        -> request_size: Str(0)
        -> response_size: Str(313)
        -> component: Str(proxy)
        -> upstream_cluster: Str(echo-server_gloo-system)
        -> upstream_cluster.name: Str(echo-server_gloo-system)
        -> http.status_code: Str(200)
        -> response_flags: Str(-)
   	{"kind": "exporter", "data_type": "traces", "name": "logging"}
   ```

Note that in this example, the span name was modified at the level of the virtual host, so all routes under the virtual host will exhibit the same span naming pattern. However, it is also possible to override this logic on a _per-route_ basis using the `routeDescriptor` field:

```
routes:
- matchers:
   - prefix: /route2
  options:
    autoHostRewrite: true
    tracing:
      routeDescriptor: CUSTOM_ROUTE_DESCRIPTOR
  routeAction:
    single:
      upstream:
        name: echo-server
        namespace: gloo-system
```

When this configuration is applied, requests for the `/route2` endpoint will result in spans being reported to the OpenTelemetry collector with the span name set to `"CUSTOM_ROUTE_DESCRIPTOR"`. However, it is also important to note that `routeDescriptor` can only set a static override value and does not support Inja transformation templates.
