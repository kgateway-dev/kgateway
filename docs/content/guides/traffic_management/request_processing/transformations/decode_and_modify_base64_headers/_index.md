---
title: Decode and modify base64 request headers
weight: 10
description: Decode and modify base64 encoded request headers before forwarding the requests upstream.
---

You can decode and modify incoming headers before sending the request to an upstream by using Gloo [transformations]({{% versioned_link_path fromRoot="/latest/guides/traffic_management/request_processing/transformations/" %}}).

## Setup
{{< readfile file="/static/content/setup_postman_echo.md" markdown="true">}}

Next, create a simple Virtual Service that matches any path and routes all traffic to the Upstream.

{{< tabs >}}
{{< tab name="kubectl" codelang="yaml">}}
apiVersion: gateway.solo.io/v1
kind: VirtualService
metadata:
  name: decode-and-modify-header
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

Finally, test that Gloo Edge picked up the configuration by sending a request with a base64-encoded header.

```shell
curl -v -H "x-test: $(echo -n 'testprefix.testsuffix' | base64)" $(glooctl proxy url)/get | jq
```

Review the JSON output similar to the following `200` status response. Note that the `x-test` header in the payload response from postman-echo has the base64 representation of the string literal `testprefix.testsuffix` that you passed in the request.

{{< highlight json "hl_lines=10" >}}
{
  "args": {},
  "headers": {
    "x-forwarded-proto": "http",
    "x-forwarded-port": "80",
    "host": "localhost",
    "x-amzn-trace-id": "Root=1-6336f537-6c0a1f3d6c6849b10f65409c",
    "user-agent": "curl/7.64.1",
    "accept": "*/*",
    "x-test": "dGVzdHByZWZpeC50ZXN0c3VmZml4",
    "x-request-id": "7b1e64fe-e30a-437f-a826-ca5e349f50d4",
    "x-envoy-expected-rq-timeout-ms": "15000"
  },
  "url": "http://localhost/get"
}
{{< /highlight >}}

## Modifying the request header
As confirmed in the test request of the setup, the upstream service echoes the headers that you include in the request inside the `headers` response body attribute. Now, you can configure Gloo Edge to decode and modify the value of this header before sending it to the upstream.

### Update the Virtual Service
To implement this behavior, add a `responseTransformation` stanza to the original Virtual Service definition. Note that the `request_header`, `base64_decode`, and `substring` functions are used in an [Inja template]({{% versioned_link_path fromRoot="/guides/traffic_management/request_processing/transformations#templating-language" %}}) to:
 - Extract the value of the `x-test` header from the request
 - Decode the extracted value from base64
 - Extract the substring beginning with the eleventh character of the input string

The output of this chain of events is injected into a new request header `x-decoded-test`.

{{< highlight yaml "hl_lines=18-24" >}}
apiVersion: gateway.solo.io/v1
kind: VirtualService
metadata:
  name: decode-and-modify-header
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
      transformations:
        requestTransformation:
          transformationTemplate:
            headers:
              x-decoded-test:
                text: '{{substring(base64_decode(request_header("x-test")), 11)}}'
{{< /highlight >}}

### Test the modified configuration
Test the modified Virtual Service by issuing a curl request.

```shell
curl -v -H "x-test: $(echo -n 'testprefix.testsuffix' | base64)" $(glooctl proxy url)/get | jq
```

Review the output similar to the following JSON response. Note that the value of the inject header `x-decoded-test` has a substring of the decoded base64 value that was sent in the `x-test` header.

{{< highlight json "hl_lines=12" >}}
{
  "args": {},
  "headers": {
    "x-forwarded-proto": "http",
    "x-forwarded-port": "80",
    "host": "localhost",
    "x-amzn-trace-id": "Root=1-6336f482-164a3d207b026fe358de000f",
    "user-agent": "curl/7.64.1",
    "accept": "*/*",
    "x-test": "dGVzdHByZWZpeC50ZXN0c3VmZml4",
    "x-request-id": "1fbed7be-0089-4d19-a9c2-221ca088e40b",
    "x-decoded-test": "testsuffix",
    "x-envoy-expected-rq-timeout-ms": "15000"
  },
  "url": "http://localhost/get"
}
{{< /highlight >}}

Congratulations! You successfully used a request transformation to decode and modify a request header!

## Cleanup

You can clean up the resources that you created in this tutorial.

```shell
kubectl delete virtualservice -n gloo-system decode-and-modify-header
kubectl delete upstream -n gloo-system postman-echo
```
