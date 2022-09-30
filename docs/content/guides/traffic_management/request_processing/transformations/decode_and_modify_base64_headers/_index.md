---
title: Decode and modify base64 request headers
weight: 10
description: Decode and modify base64 encoded request headers before forwarding them upstream.
---

What if you need to decode and modify incoming headers before sending them to an upstream?

### Setup
{{< readfile file="/static/content/setup_postman_echo.md" markdown="true">}}

Let's also create a simple Virtual Service that matches any path and routes all traffic to our Upstream:

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

Let's test that the configuration was correctly picked up by Gloo Edge by executing the following command to send a request with a base64 encoded header:

```shell
curl -v -H "x-test: $(echo -n 'testprefix.testsuffix' | base64)" localhost:8080/get | jq
```

You should get a response with status `200` and a JSON body similar to the one below. Note that the `x-test` header is in the payload response from postman-echo, containing the base64 representation of the string literal `testprefix.testsuffix` as its value.

```json
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
```

### Modifying the request header
As you can see from the response above, the upstream service echoes the headers we included in our request inside the `headers` response body attribute. We will now configure Gloo Edge to decode and modify the value of this header before sending it to the upstream

#### Update the Virtual Service
To implement this behavior, we need to add a `responseTransformation` stanza to our original Virtual Service definition. Note that the `request_header`, `base64_decode`, and `substring` functions are used in an [Inja template]({{% versioned_link_path fromRoot="/guides/traffic_management/request_processing/transformations#templating-language" %}}) to:
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

#### Test the modified configuration
We'll test our modified Virtual Service by issuing the same curl command as before:

```shell
curl -v -H "x-test: $(echo -n 'testprefix.testsuffix' | base64)" localhost:8080/get | jq
```

This should yield something similar to the following output. Note that in the JSON response, there is now the value of the inject header `x-decoded-test`, which contains a substring of the decoded base64 value sent in the x-test header

```json
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
```

Congratulations! You have successfully used a request transformation to decode and modify a request header!

### Cleanup
To cleanup the resources created in this tutorial you can run the following commands:

```shell
kubectl delete virtualservice -n gloo-system decode-and-modify-header
kubectl delete upstream -n gloo-system postman-echo
```