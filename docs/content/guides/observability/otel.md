---
title: 
weight: 5
description: 
---

OpenTelemetry (OTel)

Why use otel?
- If i want to set up distributed tracing and metrics in Gloo Edge, you currently must set up a zipkin agent to reporter for all mircoservices, and set up a collector for it. These agents and reporters are tightly coupled, and it’s difficult if you want to have some sort of heterogeneity in your setup with multiple vendors.
- OTel provides a standardized protocol for reporting traces to the collector, and a standardized collector through which to collect info. Provides export of metrics to many types of services (not just Zipkin, Jaeger, and Datadog) listed here: https://github.com/open-telemetry/opentelemetry-collector-contrib/tree/main/exporter

Maybe an option within the existing tracing docs - instead of taking from the workloads directly, taking it from the otel collector pods instead.

Add version requirement for 1.13

1. Download the [otel-config.yaml](otel-config.yaml) file, which contains the configmaps, daemonset, deployment, and service for OTel collector agents. You can optionally check out the contents to see the OTel collector configuration. For example, in the `otel-collector-conf` configmap that begins on line 92, the `data.otel-agent-config.receivers` section enables gRPC and HTTP protocols for data collection. The `data.otel-agent-config.exporters` section enables logging data to Zipkin for tracing and to the Edge console and the echo server for debugging. For more information about this configuration, see the [OTel documentation](https://opentelemetry.io/docs/collector/configuration/).
   ```sh
   cd ~/Downloads
   open otel-config.yaml
   ```

2. Install the OTel collectors into your cluster.
   ```
   kubectl apply -n gloo-system -f otel-config.yaml
   ```

3. Verify that the OTel collector agents are deployed in your cluster. Because the agents are deployed as a daemonset, the number of metrics collector agent pods equals the number of worker nodes in your cluster.
   ```
   kubectl get pods -n gloo-system
   ```
   Example output:
   ```
   NAME                                 READY   STATUS    RESTARTS      AGE
   ...
   gloo-metrics-collector-agent-5cwn5   1/1     Running   0             107s
   gloo-metrics-collector-agent-7czjb   1/1     Running   0             107s
   gloo-metrics-collector-agent-jxmnv   1/1     Running   0             107s
   ```

4. Install Zipkin, which receives tracing data from the Zipkin exporter in your OTel setup.
   ```
   kubectl -n gloo-system create deployment --image openzipkin/zipkin zipkin
   kubectl -n gloo-system expose deployments/zipkin --port 9411 --target-port 9411
   ```

5. Deploy `echo-server`, a simple HTTP server to help test your tracing setup.
   ```yaml
   kubectl -n gloo-system apply -f- <<EOF
   apiVersion: apps/v1
   kind: Deployment
   metadata:
     name: echo-server
     namespace: gloo-system
     labels:
       app: echo-server
   spec:
     replicas: 1
     selector:
       matchLabels:
         app: echo-server
     template:
       metadata:
         labels:
           app: echo-server
       spec:
         containers:
         - name: echo-server
           image: jmalloc/echo-server
           ports:
           - containerPort: 8080
           env:
           - name: LOG_HTTP_HEADERS
             value: "true"
           - name: LOG_HTTP_BODY
             value: "true"
   ---
   apiVersion: v1
   kind: Service
   metadata:
     name: echo-server
     namespace: gloo-system
     labels:
       app: echo-server
   spec:
     ports:
     - name: echo-server
       port: 8080
       protocol: TCP
       targetPort: 8080
     selector:
       app: echo-server
   EOF
   ```

6. Create the following Gloo Edge `Upstream`, `Gateway`, and `VirtualService` custom resources. 
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
     # kube:
     #   selector:
     #     app: otel-collector
     #   serviceName: otel-collector
     #   serviceNamespace: gloo-system
     #   servicePort: 4317
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

7. In four separate terminals, port-forward and view logs for the deployed services.
   1. Port-forward the gateway proxy on port 8080.
      ```
      kubectl -n gloo-system port-forward deployments/gateway-proxy 8080
      ```
   2. Port-forward the Zipkin service on port 9411.
      ```
      kubectl -n gloo-system port-forward deployments/zipkin 9411
      ```
   3. Open the logs for the echo server.
      ```sh
      kubectl -n gloo-system logs deployments/echo-server -f
      ```
   4. Open the logs for the OTel collector.
      ```sh
      kubectl -n gloo-system logs deployments/otel-collector -f
      ```

8. In your original terminal, send a request to `http://localhost:8080`.
   ```sh
   curl http://localhost:8080
   ```

9.  In the echo server logs, notice the response from OTel that was printed to the log.

10. In the OTel collector logs, notice the trace that was printed to the log.

11. Open the Zipkin web interface.
    ```sh
    open http://localhost:9411/zipkin/
    ```
12. In the Zipkin web interface, click **Run query** to view the trace for your request.
