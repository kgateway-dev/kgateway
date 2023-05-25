---
title: Upstream Health Checks
weight: 50
description: Automatically monitor the status of Upstreams by configuring health checks for them
---

As part of configuring an Upstream, Gloo Edge provides the option of adding *health checks* that periodically assess the readiness of the Upstream to receive requests. See the [Envoy documentation](https://www.envoyproxy.io/docs/envoy/v1.14.1/intro/arch_overview/upstream/health_checking#arch-overview-health-checking) for more information. 

{{% notice note %}}
Upstreams with working health checks will not be removed from Envoy's service directory, even due to configuration changes. To allow them to be removed, set `ignoreHealthOnHostRemoval` in the Upstream's configuration.
{{% /notice %}}
## Configuration

Descriptions of the options available for configuring health checks can be found {{< protobuf name="solo.io.envoy.api.v2.core.HealthCheck" display="here" >}}.

### Custom paths for HttpHealthChecks

There is a way to add custom paths to health check requests shown in the example below.

{{< highlight yaml >}}
spec:
  healthChecks:
  - healthyThreshold: 1
    httpHealthCheck:
      path: /check/healthz
    interval: 30s
    timeout: 10s
    unhealthyThreshold: 1
{{< /highlight >}}

A `path` represents an explicitly-specified path to check the health of the upstream. The `timeout` declares how much time between checks there should be. An `unhealthyThreshold` is the limit of checks that are allowed to fail before declaring the upstream unhealthy. A `healthyThreshold` is the limit of checks that are allowed to pass before declaring an upstream healthy. The `interval` is the interval of time that you send `healthchecks` as to not overload your upstream service. 
