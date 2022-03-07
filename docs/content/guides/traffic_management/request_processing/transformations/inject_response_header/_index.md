---
title: Inject response header
weight: 10
description: Inject request headers into response
---

What if you require a routing policy to inject a header from an inbound request into a response header?

### Setup
{{< readfile file="/static/content/setup_postman_echo.md" markdown="true">}}

Let's also create a simple Virtual Service that matches any path and routes all traffic to our Upstream:

{{< tabs >}}
{{< tab name="kubectl" codelang="yaml">}}
apiVersion: gateway.solo.io/v1
kind: VirtualService
metadata:
  name: inject-response-header
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

Let's test that the configuration was correctly picked up by Gloo Edge by executing the following command:

```shell
curl -v -H "x-solo-hdr1: val1" $(glooctl proxy url)/get -i
```

You should get a response with status `200` and a JSON body similar to the one below. Note that the `x-solo-hdr1` header is in the payload response from postman-echo, but it is not included in the response headers reported by curl.

```
HTTP/1.1 200 OK
date: Thu, 03 Mar 2022 22:29:21 GMT
content-type: application/json; charset=utf-8
content-length: 349
vary: Accept-Encoding
x-envoy-upstream-service-time: 42
server: envoy

{"args":{},"headers":{"x-forwarded-proto":"http","x-forwarded-port":"80","host":"35.185.51.108","x-amzn-trace-id":"Root=1-62214141-5de08d0b3bae549e7cea830e","user-agent":"curl/7.77.0","accept":"*/*","x-solo-hdr1":"val1","x-request-id":"3ab8790f-d392-4e37-93f1-ecb0e0d6ce41","x-envoy-expected-rq-timeout-ms":"15000"},"url":"http://35.185.51.108/get"}
```

### Injecting the response header
As you can see from the response above, the upstream service echoes the JSON payload we included in our request inside the `data` response body attribute. We will now configure Gloo Edge to change the response status to 400 if the `data.error` attribute is present; otherwise, the original status code should be preserved.

#### Update the Virtual Service
To implement this behavior, we need to add a `responseTransformation` stanza to our original Virtual Service definition. Note that the `request_header` function is used in an [Inja template]({{% versioned_link_path fromRoot="/guides/traffic_management/request_processing/transformations#templating-language" %}}) to extract the value of the `x-solo-hdr` header from the request. Then it injects that value into a new response header `x-solo-resp-hdr1`.

{{< highlight yaml "hl_lines=18-24" >}}
apiVersion: gateway.solo.io/v1
kind: VirtualService
metadata:
  name: inject-response-header
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
        responseTransformation:
          transformationTemplate:
            headers:
              x-solo-resp-hdr1:
                text: '{{ request_header("x-solo-hdr1") }}'
{{< /highlight >}}

#### Test the modified configuration
We'll test our modified Virtual Service by issuing the same curl command as before:

```shell
curl -H "x-solo-hdr1: val1" $(glooctl proxy url)/get -i
```

This should yield something similar to the following output. Note that in the curl response, there is a new response header: `x-solo-resp-hdr1: val1`

```
HTTP/1.1 200 OK
date: Thu, 03 Mar 2022 22:40:27 GMT
content-type: application/json; charset=utf-8
content-length: 349
vary: Accept-Encoding
x-envoy-upstream-service-time: 45
x-solo-resp-hdr1: val1
server: envoy

{"args":{},"headers":{"x-forwarded-proto":"http","x-forwarded-port":"80","host":"35.185.51.108","x-amzn-trace-id":"Root=1-622143db-7397fdb03f893d082bfa5028","user-agent":"curl/7.77.0","accept":"*/*","x-solo-hdr1":"val1","x-request-id":"ea032e6b-5536-49b9-a5d6-55f70bf480a5","x-envoy-expected-rq-timeout-ms":"15000"},"url":"http://35.185.51.108/get"}
```

Congratulations! You have successfully used a response transformation to inject the value of a request header into a new response header.

### Cleanup
To cleanup the resources created in this tutorial you can run the following commands:

```shell
kubectl delete virtualservice -n gloo-system inject-response-header
kubectl delete upstream -n gloo-system postman-echo
```