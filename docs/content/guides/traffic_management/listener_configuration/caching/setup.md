---
title: Set up caching
weight: 50
description: Deploy the caching server and start caching responses from upstream services. 
---

Set up the Gloo Edge caching server to cache responses from upstream services for quicker response times.

{{% notice note %}}
This feature is available only for Gloo Edge Enterprise v1.12.x and later.
{{% /notice %}}

When caching is enabled during installation, a caching server deployment is automatically created for you and managed by Gloo Edge. Then you must configure an HTTP or HTTPS listener on your gateway to cache responses for upstream services. When the listener routes a request to an upstream, the response from the upstream is automatically cached by the caching server if it contains a `cache-control` repsonse header. All subsequent requests receive the cached response, until the cache entry expires.

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

https://docs.solo.io/gloo-edge/main/reference/api/github.com/solo-io/gloo/projects/gloo/api/v1/enterprise/options/caching/caching.proto.sk/#settings
-->

## Configure caching for a listener

Configure your gateway to cache responses for all upstreams that are served by a listener. Enabling caching for a specific upstream is currently not supported.

1. Edit the Gateway custom resource where your listener is defined.
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
     bindAddress: '::'
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

## Verify response caching with httpbin and the Envoy caching service {#try-caching}

To illustrate how caching works with and without response validation, the following apps are used: 
- **httpbin**: The `/cache/{value}` endpoint is used to show how caching works in Gloo Edge Enterprise without response validation. 
- **Envoy caching service**: The `/valid-for-minute` endpoint is used to show how caching works in Gloo Edge Enterprise with response validation. 

Follow the steps to set up `httpbin` and the Envoy caching service, and to try out caching in Gloo Edge Enterprise. 

1. Deploy and configure `httpbin`. 
   1. Create the `httpbin` namespace and deploy the app. 
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
   
   3. Get the name of upstream for `httpbin`. 
      ```shell
      kubectl get upstreams -n gloo-system
      ```

      Example output: 
      ```
      httpbin-httpbin-8000                                   40s
      ```

2. Deploy and configure the Envoy caching service. 
   1. Create a namespace for the Envoy caching service. 
      ```shell
      kubectl create ns envoy-caching
      ```
      
   2. Deploy the caching app and expose it with a Kubernetes service. 
      ```shell
      kubectl apply -n envoy-caching -f- <<EOF
      apiVersion: v1
      kind: Pod
      metadata:
        labels:
          app: service1
        name: service1
      spec:
        containers:
        - image: ghcr.io/huzlak/caching-service:0.2
          name: service1
          ports:
          - name: http
            containerPort: 8000
          readinessProbe:
            httpGet:
              port: 8000
              path: /service/1/no-cache

      ---
      apiVersion: v1
      kind: Service
      metadata:
        name: service1
      spec:
        ports:
          - port: 8000
            name: http
            targetPort: http
        selector:
          app: service1
      EOF
      ```
      
   3. Verify that the app is up and running. 
      ```shell
      kubectl get pods -n envoy-caching
      ```
      
      Example output: 
      ```
      NAMESPACE           NAME                 READY   STATUS    RESTARTS        AGE
      envoy-caching       service1             1/1     Running   0               23h
      ```
   
   4. Create an upstream for the Envoy caching service. 
      ```shell
      kubectl apply -f- <<EOF
      apiVersion: gloo.solo.io/v1
      kind: Upstream
      metadata:
        name: "envoy-caching"
        namespace: "gloo-system"
      spec:
          kube:
            serviceName: service1
            serviceNamespace: envoy-caching
            servicePort: 8000
      EOF
      ```
   
3. Create a virtual service to set up routing for the `httpbin` and the Envoy caching apps. 
   ```shell
   kubectl apply -f- <<EOF
   apiVersion: gateway.solo.io/v1
   kind: VirtualService
   metadata:
     name: cache-test-vs
     namespace: gloo-system
   spec:
     virtualHost:
       routes:
         - matchers:
             - prefix: /httpbin
           routeAction:
             single:
               upstream:
                 name: httpbin-httpbin-8000
                 namespace: gloo-system
           options:
             prefixRewrite: /
         - matchers:
             - prefix: /
           routeAction:
             single:
               upstream:
                 name: envoy-caching
                 namespace: gloo-system
   EOF
   ```

4. Verify that you can successfully route to the `httpbin` and Envoy caching service and that a 200 HTTP response code is returned.
   ```
   curl -vik "$(glooctl proxy url)/httpbin/status/200"
   curl -vik "$(glooctl proxy url)/service/1/private" 
   ```
   
5. Try out caching without response validation by using the `/cache/{value}` endpoint of the `httpbin` app. 
   1. Send a request to the `/cache/{value}` endpoint. The `{value}` variable specifies the number of seconds that you want to cache the response for. In this example, the response is cached for 30 seconds. In your CLI output, verify that you get back the `cache-control` response header with a `max-age=30` value. This response header triggers Gloo Edge to cache the response. 
      ```shell
      curl -vik "$(glooctl proxy url)/httpbin/cache/30"
      ```
      
      Example output: 
      {{< highlight yaml "hl_lines=11-12" >}}
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
      {{< /highlight >}}
   
   2. Send another request to the same endpoint within the 30s timeframe. In your CLI output, verify that you get back the original response. In addition, check that an `age` response header is returned indicating the age of the cached response, and that the `date` header uses the date and time of the original response. 
      ```shell
      curl -vik "$(glooctl proxy url)/httpbin/cache/30"
      ```
      
      Example output: 
      {{< highlight yaml "hl_lines=2-4" >}}
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
      {{< /highlight >}}
      
   3. Wait until the 30 seconds pass and the cached response becomes stale. Send another request to the same endpoint. Verify that you get back a fresh response and that no `age` header is returned. 
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
      
6. Try out caching with response validation by using the Envoy caching service. Response validation must be implemented in the upstream service directly. The upstream must be capable of reading the date and time that is sent in the `If-Modified-Since` request header and to check if the response has changed since then. 
   1. Send a request to the `/valid-for-minute` endpoint. The endpoint is configured to cache the response for 1 minute (`cache-control: max-age=60`). When the response becomes stale after 1 minute, the request validation process starts. 
      ```shell
      curl -vik "$(glooctl proxy url)/service/1/valid-for-minute"
      ```
      
      Example output: 
      ```
      < cache-control: max-age=60
      cache-control: max-age=60
      < custom-header: any value
      custom-header: any value
      < etag: "324ce9104e113743300a847331bb942ab7ace81a"
      etag: "324ce9104e113743300a847331bb942ab7ace81a"
      < date: Thu, 15 Dec 2022 15:45:19 GMT
      date: Thu, 15 Dec 2022 15:45:19 GMT
      < server: envoy
      server: envoy
      < x-envoy-upstream-service-time: 5
      x-envoy-upstream-service-time: 5

      < 
      This response will stay fresh for one minute

      Response generated at: Thu, 15 Dec 2022 15:45:19 GMT
      ```
      
   2. Send another request to the same endpoint within the 1 minute timeframe. Because the response is cached for 1 minute, the original response is returned with an `age` header indicating the number of seconds that passed since the original response was sent. Make sure that the `date` header and response body include the same information as in the original response. 
      ```shell
      curl -vik "$(glooctl proxy url)/service/1/valid-for-minute"
      ```
      
      Example output: 
      {{< highlight yaml "hl_lines=7 11 16" >}}
      ...
      < cache-control: max-age=60
      cache-control: max-age=60
      < content-length: 99
      content-length: 99
      < date: Thu, 15 Dec 2022 15:45:19 GMT
      date: Thu, 15 Dec 2022 15:45:19 GMT
      < content-type: text/html; charset=utf-8
      content-type: text/html; charset=utf-8
      < age: 5
      age: 5

      < 
      This response will stay fresh for one minute

      Response generated at: Thu, 15 Dec 2022 15:45:19 GMT
      {{< /highlight >}}
      
   3. After the 1 minute passes and the cached response becomes stale, send another request to the same endpoint. The Envoy caching app is configured to automatically add the `If-Modified-Since` header to each request to trigger the response validation process. In addition, the app is configured to always return a `304 Not Modified` HTTP response code to indicate that the response has not changed. When the `304` HTTP response code is received by the Gloo Edge caching server, the caching server fetches the original response from Redis, and sends it back to the client. 
      
      You can verify that the response validation succeeded when the `date` response header is updated with the time and date of your new request, the `age` response header is removed, and the response body contains the same information as in the original response. 
      ```shell
      curl -vik "$(glooctl proxy url)/service/1/valid-for-minute"
      ```
      
      Example output: 
      ```
      ...
      < date: Thu, 15 Dec 2022 15:53:55 GMT
      date: Thu, 15 Dec 2022 15:53:55 GMT
      < server: envoy
      server: envoy
      < x-envoy-upstream-service-time: 5
      x-envoy-upstream-service-time: 5
      < content-length: 99
      content-length: 99
      < content-type: text/html; charset=utf-8
      content-type: text/html; charset=utf-8

      < 
      This response will stay fresh for one minute

      Response generated at: Thu, 15 Dec 2022 15:45:19 GMT
      ```
            
{{% notice note %}}
Because the Envoy caching app is configured to always return a `304` HTTP response code, you continue to see the cached response no matter how many requests you send to the app. To reset the app and force the app to return a fresh response, you must restart the pod. 
{{% /notice %}}
      
