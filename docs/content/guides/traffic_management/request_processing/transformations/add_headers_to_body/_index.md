---
title: Add headers to the body
weight: 40
description: Adding request/response headers to the body
---

In this tutorial we will see how to extract headers and add them to the JSON body of a request (or a response).

### Setup
{{< readfile file="/static/content/setup_postman_echo.md" markdown="true">}}

Let's also create a simple Virtual Service that matches any path and routes all traffic to our Upstream:

{{< tabs >}}
{{< tab name="kubectl" codelang="yaml">}}
apiVersion: gateway.solo.io/v1
kind: VirtualService
metadata:
  name: headers-to-body
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

We will be sending POST requests to the upstream, so let's create a simple JSON file that will constitute our request body. Create a file named `data.json` with the following content in your current working directory:

```shell
cat << EOF > data.json
{
  "payload": {
    "foo": "bar"
  }
}
EOF
```

Let's test that the configuration was correctly picked up by Gloo Edge by executing the following command:

```shell
curl -H "Content-Type: application/json" $(glooctl proxy url)/post -d @data.json | jq
```

You should get a response with status `200` and a JSON body similar to this:

```json
{
  "args": {},
  "data": {
    "payload": {
      "foo": "bar"
    }
  },
  "files": {},
  "form": {},
  "headers": {
    "x-forwarded-proto": "https",
    "host": "postman-echo.com",
    "content-length": "35",
    "accept": "*/*",
    "content-type": "application/json",
    "user-agent": "curl/7.54.0",
    "x-envoy-expected-rq-timeout-ms": "15000",
    "x-request-id": "2ae7b930-bf4f-476e-9c56-d3cec1c564d1",
    "x-forwarded-port": "80"
  },
  "json": {
    "payload": {
      "foo": "bar"
    }
  },
  "url": "https://postman-echo.com/post"
}
```

### Updating the response code
As you can see from the response above, the upstream service echoes the JSON payload we included in our request inside the `data` response body attribute. We will now configure Gloo Edge to add the values of two headers to the body:

- the value of the `root` header will be added to a new `root` attribute at the top level of the JSON body,
- the value of the `nested` header will be added to a new `nested` attribute inside the `payload` attribute.

#### Update Virtual Service
To implement this behavior, we need to add the following to our Virtual Service definition:

{{< highlight yaml "hl_lines=20-38" >}}
apiVersion: gateway.solo.io/v1
kind: VirtualService
metadata:
  name: headers-to-body
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
            # Merge the specified extractors to the request body
            merge_extractors_to_body: {}
            extractors:
              # The name of this attribute determines where the value will be nested in the body (using dots)
              root:
                # Name of the header to extract
                header: 'root'
                # Regex to apply to it, this is needed
                regex: '.*'
              # The name of this attribute determines where the value will be nested in the body (using dots)
              payload.nested:
                # Name of the header to extract
                header: 'nested'
                # Regex to apply to it, this is needed
                regex: '.*'
{{< /highlight >}}

The above `options` configuration is to be interpreted as following:

1. Add a transformation to all traffic handled by this Virtual Host.
1. Apply the transformation only to responses.
1. Use a [template transformation]({{% versioned_link_path fromRoot="/guides/traffic_management/request_processing/transformations#transformation-templates" %}}).
1. Define two [extractions]({{% versioned_link_path fromRoot="/guides/traffic_management/request_processing/transformations#extractors" %}}) to extract the required headers from the request. You can control where the values will be nested in the body by using separators in their names (dots if `advancedTemplates` is `false`, forward slashes otherwise).
1. Merge the defined extractions to the request body.

#### Test our configuration
To test that our configuration has been correctly applied, let's execute `curl` command again, adding the two headers:

```shell
curl -H "Content-Type: application/json" -H "root: root-val" -H "nested: nested-val" $(glooctl proxy url)/post -d @data.json | jq
```

You should get the following output, indicating that the headers have been merged into the request body at the expected locations:

{{< highlight json "hl_lines=6 8" >}}
{
  "args": {},
  "data": {
    "payload": {
      "foo": "bar",
      "nested": "nested-val"
    },
    "root": "root-val"
  },
  "files": {},
  "form": {},
  "headers": {
    "x-forwarded-proto": "https",
    "host": "postman-echo.com",
    "content-length": "65",
    "accept": "*/*",
    "content-type": "application/json",
    "nested": "nested-val",
    "root": "root-val",
    "user-agent": "curl/7.54.0",
    "x-envoy-expected-rq-timeout-ms": "15000",
    "x-request-id": "57c255fc-9412-4bf8-97cb-e7f495240703",
    "x-forwarded-port": "80"
  },
  "json": {
    "payload": {
      "foo": "bar",
      "nested": "nested-val"
    },
    "root": "root-val"
  },
  "url": "https://postman-echo.com/post"
}
{{< /highlight >}}

### Cleanup
To cleanup the resources created in this tutorial you can run the following commands:

```shell
kubectl delete virtualservice -n gloo-system headers-to-body
kubectl delete upstream -n gloo-system postman-echo
```