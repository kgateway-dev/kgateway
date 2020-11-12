---
title: Enriching access logs
weight: 50
description: Use transformations to craft custom attributes in access logs.
---

In this tutorial we will see how to use transformations to add custom attributes to your [Access Logs]({{% versioned_link_path fromRoot="/guides/security/access_logging/" %}}).

### Setup
{{< readfile file="/static/content/setup_postman_echo.md" markdown="true">}}

Let's also update the default `Gateway` resource to enable access logging:

{{< highlight yaml "hl_lines=14-38" >}}
apiVersion: gateway.solo.io/v1
kind: Gateway
metadata:
  labels:
    app: gloo
  name: gateway-proxy
  namespace: gloo-system
proxyNames:
- gateway-proxy
spec:
  bindAddress: '::'
  bindPort: 8080
  httpGateway: {}
  options:
    accessLoggingService:
      accessLog:
      - fileSink:
          jsonFormat:
            # HTTP method name
            httpMethod: '%REQ(:METHOD)%'
            # Protocol. Currently either HTTP/1.1 or HTTP/2.
            protocol: '%PROTOCOL%'
            # HTTP response code. Note that a response code of ‘0’ means that the server never sent the
            # beginning of a response. This generally means that the (downstream) client disconnected.
            responseCode: '%RESPONSE_CODE%'
            # Total duration in milliseconds of the request from the start time to the last byte out
            clientDuration: '%DURATION%'
            # Total duration in milliseconds of the request from the start time to the first byte read from the upstream host
            targetDuration: '%RESPONSE_DURATION%'
            # Value of the "x-envoy-original-path" header (falls back to "path" header if not present)
            path: '%REQ(X-ENVOY-ORIGINAL-PATH?:PATH)%'
            # Upstream cluster to which the upstream host belongs to
            upstreamName: '%UPSTREAM_CLUSTER%'
            # Request start time including milliseconds.
            systemTime: '%START_TIME%'
            # Unique tracking ID
            requestId: '%REQ(X-REQUEST-ID)%'
          path: /dev/stdout
{{< /highlight >}}

This configures the Gateway to create a log entry in JSON format for all incoming requests and write it to the standard output stream. For more information about access logging, see the [correspondent section]({{% versioned_link_path fromRoot="/guides/security/access_logging/" %}}) of the docs.

Finally, let's create a simple Virtual Service that matches any path and routes all traffic to our Upstream:

{{< tabs >}}
{{< tab name="kubectl" codelang="yaml">}}
apiVersion: gateway.solo.io/v1
kind: VirtualService
metadata:
  name: test-access-logs
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

Now let's take a look at the Envoy logs to verify whether the request has been logged:

```shell
# Print only log lines starting with {" (our access logs are formatted as JSON)
kubectl logs -n gloo-system deployment/gateway-proxy | grep '^{' | jq
```

You should see the following output, indicating that an access log entry has been created for the request we just sent:

```json
{
  "httpMethod": "GET",
  "clientDuration": "88",
  "upstreamName": "postman-echo_gloo-system",
  "responseCode": "200",
  "systemTime": "2019-11-01T16:58:57.576Z",
  "targetDuration": "88",
  "path": "/get",
  "requestId": "ebed1bcf-6fa0-4e56-84ee-134d6712a473",
  "protocol": "HTTP/1.1"
}
```

### Adding custom access log attributes
Envoy's access log [command operators](https://www.envoyproxy.io/docs/envoy/latest/configuration/observability/access_log#command-operators) provide a powerful way of extracting information from HTTP streams. The `REQ` and `RESP` operators allow you to log headers, but there is no way of including custom information that is not included in the headers, e.g. attributes included in the request/response payloads, or environment variables. There is a `DYNAMIC_METADATA` operator, but it relies on the custom information having been written to the [Dynamic Metadata](https://www.envoyproxy.io/docs/envoy/latest/configuration/advanced/well_known_dynamic_metadata) by an Envoy filter. Fortunately, as we saw in the [main page]({{% versioned_link_path fromRoot="/guides/traffic_management/request_processing/transformations#dynamicmetadatavalues" %}}) of the transformation docs, Gloo Edge's Transformation API provides you with the means of adding information the dynamic metadata.

Let's see how this can be done.

We will add two custom access logging attributes:

- `pod_name`: the name of the Gateway pod that handled the request
- `endpoint_url`: the URL of the upstream endpoint; note that this attribute is included in the JSON response payload returned by the Postman Echo service (see our earlier `curl` command output).

#### Update access logging configuration
We will start by updating the access logging configuration in our Gateway:

{{< highlight yaml "hl_lines=38-41" >}}
apiVersion: gateway.solo.io/v1
kind: Gateway
metadata:
  labels:
    app: gloo
  name: gateway-proxy
  namespace: gloo-system
proxyNames:
- gateway-proxy
spec:
  bindAddress: '::'
  bindPort: 8080
  httpGateway: {}
  options:
    accessLoggingService:
      accessLog:
      - fileSink:
          jsonFormat:
            # HTTP method name
            httpMethod: '%REQ(:METHOD)%'
            # Protocol. Currently either HTTP/1.1 or HTTP/2.
            protocol: '%PROTOCOL%'
            # HTTP response code. Note that a response code of ‘0’ means that the server never sent the
            # beginning of a response. This generally means that the (downstream) client disconnected.
            responseCode: '%RESPONSE_CODE%'
            # Total duration in milliseconds of the request from the start time to the last byte out
            clientDuration: '%DURATION%'
            # Total duration in milliseconds of the request from the start time to the first byte read from the upstream host
            targetDuration: '%RESPONSE_DURATION%'
            # Value of the "x-envoy-original-path" header (falls back to "path" header if not present)
            path: '%REQ(X-ENVOY-ORIGINAL-PATH?:PATH)%'
            # Upstream cluster to which the upstream host belongs to
            upstreamName: '%UPSTREAM_CLUSTER%'
            # Request start time including milliseconds.
            systemTime: '%START_TIME%'
            # Unique tracking ID
            requestId: '%REQ(X-REQUEST-ID)%'
            # The 'pod' dynamic metadata entry that is set by the Gloo Edge transformation filter
            pod_name: '%DYNAMIC_METADATA(io.solo.transformation:pod_name)%'
            # The 'error' dynamic metadata entry that is set by the Gloo Edge transformation filter
            endpoint_url: '%DYNAMIC_METADATA(io.solo.transformation:endpoint_url)%'
          path: /dev/stdout
{{< /highlight >}}

This relies on the `pod_name` and `endpoint_url` dynamic metadata entries having being added to the HTTP stream by Gloo Edge's transformation filter.

#### Update Virtual Service
For the above dynamic metadata to be available, we need to update our Virtual Service definition. Specifically, we need to add a transformation that extracts the value of the `POD_NAME` environment variable and the value of the `url` response attribute and uses them to populate the corresponding metadata attributes.

{{< highlight yaml "hl_lines=20-35" >}}
apiVersion: gateway.solo.io/v1
kind: VirtualService
metadata:
  name: test-access-logs
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
        # Apply a transformation to the response
        responseTransformation:
          transformationTemplate:
            dynamicMetadataValues:
            # Set a dynamic metadata entry named "pod_name"
            - key: 'pod_name'
              value:
                # The POD_NAME env is set by default on the gateway-proxy pods
                text: '{{ env("POD_NAME") }}'
            # Set a dynamic metadata entry using an request body attribute
            - key: 'endpoint_url'
              value:
                # The "url" attribute in the JSON response body
                text: '{{ url }}'
{{< /highlight >}}

#### Test our configuration
To test that our configuration has been correctly picked up by Gloo Edge, let's execute our `curl` command again:

```shell
curl $(glooctl proxy url)/get | jq
```

Now let's inspect the access logs again:

```shell
kubectl logs -n gloo-system deployment/gateway-proxy | grep '^{' | jq
```

You should see an entry like the following:

{{< highlight json "hl_lines=6-7" >}}
{
  "path": "/get",
  "targetDuration": "85",
  "protocol": "HTTP/1.1",
  "requestId": "57c71e10-5a03-407a-9a57-cd63dd50fd39",
  "endpoint_url": "\"https://postman-echo.com/get\"",
  "pod_name": "\"gateway-proxy-f46b58f89-5fkmd\"",
  "clientDuration": "85",
  "httpMethod": "GET",
  "upstreamName": "postman-echo_gloo-system",
  "responseCode": "200",
  "systemTime": "2019-11-01T17:30:36.178Z"
}
{{< /highlight >}}

As you can see, the access log entries now include the gateway pod name and the `url` attribute in the response body.

### Cleanup
To cleanup the resources created in this tutorial you can run the following commands:

```shell
kubectl delete virtualservice -n gloo-system test-access-logs
kubectl delete upstream -n gloo-system postman-echo
```