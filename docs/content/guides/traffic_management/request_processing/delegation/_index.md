---
title: Delegate VirtualHost and RouteTable options
weight: 200
description: Define 
---

You can use the `VirtualHostOption` and `RouteTableOption` resources to specify option settings for virtual hosts and route tables, such as traffic management configurations or policies. The main virtual host and route table resources can then delegate to these options to apply them. 

This approach is useful if you have traffic management or policy settings that you want to apply to multiple virtual hosts or routes, such as a rate limiting configuration that you want to reuse across hosts. It can also help to keep the main virtual host and route table resources small which can simplify the troubleshooting process in case of an error.


## Rules for delegating to VirtualHostOption and RouteTableOption resources {#rules}

When delegating to `VirtualHostOption` and `RouteTableOption` resources from the main virtual service or route table resource, the following rules apply: 
- Options that are specified on the main virtual service or route table resource take precedence over options that are specified on `VirtualHostOption` and `RouteTableOption` resources. For example, if a header manipulation policy is specified on the main virtual service resource, header manipulation policies in `VirtualHostOption` resources that the virtual service delegates to are ignored. 
- If delegating to multiple `VirtualHostOption` and `RouteTableOption` resources, the resource configuration is applied in order. For example, if a virtual service delegates to two `VirtualHostOption` resources, and both of them specify retry policies, only the retry policy from the `VirtualHostOption` resource that is delegated to first is applied. The retry policy in subsequent `VirtualHostOption` resources is ignored. 

{{% notice note %}}
Keep in mind that delegating to a lot of `VirtualHostOption` and `RouteTableOption` resources can complicate the troubleshooting process as you might need to analyze multiple option resources to find the root cause of an issue. In addition, updates to `VirtualHostOption` and `RouteTableOption` resources that introduce errors can have large impacts on your environment depending on how often these resources are delegated to. 
{{% /notice %}}

## Example 

The following example uses the httpbin app to demonstrate how option settings in`VirtualService` and `VirtualHostOption` resources are applied. The same rules apply to `RouteTable` and `RouteTableOption` resources. 

1. Create the httpbin namespace.
   ```sh
   kubectl create ns httpbin
   ```
2. Deploy the httpbin app.
   ```sh
   kubectl -n httpbin apply -f https://raw.githubusercontent.com/solo-io/gloo-mesh-use-cases/main/policy-demo/httpbin.yaml
   ```

   Example output: 
   ```
   serviceaccount/httpbin created
   service/httpbin created
   deployment.apps/httpbin created
   ```

3. Verify that the httpbin app is running.
   ```sh
   kubectl -n httpbin get pods
   ```

   Example output:
   ```
   NAME                      READY   STATUS    RESTARTS   AGE
   httpbin-d57c95548-nz98t   3/3     Running   0          18s
   ```

4. Create a virtual service to expose the httpbin app on the gateway. The following virtual service removes the `User-Agent` header from the request and delegates to two `VirtualHostOption` resources that you create in later steps. Both `VirtualHostOption` resources apply additional configuration to the virtual host. 
   ```yaml
   kubectl apply -f- <<EOF
   apiVersion: gateway.solo.io/v1
   kind: VirtualService
   metadata:
     name: httpbin
     namespace: httpbin
     ownerReferences: []
   spec:
     virtualHost:
       domains:
       - '*'
       options:
         headerManipulation:
           requestHeadersToRemove: ["User-Agent"] 
       optionsConfigRefs:
         delegateOptions:
           - name: virtualhost-options-1
             namespace: gloo-system
           - name: virtualhost-options-2
             namespace: gloo-system
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

4. Create a `VirtualHostOption` resource that defines additional virtual host options. In the following example, you want to add the `myheader` header to the request and apply a retry policy. 
   ```yaml
   kubectl apply -f- <<EOF
   apiVersion: gateway.solo.io/v1
   kind: VirtualHostOption
   metadata:
     name: virtualhost-options-1
     namespace: gloo-system
   spec:
     options:
       headerManipulation:
         requestHeadersToAdd: 
           - header: 
               key: myheader
               value: "option-test"
       retries:
         retryOn: 'connect-failure'
         numRetries: 3
         perTryTimeout: '5s'
   EOF
   ```

5. Create a second `VirtualHostOption` resource that defines more virtual host options. In this example, you want to remove the `content-type` request header and limit requests to your app to 1 per minute. 
   ```yaml
   kubectl apply -f- <<EOF
   apiVersion: gateway.solo.io/v1
   kind: VirtualHostOption
   metadata:
     name: virtualhost-options-2
     namespace: gloo-system
   spec:
     options:
       headerManipulation:
         requestHeadersToRemove: ["content-header"]
       ratelimitBasic:
         anonymousLimits:
           unit: MINUTE
           requestsPerUnit: 1
       retries:
         retryOn: 'connect-failure'
         numRetries: 5
         perTryTimeout: '10s'
   EOF
   ```

6. Verify your final settings by sending multiple requests to the httpbin app. Following the [rules for applying virtual host and route table option resources](#rules), the following behavior is expected: 
   - `headerManipulation`: Because the main virtual service resource specifies header manipulation settings, these settings take precedence over header manipulation settings that are configured on delegated `VirtualHostOption` resources. Consequently, the header manipulation settings from both `VirtualHostOption` resources are ignored. 
   - `retries`: Retry settings from the `virtualhost-options-1` resource take precedence over the retry settings in `virtualhost-options-2`, because the main virtual service resource delegates to `virtualhost-options-1` first. Retry settings in subsequent `VirtualHostOption` resources are ignored. 
   - `ratelimitBasic`: Because the main virtual service and the `virtualhost-options-1` virtual host option resources do not specify any rate limiting settings, the rate limit settings from `virtualhost-options-2` are applied to the virtual host. 

   ```sh
   curl -vik $(glooctl proxy url)/headers
   ```

   Example output for the first request. Note that the `User-Agent` request header is removed and that neither the `myheader` header from `virtualhost-options-1` was added, nor the `Accept` header from `virtualhost-options-2` was removed. 
   ```
   * Mark bundle as not supporting multiuse
   < HTTP/1.1 200 OK
   HTTP/1.1 200 OK
   < access-control-allow-credentials: true
   access-control-allow-credentials: true
   < access-control-allow-origin: *
   access-control-allow-origin: *
   < content-type: application/json; encoding=utf-8
   content-type: application/json; encoding=utf-8
   < date: Wed, 29 Nov 2023 19:40:11 GMT
   date: Wed, 29 Nov 2023 19:40:11 GMT
   < content-length: 339
   content-length: 339
   < x-envoy-upstream-service-time: 2
   x-envoy-upstream-service-time: 2
   < server: envoy
   server: envoy

   < 
   {
     "headers": {
       "Accept": [
         "*/*"
       ],
       "Host": [
         "a852ba7eb4adf4042a8e392766a1eca4-1405206594.us-east-2.elb.amazonaws.com"
       ],
       "X-Envoy-Expected-Rq-Timeout-Ms": [
         "5000"
       ],
       "X-Forwarded-Proto": [
         "http"
       ],
       "X-Request-Id": [
         "d809363f-c8c7-45fc-a33b-e67246c874b3"
       ]
      }
   }
   ```

   Example output after sending a few requests to httpbin. Verify that your rate limiting settings from `virtualhost-options-2` are applied and that you see a 429 HTTP response code. 
   ```
   * Mark bundle as not supporting multiuse
   < HTTP/1.1 429 Too Many Requests
   HTTP/1.1 429 Too Many Requests
   < x-envoy-ratelimited: true
   x-envoy-ratelimited: true
   < date: Wed, 29 Nov 2023 19:45:01 GMT
   date: Wed, 29 Nov 2023 19:45:01 GMT
   < server: envoy
   server: envoy
   < content-length: 0
   content-length: 0
   ```

7. Find your settings in the currently applied Gloo Gateway configuration to verify the settings that were applied. 
   ```sh
   glooctl proxy served-config
   ```

   Example output: 
   ```
   name: listener-::-8080-routes
   virtualHosts:
   - domains:
     - '*'
     name: gloo-system_httpbin
     # rate limit settings from virtualhost-options-2
     rateLimits:
     - actions:
       - genericKey:
           descriptorValue: gloo-system_httpbin
       - headerValueMatch:
           descriptorValue: is-authenticated
           expectMatch: true
           headers:
           - name: x-user-id
             presentMatch: true
       - requestHeaders:
           descriptorKey: userid
           headerName: x-user-id
       stage: 0
     - actions:
          - genericKey:
           descriptorValue: gloo-system_httpbin
       - headerValueMatch:
           descriptorValue: not-authenticated
           expectMatch: false
           headers:
           - name: x-user-id
             presentMatch: true
       - remoteAddress: {}
       stage: 0
     # header manipulation settings from main virtual service
     requestHeadersToRemove:
     - User-Agent
     # retry policy settings from virtualhost-options-1
     retryPolicy:
       numRetries: 3
       perTryTimeout: 5s
       retryOn: connect-failure
     routes:
     - match:
         prefix: /
       name: gloo-system_httpbin-route-0-matcher-0
       route:
         cluster: httpbin-httpbin-8000_gloo-system
     typedPerFilterConfig:
       envoy.filters.http.ext_authz:
         '@type': type.googleapis.com/envoy.extensions.filters.http.ext_authz.v3.ExtAuthzPerRoute
         disabled: true
   ```


## Cleanup

You can optionally clean up the resources that you created as part of this guide. 

```sh
kubectl delete virtualservice httpbin -n httpbin
kubectl delete virtualhostoption virtualhost-options-1 -n gloo-system
kubectl delete virtualhostoption virtualhost-options-2 -n gloo-system
kubectl delete -n httpbin -f https://raw.githubusercontent.com/solo-io/gloo-mesh-use-cases/main/policy-demo/httpbin.yaml
kubectl delete namespace httpbin
```