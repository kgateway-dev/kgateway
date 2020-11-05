---
title: Change response status
weight: 10
description: Conditionally update the status of an HTTP response
---

Sometimes an upstream service does not communicate a failure via HTTP response status codes, but rather includes the failure information within the response body. But what if some of your downstream clients expect the status code to be set? In this tutorial we will see how to use transformations to change the HTTP status code based on the contents of the response payload.

### Setup
{{< readfile file="/static/content/setup_postman_echo.md" markdown="true">}}

Let's also create a simple Virtual Service that matches any path and routes all traffic to our Upstream:

{{< tabs >}}
{{< tab name="kubectl" codelang="yaml">}}
apiVersion: gateway.solo.io/v1
kind: VirtualService
metadata:
  name: update-response-code
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
  "error": {
    "message": "This is an error"
  }
}
EOF
```

Let's test that the configuration was correctly picked up by Gloo Edge by executing the following command:

```shell
curl -v -H "Content-Type: application/json" $(glooctl proxy url)/post -d @data.json | jq
```

You should get a response with status `200` and a JSON body similar to this:

```json
{
  "args": {},
  "data": {
    "error": {
      "message": "This is an error"
    }
  },
  "files": {},
  "form": {},
  "headers": {
    "x-forwarded-proto": "https",
    "host": "postman-echo.com",
    "content-length": "50",
    "accept": "*/*",
    "content-type": "application/json",
    "user-agent": "curl/7.54.0",
    "x-envoy-expected-rq-timeout-ms": "15000",
    "x-request-id": "65c4cf68-0a92-4650-a1a0-3d6104d24e51",
    "x-forwarded-port": "80"
  },
  "json": {
    "error": {
      "message": "This is an error"
    }
  },
  "url": "https://postman-echo.com/post"
}
```

### Updating the response code
As you can see from the response above, the upstream service echoes the JSON payload we included in our request inside the `data` response body attribute. We will now configure Gloo Edge to change the response status to 400 if the `data.error` attribute is present; otherwise, the original status code should be preserved.

#### Update Virtual Service
To implement this behavior, we need to add the following to our Virtual Service definition:

{{< highlight yaml "hl_lines=20-35" >}}
apiVersion: gateway.solo.io/v1
kind: VirtualService
metadata:
  name: update-response-code
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
        responseTransformation:
          transformationTemplate:
            headers:
              # We set the response status via the :status pseudo-header based on the response code
              ":status":
                text: '{% if default(data.error.message, "") != "" %}400{% else %}{{ header(":status") }}{% endif %}'
{{< /highlight >}}

The above `options` configuration is to be interpreted as following:

1. Add a transformation to all traffic handled by this Virtual Host.
1. Apply the transformation only to responses.
1. Use a [template transformation]({{% versioned_link_path fromRoot="/guides/traffic_management/request_processing/transformations#transformation-templates" %}}).
1. Transform the ":status" pseudo-header according to the template string.

The template uses the [Inja templating language]({{% versioned_link_path fromRoot="/guides/traffic_management/request_processing/transformations#templating-language" %}}) to define the conditional logic that will be applied to the ":status" header.

#### Test our configuration
To test that our configuration has been correctly applied, let's execute `curl` command again, with a slight 
modification so that it will only output the status code:

```shell
curl -s -o /dev/null -w "%{http_code}" -H "Content-Type: application/json" $(glooctl proxy url)/post -d @data.json
```

You should get the following output, representing the response code:

```
400%
```

Now let's update the `data.json` file to turn `error` into an empty object:


```shell
cat << EOF > data.json
{
  "error": {}
}
EOF
```

If you execute the same `curl` command again, you should now get a `200` response:

```shell
curl -s -o /dev/null -w "%{http_code}" -H "Content-Type: application/json" $(glooctl proxy url)/post -d @data.json
200%
```

### Cleanup
To cleanup the resources created in this tutorial you can run the following commands:

```shell
kubectl delete virtualservice -n gloo-system update-response-code
kubectl delete upstream -n gloo-system postman-echo
```