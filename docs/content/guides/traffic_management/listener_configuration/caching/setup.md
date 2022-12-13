---
title: Set up caching
weight: 50
description: Deploy the caching server and start caching responses from Upstream services. 
---

Set up the Gloo Edge caching server to cache responses from upstream services for quicker response times.

{{% notice note %}}
This feature is available only for Gloo Edge Enterprise v1.12.x and later.
{{% /notice %}}

When you enable caching during installation, the caching server deployment is automatically created for you and is managed by Gloo Edge. Then, you must configure an HTTP or HTTPS listener to cache responses for its upstream services. When the listener routes a request to an upstream, the response from the upstream is automatically cached for one hour. All subsequent requests receive the cached response.

## Deploy the caching server

Create a caching server during Gloo Edge Enterprise installation time, and specify any Redis overrides. 

1. [Install Gloo Edge Enterprise version 1.12.x or later by using Helm]({{% versioned_link_path fromRoot="/installation/enterprise/#customizing-your-installation-with-helm" %}}). In your `values.yaml` file, specify the following settings:
   * Caching server: Set `global.extensions.caching.enabled: true` to enable the caching server deployment.
   * Redis overrides: By default, the caching server uses the Redis instance that is deployed with Gloo Edge. To use your own Redis instance, such as in production deployments:
     * Set `redis.disabled` to `true` to disable the default Redis instance.
     * Set `redis.service.name` to the name of the Redis service instance. If the instace is an external service, set the endpoint of the external service as the value.
     * For other Redis override settings, see the Redis section of the [Enterprise Helm chart values]({{% versioned_link_path fromRoot="/reference/helm_chart_values/enterprise_helm_chart_values/" %}}).

2. Verify that the caching server is deployed.
   ```sh
   kubectl --namespace gloo-system get all | grep caching
   ```
   Example output:
   ```
   pod/caching-service-5d7f867cdc-bhmqp                  1/1     Running   0          74s
   service/caching-service                       ClusterIP      10.76.11.242   <none>          8085/TCP                                               77s
   deployment.apps/caching-service                       1/1     1            1           77s
   replicaset.apps/caching-service-5d7f867cdc            1         1         1       76s
   ```

3. Optional: You can also check the debug logs to verify that caching is enabled.
   ```sh
   glooctl debug logs
   ```
   Search the output for `caching` to verify that you have log entries similar to the following:
   ```json
   {"level":"info","ts":"2022-08-02T20:47:30.057Z","caller":"radix/server.go:31","msg":"Starting our basic redis caching service","version":"1.12.0"}
   {"level":"info","ts":"2022-08-02T20:47:30.057Z","caller":"radix/server.go:35","msg":"Created redis pool for caching","version":"1.12.0"}
   ```

<!-- future work
## Configure settings for the caching server

should be able to configure general settings for the server in the future, like the default caching time

https://docs.solo.io/gloo-edge/master/reference/api/github.com/solo-io/gloo/projects/gloo/api/v1/enterprise/options/caching/caching.proto.sk/#settings
-->

## Configure caching for a listener

Configure your gateway to cache responses for all upstreams that are served by a listener. Enabling caching for a specific upstream is currently not supported.

1. Edit the Gateway CRD where your listener is defined.
   ```sh
   kubectl edit gateway -n gloo-system gateway-proxy
   ```

2. Specify the caching server in the `httpGateway.options` section. 
   {{< highlight yaml "hl_lines=11-16" >}}
   apiVersion: gateway.solo.io/v1
   kind: Gateway
   metadata:
     name: gateway-proxy
     namespace: gloo-system
   spec:
     bindAddress: ‘::’
     bindPort: 8080
     proxyNames:
     - gateway-proxy
     httpGateway:
       options:
         caching:
           cachingServiceRef:
             name: caching-service
             namespace: gloo-system
   {{< /highlight >}}

<!-- future work: define matchers to specify which paths should be cached -->

## Verify response caching with httpbin

In the following example, the `httpbin` app is used to show how response caching works with Gloo Edge Enterprise. 

1. Deploy `httpbin`. 
   ```shell
   kubectl create ns httpbin
   kubectl -n httpbin apply -f https://raw.githubusercontent.com/solo-io/gloo-mesh-use-cases/main/policy-demo/httpbin.yaml
   ```
   
2. Verify that the app is up and running. 
   ```shell
   kubectl get pods -n httpbin
   ```
   
   Example output: 
   ```
   httpbin-847f64cc8d-9kz2d   1/1     Running   0          35s
   ```
   
3. Create a virtual service for the `httpbin` app. 
   1. Get the name of the upstream that was created automatically for your `httpbin` service. 
      ```shell
      kubectl get upstreams -A | grep httpbin
      ```
      
   2. Set the routing rules for the `httpbin` by creating a virtual service. 
      ```
      kubectl apply -f- <<EOF
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
            routeAction:
              single:
                upstream:
                  name: httpbin-httpbin-8000
                  namespace: gloo-system
      EOF
      ```
   
4. Curl the `httpbin` app and verify that you get back a 200 HTTP response. 
   ```shell
   curl -vik "$(glooctl proxy url)/status/200"
   ```
   
5. Try out caching without response validation. 
      
