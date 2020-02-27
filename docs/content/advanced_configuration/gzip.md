---
title: Envoy Gzip filter with Gloo
weight: 70
description: Using Gzip filter in Envoy with Gloo
---

This guide assumes you already have Gloo installed.

## Configuration

To get started with Gzip, modify the gateway:
```shell
kubectl edit gateway -n gloo-system gateway-proxy
```

and change the `httpGateway` object to include the gzip option. For example:
```yaml
  httpGateway:
    options:
      gzip:
        compressionLevel: BEST
        contentType:
        - text/plain
```

Once that is saved, you're all set. Traffic on the http gateway will call the gzip filter.

You can learn about the configuration options [here]({{< versioned_link_path fromRoot="/reference/api/github.com/solo-io/gloo/projects/gloo/api/external/envoy/config/filter/http/gzip/v2/gzip.proto.sk" >}}).

More information about the Gzip filter can be found in the [relevant Envoy docs](https://www.envoyproxy.io/docs/envoy/latest/configuration/http/http_filters/gzip_filter).  
If data is not being compressed, you may want to check that all the necessary conditions for the Envoy filter are met.
See the [How it works](https://www.envoyproxy.io/docs/envoy/latest/configuration/http/http_filters/gzip_filter#how-it-works)
section for information on when compression will be skipped.

## Example

Let's see Gzip compression in action.

First let's add a Virtual Service:
```shell
kubectl apply -f - <<EOF
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
EOF
```

Now, we'll send a request to the route referenced in our virtual service:
```shell
curl -v $(glooctl proxy url)/helloworld -H "Accept-Encoding: gzip"
```
You should see that the response is in plain text.

Now edit the gateway as described [above]({{< versioned_link_path fromRoot="/advanced_configuration/gzip/#configuration" >}}).

If we send the same request:
```shell
curl -v $(glooctl proxy url)/helloworld -H "Accept-Encoding: gzip"
```
We now see that the response includes the header `content-encoding: gzip` and that the body is binary.
