---
title: Health Checks
weight: 50
description: Enable a health check plugin to respond with common HTTP codes
---

Gloo Edge includes an HTTP health checking plugin that can be enabled in a {{< protobuf display="Gateway" name="gateway.solo.io.Gateway" >}} (which becomes an [Envoy Listener](https://www.envoyproxy.io/docs/envoy/latest/configuration/listeners/listeners)). This plugin will respond to health check requests directly with either a 200 OK or 503 Service Unavailable depending on the current draining state of Envoy.
 
Envoy can be forced into a draining state by sending an `HTTP GET` to the Envoy admin port on `<envoy-ip>:<admin-addr>/healthcheck/fail`. This port defaults to `19000`. 

To add the health check to a gateway, add the `healthCheck` stanza to the Gateway's `options`, like so:

{{< highlight yaml "hl_lines=9-12" >}}
apiVersion: gateway.solo.io/v1
kind: Gateway
metadata:
  name: gateway-proxy
  namespace: gloo-system
spec:
  bindAddress: '::'
  bindPort: 8080
  httpGateway:
    options:
      healthCheck:
        path: /any-path-you-want
{{< /highlight >}}

The HTTP Path of health check requests must be an *exact* match to the provided `healthCheck.path` variable.
