---
title: HTTP gateway
weight: 20
description: Learn how to apply local rate limiting settigns to the HTTP Envoy filter for Layer 4 and Layer 7 traffic. 
---

Set local rate limiting settings on an HTTP gateway, virtual service, or route to limit the number of incoming HTTP requests that are allowed to enter the cluster before global rate limiting and external auth policies are applied. 

To learn more about what local rate limiting is and the differences between local and global rate limiting, see [About local rate limiting]({{% versioned_link_path fromRoot="/guides/security/local_rate_limiting/overview/" %}}).

On this page: 
- [Deploy the httpbin sample app](#sample-app)
- [Apply local rate limit settings to Layer 4 traffic](#layer4)
- [Apply local rate limit settings to Layer 7 traffic](#layer7)
- [Different local rate limit settings on the gateway and virtual service](#gateway-and-virtual-service)
- [Cleanup](#cleanup)

## Deploy the httpbin sample app {#sample-app}

To try out local rate limiting for HTTP traffic, deploy the httpbin app to your cluster. 

1. Create a service account, deployment, and service to deploy and expose the httpbin app within the cluster. 
   {{< tabs >}}
   {{% tab %}}
   ```yaml
   kubectl apply -f- <<EOF
   apiVersion: v1
   kind: ServiceAccount
   metadata:
     name: httpbin
   ---
   apiVersion: v1
   kind: Service
   metadata:
     name: httpbin
     labels:
       app: httpbin
   spec:
     ports:
     - name: http
       port: 8000
       targetPort: 80
     selector:
       app: httpbin
   ---
   apiVersion: apps/v1
   kind: Deployment
   metadata:
     name: httpbin
   spec:
     replicas: 1
     selector:
       matchLabels:
         app: httpbin
         version: v1
     template:
       metadata:
         labels:
           app: httpbin
           version: v1
       spec:
         serviceAccountName: httpbin
         containers:
         - image: docker.io/kennethreitz/httpbin
           imagePullPolicy: IfNotPresent
           name: httpbin
           ports:
           - containerPort: 80
   EOF
   ```

   Example output: 
   ```
   serviceaccount/httpbin created
   service/httpbin created
   deployment.apps/httpbin created
   ```

   {{% /tab %}}
   {{< /tabs >}}

2. Create a virtual service to expose the `/status/200` route for the httpbin app. 
   ```yaml
   kubectl apply -f- <<EOF
   apiVersion: gateway.solo.io/v1
   kind: VirtualService
   metadata:
     name: httpbin
     namespace: gloo-system
   spec:
     virtualHost:
       domains:
       - '*'
       routes:
       - matchers:
         - prefix: /status/200
         routeAction:
           single:
             upstream:
               name: default-httpbin-8000
               namespace: gloo-system
   EOF
   ```

3. Send a request to the httpbin app and verify that you get back a 200 HTTP response code.
   ```sh
   curl -vik $(glooctl proxy url)/status/200
   ```

   Example output: 
   ```
   * Mark bundle as not supporting multiuse
   < HTTP/1.1 200 OK
   HTTP/1.1 200 OK
   ...
   ```



## Apply local rate limit settings to Layer 4 traffic {#layer4}

1. Apply local rate limiting settings to Layer 4 traffic by adding the `networkLocalRatelimit` setting to the `gateway-proxy` resource. The following example configures the gateway with a token bucket with a maximum of 1 token that is refilled every 100 seconds. 
   ```yaml
   kubectl apply -n gloo-system -f - <<EOF
   apiVersion: gateway.solo.io/v1
   kind: Gateway
   metadata:
     name: gateway-proxy
     namespace: gloo-system
   spec:
     bindAddress: '::'
     bindPort: 8080
     httpGateway:
       options: 
         networkLocalRatelimit: 
           maxTokens: 1
           tokensPerFill: 1
           fillInterval: 100s
     ssl: false
     useProxyProto: false
   EOF
   ```

2. Send a request to the httpbin app. Verify that your request succeeds and a 200 HTTP response code is returned. 
   ```sh
   curl -vik $(glooctl proxy url)/status/200
   ```

   Example output: 
   ```
   ...
   * Mark bundle as not supporting multiuse
   < HTTP/1.1 200 OK
   HTTP/1.1 200 OK
   < content-type: application/xml
   content-type: application/xml
   ...
   ```

3. Send another request to the httpbin app. Note that this time the request is denied immediately. Because the gateway is configured with only 1 token that is refilled every 100 seconds, the token was assigned to the connection of the first request. No tokens were available to get assigned to the second request. Because the request is rejected on Layer 4, no HTTP response code or message is returned. 
   ```sh
   curl -vik $(glooctl proxy url)/status/200
   ```

   Example output: 
   ```
   * Empty reply from server
   * Closing connection 0
   curl: (52) Empty reply from server
   ```

## Apply local rate limit settings to Layer 7 traffic {#layer7}

1. Change the local rate limiting settings in the gateway to apply to Layer 7 traffic instead of Layer 4 traffic by using the `httpLocalRatelimit` option. The following example configures the gateway with a token bucket with a maximum of 1 token that is refilled every 100 seconds. To verify that your rate limiting settings are working as expected and to simplify troubleshooting, set `enableXRatelimitHeaders: true`. This option adds additional rate limiting headers to your response that indicate the local rate limiting settings that are applied, the number of tokens that are left in the bucket, and the number of seconds until the token bucket is refilled. For more information, see the [Envoy documentation](https://www.envoyproxy.io/docs/envoy/latest/api-v3/extensions/common/ratelimit/v3/ratelimit.proto#envoy-v3-api-enum-extensions-common-ratelimit-v3-xratelimitheadersrfcversion).
   ```yaml
   kubectl apply -n gloo-system -f - <<EOF
   apiVersion: gateway.solo.io/v1
   kind: Gateway
   metadata:
     name: gateway-proxy
     namespace: gloo-system
   spec:
     bindAddress: '::'
     bindPort: 8080
     httpGateway:
       options: 
         httpLocalRatelimit: 
           defaultLimit:
             maxTokens: 1
             tokensPerFill: 1
             fillInterval: 100s
           enableXRatelimitHeaders: true
     ssl: false
     useProxyProto: false
   EOF
   ```

2. Send a request to the httpbin app. Verify that your request succeeds and a 200 HTTP response code is returned. In addition, review the `x-ratelimit-*` headers that are returned. The `x-ratelimit-limit` header represents the token limit that is set on the gateway. To check how many tokens are available for subsequent requests, check out the `x-ratelimit-remaining` header. Use the `x-ratelimit-reset` header to view how many sesconds are left until the token bucket is refilled.
   ```sh
   curl -vik $(glooctl proxy url)/status/200
   ```

   Example output: 
   ```
   ...
   * Mark bundle as not supporting multiuse
   < HTTP/1.1 200 OK
   HTTP/1.1 200 OK
   < x-ratelimit-limit: 1
   x-ratelimit-limit: 1
   < x-ratelimit-remaining: 0
   x-ratelimit-remaining: 0
   < x-ratelimit-reset: 95
   x-ratelimit-reset: 95
   ...
   ```

3. Send another request to the httpbin app. Note that this time the request is denied with a 429 HTTP response code and a `local_rate_limited` message in your CLI output. Because the gateway is configured with only 1 token that is refilled every 100 seconds, the token was assigned to the connection of the first request. No tokens were available to get assigned to the second request. If you wait for 100 seconds, the token bucket is refilled and a new connection can be accepted by the gateway. 
   ```sh
   curl -vik $(glooctl proxy url)/status/200
   ```

   Example output:
   ```
   ...
   * Mark bundle as not supporting multiuse
   < HTTP/1.1 429 Too Many Requests
   HTTP/1.1 429 Too Many Requests
   < x-ratelimit-limit: 1
   x-ratelimit-limit: 1
   < x-ratelimit-remaining: 0
   x-ratelimit-remaining: 0
   < x-ratelimit-reset: 79
   x-ratelimit-reset: 79
   ...
   Connection #0 to host 34.XXX.XX.XXX left intact
   local_rate_limited      
   ```

## Different local rate limit settings on the gateway and virtual service {#gateway-and-virtual-service}

   
1. Change the virtual service for the httpbin app to add 2 more routes. The `/status/200` and `/ip` routes do not configure any local rate limiting settings and therefore share the same limit that is set on the gateway. However, the `/headers` route specifies and enforces its own local rate limiting settings with a maximum number of 3 tokens in the token bucket that is refilled every 30 seconds. 
   ```yaml
   kubectl apply -f- <<EOF
   apiVersion: gateway.solo.io/v1
   kind: VirtualService
   metadata:
     name: httpbin
     namespace: gloo-system
   spec:
     virtualHost:
       domains:
       - '*'
       routes:
       - matchers:
         - prefix: /status/200
         routeAction:
           single:
             upstream:
               name: default-httpbin-8000
               namespace: gloo-system
       - matchers:
         - prefix: /ip
         routeAction:
           single:
             upstream:
               name: default-httpbin-8000
               namespace: gloo-system
       - matchers:
         - prefix: /headers
         options: 
           ratelimit: 
             localRatelimit:
               maxTokens: 3
               tokensPerFill: 3
               fillInterval: 30s
         routeAction:
           single:
             upstream:
               name: default-httpbin-8000
               namespace: gloo-system
   EOF
   ```

2. Send a request to the `/status/200` route and verify that the request succeeds and you get back a 200 HTTP response code.  
   ```sh
   curl -vik $(glooctl proxy url)/status/200
   ```

   Example output: 
   ```
   ...
   * Mark bundle as not supporting multiuse
   < HTTP/1.1 200 OK
   HTTP/1.1 200 OK
   < x-ratelimit-limit: 1
   x-ratelimit-limit: 1
   < x-ratelimit-remaining: 0
   x-ratelimit-remaining: 0
   < x-ratelimit-reset: 96
   x-ratelimit-reset: 96
   ```

3. Send another request to the `/status/200` route and verify that the request is now denied and that you get back a 429 HTTP response code. 
   ```sh
   curl -vik $(glooctl proxy url)/status/200
   ```

   Example output: 
   ```
   ...
   * Mark bundle as not supporting multiuse
   < HTTP/1.1 429 Too Many Requests
   HTTP/1.1 429 Too Many Requests
   < x-ratelimit-limit: 1
   x-ratelimit-limit: 1
   < x-ratelimit-remaining: 0
   x-ratelimit-remaining: 0
   < x-ratelimit-reset: 88
   x-ratelimit-reset: 88
   ...
   local_rate_limited
   ```

4. Send a request to the `/ip` route and verify that the request is denied and that you get back a 429 HTTP response code. Because the `/status/200` and the `/ip` routes do not configure local rate limiting settings, they share the same limit that is set on the gateway, which is 1 token in the token bucket that is refilled every 100 seconds. 
   ```
   curl -vik $(glooctl proxy url)/ip
   ```

   Example output: 
   ```
   * Mark bundle as not supporting multiuse
   < HTTP/1.1 429 Too Many Requests
   HTTP/1.1 429 Too Many Requests
   < x-ratelimit-limit: 1
   x-ratelimit-limit: 1
   < x-ratelimit-remaining: 0
   x-ratelimit-remaining: 0
   < x-ratelimit-reset: 84
   x-ratelimit-reset: 84
   ```

5. Send a request to the `/headers` route and verify that this request succeeds with a 200 HTTP response code. Because this route specifies its own local rate limiting settings, these settings take precedence over the settings on the gateway. 
   ```sh
   curl -vik $(glooctl proxy url)/headers
   ```

   Example output: 
   ```
   < HTTP/1.1 200 OK
   HTTP/1.1 200 OK
   < x-ratelimit-limit: 3
   x-ratelimit-limit: 3
   < x-ratelimit-remaining: 2
   x-ratelimit-remaining: 2
   < x-ratelimit-reset: 6
   x-ratelimit-reset: 6
   ```

6. Send 3 more requests to the `/headers` route to verify that your requests are rate limited properly. 

## Cleanup

You can optionally clean up the resources that you created as part of this guide.

1. Delete the httpbin app. 
   ```sh
   kubectl delete deployment httpbin
   kubectl delete service httpbin
   kubectl delete serviceaccount httpbin
   ```

2. Delete the virtual service that exposed routes for the httpbin app. 
   ```sh
   kubectl delete virtualservice httpbin -n gloo-system
   ```

3. Restore the settings in the `gateway-proxy` gateway resource. 
   ```yaml
   kubectl apply -n gloo-system -f - <<EOF
   apiVersion: gateway.solo.io/v1
   kind: Gateway
   metadata:
     name: gateway-proxy
     namespace: gloo-system
   spec:
     bindAddress: '::'
     bindPort: 8080
     httpGateway: {}
     ssl: false
     useProxyProto: false
   EOF
   ```