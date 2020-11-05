---
title: Envoy Gzip filter with Gloo Edge
weight: 70
description: Using Gzip filter in Envoy with Gloo Edge
---

Gzip is an HTTP option which enables Gloo Edge to compress data returned from an upstream service upon client request.
Compression is useful in situations where large payloads need to be transmitted without compromising the response time.

This guide assumes you already have Gloo Edge installed.  
Support for the Envoy Gzip filter was added to Open Source Gloo Edge as of version 1.3.4 and to Gloo Edge Enterprise as of version 1.3.0-beta2.


## Configuration

To get started with Gzip, modify the gateway and change the `httpGateway` object to include the gzip option. For example:
```shell
kubectl patch gateway -n gloo-system gateway-proxy --type merge -p '{"spec":{"httpGateway":{"options":{"gzip":{"compressionLevel":"BEST","contentType":["text/plain"]}}}}}'
```

Here's an example of an edited gateway:
{{< highlight yaml "hl_lines=11-16" >}}
apiVersion: gateway.solo.io/v1
kind: Gateway
metadata:
  labels:
    app: gloo
  name: gateway-proxy
  namespace: gloo-system
spec:
  bindAddress: '::'
  bindPort: 8080
  httpGateway:
    options:
      gzip:
        compressionLevel: BEST
        contentType:
        - text/plain
  proxyNames:
  - gateway-proxy
  useProxyProto: false
{{< /highlight >}}

Once that is saved, you're all set. Traffic on the http gateway will call the gzip filter.

You can learn about the configuration options [here]({{< versioned_link_path fromRoot="/reference/api/github.com/solo-io/gloo/projects/gloo/api/external/envoy/config/filter/http/gzip/v2/gzip.proto.sk" >}}).

More information about the Gzip filter can be found in the [relevant Envoy docs](https://www.envoyproxy.io/docs/envoy/latest/configuration/http/http_filters/gzip_filter). If data is not being compressed, you may want to check that all the necessary conditions for the Envoy filter are met.
See the [How it works](https://www.envoyproxy.io/docs/envoy/latest/configuration/http/http_filters/gzip_filter#how-it-works)
section for information on when compression will be skipped.

## Example

Let's see Gzip compression in action.

First let's add the following Virtual Service:
```yaml
apiVersion: gateway.solo.io/v1
kind: VirtualService
metadata:
  name: sample-vs
  namespace: gloo-system
spec:
  virtualHost:
    domains:
      - '*'
    routes:
      - matchers:
        - exact: /helloworld
        directResponseAction:
          status: 200
          body: "Hello, world! It's me. I've been wondering if after all these years you'd like to meet."
```

This can be added by copying the Virtual Service definition and using the `pbpaste | kubectl apply -f -` command
or by saving the Virtual Service definition to a file and using `kubectl apply -f myFile.yaml`, among other options.

Now, we'll send a request to the route referenced in our virtual service:
```shell
curl -v $(glooctl proxy url)/helloworld -H "Accept-Encoding: gzip"
```
You should see that the response is in plain text.

Now edit the gateway as described [above](#configuration).

If we send the same request:
```shell
curl -v $(glooctl proxy url)/helloworld -H "Accept-Encoding: gzip"
```
We now see that the response includes the header `content-encoding: gzip` and that the body is binary.
