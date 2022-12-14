---
title: Set up caching
weight: 50
description: Deploy the caching server and start caching responses from upstream services. 
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
   
5. Try out caching without response validation by using the `/cache/{value}` endpoint of the `httpbin` app. 
   1. Send a request to the `/cache/{value}` endpoint. The `{value}` variable determines the number of seconds you want to cache the response for. In this example, the response is cached for 30 seconds. In your CLI output, verify that you get back the `cache-control` response header with a `max-age=30` value. 
      ```shell
      curl -vik "$(glooctl proxy url)/httpbin/cache/30"
      ```
      
      Example output: 
      ```
      < HTTP/1.1 200 OK
      HTTP/1.1 200 OK
      < date: Wed, 14 Dec 2022 19:32:13 GMT
      date: Wed, 14 Dec 2022 19:32:13 GMT
      < content-type: application/json
      content-type: application/json
      < content-length: 423
      content-length: 423
      < server: envoy
      server: envoy
      < cache-control: public, max-age=30
      cache-control: public, max-age=30
      < access-control-allow-origin: *
      access-control-allow-origin: *
      < access-control-allow-credentials: true
      access-control-allow-credentials: true
      < x-envoy-upstream-service-time: 60
      x-envoy-upstream-service-time: 60

      < 
     {
       "args": {}, 
       "headers": {
       "Accept": "*/*", 
       "Host": "34.173.214.185", 
       "If-Modified-Since": "Wed, 14 Dec 2022 19:03:15 GMT", 
       "User-Agent": "curl/7.77.0", 
       "X-Amzn-Trace-Id": "Root=1-639a24bd-368eb5d92130a8b35144ce4d", 
       "X-Envoy-Expected-Rq-Timeout-Ms": "15000", 
       "X-Envoy-Original-Path": "/httpbin/cache/30"
      }, 
        "origin": "32.200.10.110", 
        "url": "http://34.173.214.185/cache/30"
      }
      ```
   
   2. Send another request to the same endpoint within the 30s timeframe. In your CLI output, verify that you get back the original response. In addition, check that an `age` response header is returned indicating the age of the cached response and that the `date` header uses the date and time of the original response. 
      ```shell
      curl -vik "$(glooctl proxy url)/httpbin/cache/30"
      ```
      
      Example output: 
      ```
      ...
      date: Wed, 14 Dec 2022 19:32:13 GMT
      < age: 24
      age: 24

      < 
      {
        "args": {}, 
        "headers": {
        "Accept": "*/*", 
        "Host": "34.173.214.185", 
        "If-Modified-Since": "Wed, 14 Dec 2022 19:03:15 GMT", 
        "User-Agent": "curl/7.77.0", 
        "X-Amzn-Trace-Id": "Root=1-639a24bd-368eb5d92130a8b35144ce4d", 
        "X-Envoy-Expected-Rq-Timeout-Ms": "15000", 
        "X-Envoy-Original-Path": "/httpbin/cache/30"
      }, 
        "origin": "32.200.10.110", 
        "url": "http://34.173.214.185/cache/30"
      }
      ```
      
   3. Wait until the 30 seconds have passed and the response becomes stale. Send another request to the same endpoint and verify that you get back a fresh response and that no `age` header is returned. 
      ```shell
      curl -vik "$(glooctl proxy url)/httpbin/cache/30"
      ```
      
      Example output: 
      ```
      cache-control: public, max-age=30
      < access-control-allow-origin: *
      access-control-allow-origin: *
      < access-control-allow-credentials: true
      access-control-allow-credentials: true
      < x-envoy-upstream-service-time: 275
      x-envoy-upstream-service-time: 275

      < 
       {
         "args": {}, 
         "headers": {
         "Accept": "*/*", 
         "Host": "34.173.214.185", 
         "If-Modified-Since": "Wed, 14 Dec 2022 19:32:13 GMT", 
         "User-Agent": "curl/7.77.0", 
         "X-Amzn-Trace-Id": "Root=1-639a27f5-2e83d6cb694728cd3e53c8fc", 
         "X-Envoy-Expected-Rq-Timeout-Ms": "15000", 
         "X-Envoy-Original-Path": "/httpbin/cache/30"
      }, 
        "origin": "32.200.10.110", 
        "url": "http://34.173.214.185/cache/30"
      }
      ```
      
6. Try out caching with response validation. Response validation must be implemented in the upstream service directly. The upstream must be capable of reading the date and time that is sent in the `If-Modified-Since` request header and to check whether or not the response has changed since then. 
   1. Repeat setps 5.1 and 5.2 to 
   
   ```shell
   curl -vik "$(glooctl proxy url)/httpbin/cache/30"
   ```
      
      
