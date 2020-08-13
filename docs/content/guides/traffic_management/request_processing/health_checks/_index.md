---
title: Health Checks
weight: 50
description: Enable a health check plugin to respond with common HTTP codes
---

Gloo includes an HTTP health checking plugin that can be enabled in a {{< protobuf display="Gateway" name="gateway.solo.io.Gateway" >}} (which becomes an [Envoy Listener](https://www.envoyproxy.io/docs/envoy/latest/configuration/listeners/listeners)). This plugin will respond to health check requests directly with either a 200 OK or 503 Service Unavailable depending on the current draining state of Envoy.
 
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

## Configuration

Descriptions of the options available for configuring health checks can be found {{< protobuf name="envoy.api.v2.core.HealthCheck" display="here" >}}.

### Custom Headers for HttpHealthChecks

There are two ways to add custom headers to health check requests, both of which are shown in the example below.

{{< highlight yaml >}}
...
      healthCheck:
        requestHeadersToAdd:
          - header:
              key: example_name
              value: example_value
            append: true
          - headerSecretRef:
              name: example-name
              namespace: example-namespace
            append: true
{{< /highlight >}}

A `header` represents an explicitly-specified header where the key is the header name and the value is the header value. In contrast, a `headerSecretRef` points to headers contained in a Kubernetes secret. Each secret represents one or more (header name, header value) pairs that will all be added to the request. Secrets for this purpose can be [created with glooctl]({{< versioned_link_path fromRoot="/reference/cli/glooctl_create_secret_header" >}}). In both cases, the `append` field controls whether headers from the give source add to existing headers with the same name (`true`) or overwrite them (`false`).
