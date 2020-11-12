---
title: Upstream Health Checks
weight: 50
description: Automatically monitor the status of Upstreams by configuring health checks for them
---

As part of configuring an Upstream, Gloo Edge provides the option of adding *health checks* that periodically assess the readiness of the Upstream to receive requests. See the [Envoy documentation](https://www.envoyproxy.io/docs/envoy/v1.14.1/intro/arch_overview/upstream/health_checking#arch-overview-health-checking) for more information. 

## Configuration

Descriptions of the options available for configuring health checks can be found {{< protobuf name="envoy.api.v2.core.HealthCheck" display="here" >}}.

### Custom Headers for HttpHealthChecks

There are two ways to add custom headers to health check requests, both of which are shown in the example below.

{{< highlight yaml >}}
...
  healthCheck:
    httpHealthCheck:
      requestHeadersToAdd:
        - header:
            key: example-name
            value: example-value
          append: true
        - headerSecretRef:
            name: example-name
            namespace: example-namespace
          append: true
{{< /highlight >}}

A `header` represents an explicitly-specified header where the key is the header name and the value is the header value. In contrast, a `headerSecretRef` points to headers contained in a Kubernetes secret. Each secret represents one or more (header name, header value) pairs that will all be added to the request. Secrets for this purpose can be [created with glooctl]({{< versioned_link_path fromRoot="/reference/cli/glooctl_create_secret_header" >}}). In both cases, the `append` field controls whether headers from the give source add to existing headers with the same name (`true`) or overwrite them (`false`).
