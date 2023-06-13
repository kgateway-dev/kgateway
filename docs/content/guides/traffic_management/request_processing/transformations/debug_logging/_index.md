---
title: Transformation Debug Logging
weight: 50
description: Debug complex sequences of transformations.
---

In this tutorial we will see how to use Gloo Edge's debug logging feature to debug complex sequences of transformations.

{{% notice warning %}}
This feature has the potential to log sensitive information. We do not recommend enabling this feature in production environments.
{{% /notice %}}

### Setup
{{< readfile file="/static/content/setup_postman_echo.md" markdown="true">}}

Let's also create a simple Virtual Service that matches any path and routes all traffic to our Upstream:

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
      options:
        autoHostRewrite: true
{{< /tab >}}
{{< /tabs >}}

Let's test that the configuration was correctly picked up by Gloo Edge by executing the following command:

```shell
curl $(glooctl proxy url)/get | jq
```

You should see an output similar like this:

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

#### Update Virtual Service
Now, let's update the virtual service to include transformations that add headers to the response.

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
        autoHostRewrite: true
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

#### Test the modified configuration
To test that our configuration has been correctly picked up by Gloo Edge, let's execute our `curl` command again:

```shell
curl $(glooctl proxy url)/get | jq
```

Notice that the response now includes the headers we added in the transformation:

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

#### Add debug logging
Now, let's add debug logging to our Virtual Service. We will add debug logging for the `regular` stage of the transformation.

{{< highlight yaml "hl_lines=32" >}}
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
        autoHostRewrite: true
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

#### Test the modified configuration
To test that our configuration has been correctly picked up by Gloo Edge, let's first set the log level within the gateway-proxy pod to `debug`:

```shell
kubectl port-forward -n gloo-system deployment/gateway-proxy 19000:19000
curl "localhost:19000/logging?level=debug" -X POST
```

Then, let's execute our `curl` command again:

```shell
curl $(glooctl proxy url)/get | jq
```

You should see identical output as before, but now you should also see the debug logs in the gateway-proxy logs. You can see the logs by running the following command:

```shell
kubectl logs -n gloo-system deployment/gateway-proxy
```

These logs should contain the following excerpt:

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

Here, we can see that the body and request headers are logged before an after the transformation -- note that the `x-regular-request-header` is present in the request headers after the transformation is processed.

### Cleanup
To cleanup the resources created in this tutorial you can run the following commands:

```shell
kubectl delete virtualservice -n gloo-system test-debug-logs
kubectl delete upstream -n gloo-system postman-echo
```