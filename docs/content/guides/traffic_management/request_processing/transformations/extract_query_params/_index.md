---
title: Extract query parameters
weight: 20
description: Extract query parameters from a request
---

In this tutorial we will see how to extract query parameters from a request and use them in a transformation.

### Setup
{{< readfile file="/static/content/setup_postman_echo.md" markdown="true">}}

Let's also create a simple Virtual Service that matches any path and routes all traffic to our Upstream:

{{< tabs >}}
{{< tab name="kubectl" codelang="yaml">}}
apiVersion: gateway.solo.io/v1
kind: VirtualService
metadata:
  name: extract-query-params
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
    "x-request-id": "db7eca70-630a-4aab-8e42-7ac3cfa064e8",
    "x-forwarded-port": "80"
  },
  "url": "https://postman-echo.com/get"
}
```

#### Update Virtual Service
As you can see from the response above, the upstream service returns the request headers as part of the JSON payload. We will now configure Gloo Edge to extract the values of the `foo` and `bar` query parameters and use them to create two new headers named - you guessed it - `foo` and `bar`.

To implement this behavior, we need to add the following to our Virtual Service definition:

{{< highlight yaml "hl_lines=20-44" >}}
apiVersion: gateway.solo.io/v1
kind: VirtualService
metadata:
  name: extract-query-params
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
            extractors:
              # This extracts the 'foo' query param to an extractor named 'foo'
              foo:
                # The :path pseudo-header contains the URI
                header: ':path'
                # Use a nested capturing group to extract the query param
                regex: '(.*foo=([^&]*).*)'
                subgroup: 2
              # This extracts the 'bar' query param to an extractor named 'bar'
              bar:
                # The :path pseudo-header contains the URI
                header: ':path'
                # Use a nested capturing group to extract the query param
                regex: '(.*bar=([^&]*).*)'
                subgroup: 2
            # Add two new headers with the values of the 'foo' and 'bar' extractions
            headers:
              foo:
                text: '{{ foo }}'
              bar:
                text: '{{ bar }}'
{{< /highlight >}}  

The above `options` configuration is to be interpreted as following:

1. Add a transformation to all traffic handled by this Virtual Host.
1. Apply the transformation only to requests.
1. Define two [extractions]({{% versioned_link_path fromRoot="/guides/traffic_management/request_processing/transformations#extractors" %}}) 
to extract the values of the query parameters. We achieve this by using regex capturing groups and selecting the nested group 
which matches only the value of the relevant query parameter.
1. Add two headers and set their values of the values of the extractions.

#### Test our configuration
To test that our configuration has been correctly applied, let's add the two expected query parameters to the previously 
used `curl` command:

```shell
curl "$(glooctl proxy url)/get?foo=foo-value&bar=bar-value" | jq
```

You should get the following output:

```json
{
  "args": {
    "foo": "foo-value",
    "bar": "bar-value"
  },
  "headers": {
    "x-forwarded-proto": "https",
    "host": "postman-echo.com",
    "accept": "*/*",
    "bar": "bar-value",
    "foo": "foo-value",
    "user-agent": "curl/7.54.0",
    "x-envoy-expected-rq-timeout-ms": "15000",
    "x-request-id": "9dbe87fe-7ee8-4d47-b3e4-76c545515d33",
    "x-forwarded-port": "80"
  },
  "url": "https://postman-echo.com/get?foo=foo-value&bar=bar-value"
}
```

Notice that the `headers` section now contains two new attributes with the expected values.

{{% notice note %}}
In this guide we used **extractions** to add new headers, but you can use the extractions you define in any template.
{{% /notice %}}

### Cleanup
To cleanup the resources created in this tutorial you can run the following commands:

```shell
kubectl delete virtualservice -n gloo-system extract-query-params
kubectl delete upstream -n gloo-system postman-echo
```
