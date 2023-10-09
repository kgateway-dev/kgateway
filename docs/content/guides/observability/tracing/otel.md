---
title: OpenTelemetry tracing
weight: 1
description: Leverage OpenTelementry tracing capabilities in Gloo Edge.
---

Enable [OpenTelemetry](https://opentelemetry.io/) (OTel) tracing capabilities to obtain visibility and track requests as they pass through your API gateway to distributed backends.

OTel provides a standardized protocol for reporting traces, and a standardized collector through which to recieve metrics. Additionally, OTel supports exporting metrics to several types of distributed tracing platforms. For the full list of supported platforms, see the [OTel GitHub respository](https://github.com/open-telemetry/opentelemetry-collector-contrib/tree/main/exporter).

To get started, deploy an OTel collector and agents to your Gloo Edge cluster to trace requests, and modify your gateway proxy with the OTel tracing configuration. Then, use a tracing provider to collect and visualize the sampled spans.

{{% notice note %}}
The OTel integration is supported as a beta feature in Gloo Edge 1.13.0 and later.
</br></br>
This guide uses the Zipkin tracing platform as an example to show how to set up tracing with OTel in Gloo Edge. To set up other tracing platforms, refer to the platform-specific documentation.
{{% /notice %}}

**Before you begin**: Create or update your Gloo Edge installation to version 1.13.0 or later.

1. Download the [otel-config.yaml](../otel-config.yaml) file, which contains the configmaps, daemonset, deployment, and service for the OTel collector and agents. You can optionally check out the contents to see the OTel collector configuration.
   * For example, in the `otel-collector-conf` configmap that begins on line 92, the `data.otel-agent-config.receivers` section enables gRPC and HTTP protocols for data collection. The `data.otel-agent-config.exporters` section enables logging data to Zipkin for tracing and to the Edge console for debugging.
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

5. Create the following Gloo Edge `Upstream`, `Gateway`, and `VirtualService` custom resources. 
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

9. [Open the Zipkin web interface.](http://localhost:9411/zipkin/)

10. In the Zipkin web interface, click **Run query** to view traces for your requests, and click **Show** to review the details of the trace.