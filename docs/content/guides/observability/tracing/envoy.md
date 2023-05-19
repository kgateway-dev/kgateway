---
title: Default Envoy tracing
weight: 2
description: Configure Gloo Edge with default Envoy tracing capabilities.
---

With Gloo Edge, you can use Envoy's end-to-end tracing capabilities to obtain visibility and track requests as they pass through your API gateway to distributed backends, such as services, databases, or other endpoints in your ecosystem. 

To get started, enable the default Envoy distributed tracing in your Gloo Edge installation to trace requests, analyze service dependencies, and find bottlenecks or high latency services. Then, use a tracing provider to collect and visualize the sampled spans. The following distributed tracing platforms are supported for the default Envoy metrics collection in Gloo Edge: 
- [Zipkin](https://zipkin.io/)
- [Jaeger](https://www.jaegertracing.io/)
- [Datadog](https://docs.datadoghq.com/getting_started/tracing/)

{{% notice note %}} 
This guide uses the Zipkin tracing platform as an example to show how to set up tracing in Gloo Edge. To set up other tracing platforms, refer to the platform-specific documentation or the [Envoy tracing provider documentation](https://www.envoyproxy.io/docs/envoy/v1.21.1/api-v3/config/trace/v3/http_tracer.proto#config-trace-v3-tracing-http).
{{% /notice %}}

## How does it work? 

To trace a request, data must be captured from the moment the request is initiated and every time the request is forwarded to another endpoint, or when other microservices are called along the way. When a request is initiated, a trace ID and an initial span (parent span) is created. A span represents an operation that is performed on your request, such as an API call, a database lookup, or a call to an external service. If a request is sent to a service, a child span is created in the trace capturing all the operations that are performed within the service. 

Each operation and span is documented with a timestamp so that you can easily see how long a request was processed by a specific endpoint in your trace. Most tracing platforms have support to visualize the tracing information in a graph so that you can easily see bottlenecks in your microservices stack. 

To configure a tracing platform, you must update the Envoy bootstrap configuration. The bootstrap configuration is automatically applied when an Envoy process is initialized. To update the bootstrap configuration, you can use one of the following ways: 
- **Gloo Edge**: Configure the tracing platform in the installation Helm chart template or in the Gloo Edge custom resources, and let Gloo Edge determine how to best apply the configuration in Envoy. 
- **Manually update Envoy**: Use a Kubernetes configmap and provide the Envoy code that you want to apply. You then manually restart all the deployments where you want to apply the updated Envoy configuration.

## Set up Zipkin tracing

To set up Zipkin tracing, you follow these general steps: 

1. [Set up Zipkin locally](#setup).
2. [Configure the Zipkin tracing cluster in Gloo Edge](#cluster). 
3. [Configure Zipkin as the tracing provider for a listener](#provider).
4. [Optional: Annotate routes with descriptors](#annotations). 
5. [Initiate a request and view traces](#traces).

### Step 1. Set up Zipkin locally {#setup}
Set up Zipkin tracing in a [local Kind cluster]({{< versioned_link_path fromRoot="/installation/platform_configuration/cluster_setup/#kind" >}}) for local troubleshooting and experimentation. 
1. Run Zipkin.
   ```shell
   docker run --network=kind -itd --name zipkin -p 9411:9411 openzipkin/zipkin
   ```

2. Verify that both `zipkin` and `zipkin-tracing-control-plane` are in your local Kind cluster network, and note the `zipkin` container IP address without the CIDR. In the following example, the `zipkin` container IP address is `172.xx.x.2`.
   ```shell
   docker network inspect kind
   ```
   {{< highlight json "hl_lines=35 42">}}
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
                       "Subnet": "172.xx.x.0/16",
                       "Gateway": "172.xx.x.1"
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
                   "IPv4Address": "172.xx.x.3/16",
                   "IPv6Address": "fc00:f853:ccd:e793::3/64"
               },
               "84dadbd86f113c7104eca23d3d78e9dec997a47666c1ba4eed2ae7a5ad8eb20d": {
                   "Name": "zipkin",
                   "EndpointID": "09e07c8ac6b1cd912c325962586d9497520e216a4ec357384c663594248fc104",
                   "MacAddress": "02:42:ac:12:00:02",
                   "IPv4Address": "172.xx.x.2/16",
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

3. Configure the Zipkin [tracing cluster](#cluster) with the IP address that was assigned in the previous step. In this example, the Zipkin cluster is assigned the `172.18.0.2` IP address.

4. [Open Zipkin on your local machine](http://localhost:9411). 

   ![Zipkin UI]({{% versioned_link_path fromRoot="/img/zipkin.png" %}})

### Step 2. Configure the Zipkin tracing cluster in Gloo Edge {#cluster}

Zipkin uses a dedicated tracing cluster where tracing information is sent to. The name of the tracing cluster must be set in the Envoy bootstrap configuration for Envoy to know where to send the information to. The following example shows how you can configure the Zipkin tracing cluster by using Gloo Edge or updating the Envoy bootstrap configuration directly. 


**Option 1: Install Gloo Edge with Zipkin tracing**

Use the Gloo Edge installation Helm chart template to configure the Zipkin tracing platform. Gloo Edge automatically determines the updates that must be made to apply the Zipkin configuration in your Envoy proxies. 

1. Create a `values.yaml` file and add your Zipkin configuration. In the following example, the Zipkin cluster is called `zipkin`. Replace `<zipkin_container_IP_address>` with the value that you retrieved in [Step 1: Set up Zipkin locally](#setup), such as `172.xx.x.2`.

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
                         address: <zipkin_container_IP_address>
                         port_value: 9411
   {{< /highlight >}}
   
2. Install Gloo Edge with your Zipkin configuration.   
   ```shell
   helm install gloo gloo/gloo --namespace gloo-system --create-namespace -f values.yaml
   ```

**Option 2: Update the Envoy configmap directly**

Add the Envoy code that you want to apply to a Kubernetes configmap and restart the proxy deployments. 

1. Edit the Envoy proxy configuration. 

   ```bash
   kubectl edit configmap -n gloo-system gateway-proxy-envoy-config
   ```
   
2. Enter the Zipkin tracing changes. Replace `<zipkin_container_IP_address>` with the value that you retrieved in [Step 1: Set up Zipkin locally](#setup), such as `172.xx.x.2`.

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
                         address: <zipkin_container_IP_address>
                         port_value: 9411
   {{< /highlight >}}

3. Apply the updated Envoy config. For Envoy to pick up the new config, you need to restart the Envoy proxy deployment.  

   ```bash
   kubectl rollout restart deployment gateway-proxy
   ```

### Step 3. Configure Zipkin as the tracing provider for a listener {#provider}

After you configure the [tracing cluster](#cluster), you can now set Zipkin as the tracing platform for a listener in your Gloo Edge gateway. To do that, you can either update the Gloo Edge gateway, or provide the Envoy code in a Kubernetes configmap and apply this configmap by manually restarting the Envoy proxies.

{{% notice note %}}
When you choose to manually update the Envoy proxies with a configmap, you can apply the updated configuration to a static listener that is defined in the Envoy bootstrap config only. If you want to configure a tracing provider for dynamically created listeners, you must update the gateway in Gloo Edge. 
{{% /notice %}}

**Option 1: Dynamic listeners with Gloo Edge**

You can enable tracing on a listener-by-listener basis. To find an example tracing listener configuration for your gateway, see [the tracing listener docs]({{% versioned_link_path fromRoot="/guides/traffic_management/listener_configuration/http_connection_manager/#tracing" %}}). In this example, the Zipkin cluster that you created in step 1 is referenced in the `clusterName` field. 

**Option 2: Static listeners with configmaps**

1. Edit the Envoy proxy configuration. 

   ```bash
   kubectl edit configmap -n gloo-system gateway-proxy-envoy-config
   ```
   
2. Enter the tracing provider changes. 

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
                             "@type": "type.googleapis.com/envoy.config.trace.v3.ZipkinConfig"
                             collector_cluster: zipkin
                             collector_endpoint: "/api/v2/spans"
                             collector_endpoint_version: HTTP_JSON
   {{< /highlight >}}

3. Apply the updated Envoy config. For Envoy to pick up the new config, you need to restart the Envoy proxy deployment.  

   ```bash
   kubectl rollout restart deployment [deployment_name]
   ```

{{% notice note %}}
This provider configuration will only be applied to the static listeners that are defined in the bootstrap config. If you need to support tracing on dynamically created listeners, see `Option 1: Dynamic listeners with Gloo Edge`.
{{% /notice %}}

### Step 4. Optional: Annotate routes with tracing descriptors {#annotations}

In order to associate a trace with a route, it can be helpful to annotate your routes with a descriptive name. You can add the name to the virtual service directly, or use the `x-envoy-decorator-operation` Envoy header in your request. If a name is set in both, the name in the header takes precedence.

The following steps show how to add the name to the virtual service in Gloo Edge. 

1. List the virtual services in the `gloo-system` namespace and select the one that you want to edit. 
   ```bash
   kubectl get virtualservice -n gloo-system
   ```
   
2. Edit the virtual service. 
   ```bash
   kubectl edit virtualservice -n gloo-system <virtual-service-name>
   ```
   
3. Enter the name for the route that you want to associate your trace with in the `routeDescriptor` field. 

   {{< highlight yaml "hl_lines=17-18" >}}
   apiVersion: gateway.solo.io/v1
   kind: VirtualService
   metadata: # omitted for brevity
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
             routeDescriptor: <route-descriptor-name>
           prefixRewrite: /
   status: # omitted for brevity
   {{< /highlight >}}


### Step 5. Initiate a request and view traces {#traces}

1. Send a request to your app. 
   ```shell
   curl localhost:31500/abc
   ```

2. [Open Zipkin on your local machine](http://localhost:9411) and review the traces. 
