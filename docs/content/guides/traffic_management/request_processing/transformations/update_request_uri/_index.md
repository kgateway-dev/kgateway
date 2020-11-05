---
title: Update request path
weight: 30
description: Conditionally update the request path
---

In this tutorial we will see how to conditionally update the request path by using a transformation.

### Setup
{{< readfile file="/static/content/setup_postman_echo.md" markdown="true">}}

Let's also create a simple Virtual Service that matches any path and routes all traffic to our Upstream:

{{< tabs >}}
{{< tab name="kubectl" codelang="yaml">}}
apiVersion: gateway.solo.io/v1
kind: VirtualService
metadata:
  name: update-request-path
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

You should get a response with status `200` and a JSON body similar to this:

```json
{
  "args": {},
  "headers": {
    "x-forwarded-proto": "https",
    "host": "postman-echo.com",
    "accept": "*/*",
    "user-agent": "curl/7.54.0",
    "x-envoy-expected-rq-timeout-ms": "15000",
    "x-request-id": "3ed578a1-b33a-40db-b18a-e8d11e24c22c",
    "x-forwarded-port": "80"
  },
  "url": "https://postman-echo.com/get"
}
```

#### Update Virtual Service
We will now configure Gloo Edge to update the request path from `/get` to `/post` if a header named `foo` is present and has the value `bar`. Since the `/post` endpoint on the Postman Echo service expected `POST` requests, we will need to update the HTTP method of the request as well.

To do this, we need to add the following to our Virtual Service definition:

{{< highlight yaml "hl_lines=20-44" >}}
apiVersion: gateway.solo.io/v1
kind: VirtualService
metadata:
  name: update-request-path
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
      transformations:
        requestTransformation:
          transformationTemplate:
            headers:
              # By updating the :path pseudo-header, we update the request URI
              ":path":
                text: '{% if header("foo") == "bar" %}/post{% else %}{{ header(":path") }}{% endif %}'
              # By updating the :method pseudo-header, we update the request HTTP method
              ":method":
                text: '{% if header("foo") == "bar" %}POST{% else %}{{ header(":method") }}{% endif %}'
{{< /highlight >}}  

The above `options` configuration is to be interpreted as following:

1. Add a transformation to all traffic handled by this Virtual Host.
1. Apply the transformation only to requests.
1. Use a [template transformation]({{% versioned_link_path fromRoot="/guides/traffic_management/request_processing/transformations#transformation-templates" %}}).
1. Update the `:path` and `:method` pseudo-headers if the `foo` header is present and has value `bar`; otherwise keep the original values.

The template uses the [Inja templating language]({{% versioned_link_path fromRoot="/guides/traffic_management/request_processing/transformations#templating-language" %}}) to define the conditional logic that will be applied to the `:path` and `:method` pseudo-headers.

#### Test our configuration
To test that our configuration has been correctly applied, let's add the `foo` header with the expected `bar` value to the previously used `curl` command:

```shell
curl -H "foo: bar" $(glooctl proxy url)/get | jq
```

You should get the following output:

{{< highlight yaml "hl_lines=18" >}}
{
  "args": {},
  "data": {},
  "files": {},
  "form": {},
  "headers": {
    "x-forwarded-proto": "https",
    "host": "postman-echo.com",
    "content-length": "0",
    "accept": "*/*",
    "foo": "bar",
    "user-agent": "curl/7.54.0",
    "x-envoy-expected-rq-timeout-ms": "15000",
    "x-request-id": "193637cb-d551-4c0e-80a6-866218c25c3b",
    "x-forwarded-port": "80"
  },
  "json": null,
  "url": "https://postman-echo.com/post"
}
{{< /highlight >}} 

Notice that the `url` attribute now displays a `/post` path where it previously displayed `/get`.

Now let's try omitting the header from the request:

```shell
curl $(glooctl proxy url)/get | jq
```

We will hit the same endpoint as we did at the beginning of this tutorial:

{{< highlight yaml "hl_lines=12" >}}
{
  "args": {},
  "headers": {
    "x-forwarded-proto": "https",
    "host": "postman-echo.com",
    "accept": "*/*",
    "user-agent": "curl/7.54.0",
    "x-envoy-expected-rq-timeout-ms": "15000",
    "x-request-id": "f8844eba-85cd-4253-81a2-12f143d9db41",
    "x-forwarded-port": "80"
  },
  "url": "https://postman-echo.com/get"
}
{{< /highlight >}} 

### Cleanup
To cleanup the resources created in this tutorial you can run the following commands:

```shell
kubectl delete virtualservice -n gloo-system update-request-path
kubectl delete upstream -n gloo-system postman-echo
```