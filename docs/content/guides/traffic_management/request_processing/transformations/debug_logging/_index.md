---
title: Debug logging for transformations
weight: 50
description: Debug complex sequences of transformations.
---

You can use Gloo Edge's debug logging feature to debug complex sequences of transformations.

{{% notice warning %}}
This feature has the potential to log sensitive information. Do not recommend enable this feature in production environments.
{{% /notice %}}

## Before you begin
{{< readfile file="/static/content/setup_postman_echo.md" markdown="true">}}

Next, create a simple Virtual Service that matches any path and routes all traffic to the Upstream:

{{< tabs >}}
{{< tab name="kubectl" codelang="yaml">}}
apiVersion: gateway.solo.io/v1
kind: VirtualService
metadata:
  name: test-debug-logs
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
            name: postman-echo
            namespace: gloo-system
{{< /tab >}}
{{< /tabs >}}

Test that the configuration was correctly picked up by Gloo Edge by executing the following command:

```shell
curl $(glooctl proxy url)/get | jq
```

Example output:

```json
{
  "args": {},
  "headers": {
    "x-forwarded-proto": "https",
    "host": "postman-echo.com",
    "accept": "*/*",
    "user-agent": "curl/7.54.0",
    "x-envoy-expected-rq-timeout-ms": "15000",
    "x-request-id": "bd20e4be-3c8a-405f-80a4-027204f732cb",
    "x-forwarded-port": "80"
  },
  "url": "https://postman-echo.com/get"
}
```

## Add debug logging to a transformation {#add-debug-logging}

You can add debug logging to individual transformations in your Virtual Service.

1. Update the virtual service to include transformations that add headers to the response.

   {{< highlight yaml "hl_lines=20-35" >}}
apiVersion: gateway.solo.io/v1
kind: VirtualService
metadata:
  name: test-debug-logs
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
            name: postman-echo
            namespace: gloo-system
    options:
      stagedTransformations:
        early:
          requestTransforms:
          - requestTransformation:
              transformationTemplate:
                headers:
                  x-early-request-header: 
                    text: "early"
        regular:
          requestTransforms:
          - requestTransformation:
              transformationTemplate:
                headers:
                  x-regular-request-header: 
                    text: "regular"
   {{< /highlight >}}

1. Test that Gloo Edge picked up the configuration update.

   ```shell
   curl $(glooctl proxy url)/get | jq
   ```

   Example output: Notice that the response now includes the early and regular headers that you added in the transformation.

   {{< highlight json "hl_lines=11-12" >}}
{
  "args": {},
  "headers": {
    "x-forwarded-proto": "http",
    "x-forwarded-port": "80",
    "host": "postman-echo.com",
    "x-amzn-trace-id": "Root=1-64886c58-5abcc9740f12068f5d1fabc8",
    "user-agent": "curl/7.87.0",
    "accept": "*/*",
    "x-request-id": "7342aa2e-77ed-4092-a646-1793ece6aab0",
    "x-early-request-header": "early",
    "x-regular-request-header": "regular",
    "x-envoy-expected-rq-timeout-ms": "15000"
  },
  "url": "http://postman-echo.com/get"
}
   {{< /highlight >}}

1. Add debug logging by using the `logRequestResponseInfo` setting. Note that logging is added only to the `regular` stage of the transformation, not the `early` stage.

   {{< highlight yaml "hl_lines=30" >}}
apiVersion: gateway.solo.io/v1
kind: VirtualService
metadata:
  name: test-debug-logs
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
            name: postman-echo
            namespace: gloo-system
    options:
      stagedTransformations:
        early:
          requestTransforms:
          - requestTransformation:
              transformationTemplate:
                headers:
                  x-early-request-header: 
                    text: "early"
        regular:
          requestTransforms:
          - requestTransformation:
              logRequestResponseInfo: true
              transformationTemplate:
                headers:
                  x-regular-request-header: 
                    text: "regular"
   {{< /highlight >}}

1. Set the log level of the gateway-proxy pod to `debug` so that you can view debug logs.

   ```shell
   kubectl port-forward -n gloo-system deployment/gateway-proxy 19000:19000
   curl "localhost:19000/logging?level=debug" -X POST
   ```

1. Repeat the request to generate fresh logs. The response matches your previous output.

   ```shell
   curl $(glooctl proxy url)/get | jq
   ```

1. Check the debug logs in the `gateway-proxy`.

   ```shell
   kubectl logs -n gloo-system deployment/gateway-proxy
   ```

   Example output: The body and request headers are logged before and after the transformation. Note that only the `x-regular-request-header` is present in the request headers after the transformation is processed, because the regular transformation is the one that you enabled debug logging for.

   ```
   [2023-06-13 13:28:27.724][38][debug][filter] [source/extensions/filters/http/transformation/transformation_filter.cc:257] [C4][S8942229055955075319] headers before transformation: ':authority', 'localhost:8080'
   ':path', '/get'
   ':method', 'GET'
   ':scheme', 'http'
   'user-agent', 'curl/7.87.0'
   'accept', '*/*'
   'x-forwarded-proto', 'http'
   'x-request-id', '18a9cdc4-c2e4-4255-84a6-5e69f4ef0ca2'
   'x-early-request-header', 'early'

   [2023-06-13 13:28:27.724][38][debug][filter] [source/extensions/filters/http/transformation/transformation_filter.cc:259] [C4][S8942229055955075319] body before transformation: 
   [2023-06-13 13:28:27.724][38][debug][filter] [source/extensions/filters/http/transformation/transformation_filter.cc:263] [C4][S8942229055955075319] headers after transformation: ':authority', 'localhost:8080'
   ':path', '/get'
   ':method', 'GET'
   ':scheme', 'http'
   'user-agent', 'curl/7.87.0'
   'accept', '*/*'
   'x-forwarded-proto', 'http'
   'x-request-id', '18a9cdc4-c2e4-4255-84a6-5e69f4ef0ca2'
   'x-early-request-header', 'early'
   'x-regular-request-header', 'regular'
   ```

## Notes

You can log request and response information at the following levels:

* The individual stage of the transformation, as shown in the previous example.
* For all staged transformations, by setting `logRequestResponseInfo` at the `stagedTransformations` level.
* For all staged transformations in your cluster, by setting `gloo.logTransformationRequestResponseInfo` in the global Settings object.
## Cleanup

Cleanup the resources created in this tutorial:

```shell
kubectl delete virtualservice -n gloo-system test-debug-logs
kubectl delete upstream -n gloo-system postman-echo
```
