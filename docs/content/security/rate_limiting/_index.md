---
title: Rate Limiting
weight: 20
description: Control the rate of traffic sent to your services.
---

#### Why Rate Limit in API Gateway Environments
API Gateways act as a control point for the outside world to access the various application services 
(monoliths, microservices, serverless functions) running in your environment. In microservices or hybrid application 
architecture, any number of these workloads will need to accept incoming requests from external end users (clients). 
Incoming requests can be numerous and varied -- protecting backend services and globally enforcing business limits 
can become incredibly complex being handled at the application level. Using an API gateway we can define client
request limits to these varied services in one place.

#### Rate Limiting in Gloo

Gloo exposes Envoy's rate-limit API, which allows users to provide their own implementation of an Envoy gRPC rate-limit
service. Lyft provides an example implementation of this gRPC rate-limit service 
[here](https://github.com/lyft/ratelimit). To configure Gloo to use your rate-limit server implementation,
install Gloo gateway with helm and provide the following `.Values.settings.extensions` values override:

```yaml
configs:
  rate-limit:
    ratelimit_server_ref:
      name: ...      # rate-limit upstream name
      namespace: ... # rate-limit upstream namespace
    request_timeout: ...  # optional, default 100ms
    deny_on_fail: ...     # optional, default false
    rate_limit_before_auth: ... # optional, default false
```

{{% notice note %}}
Setting the value `rate_limit_before_auth` to true will cause the rate limiting filter to run before the Ext Auth filter.
This necessarily means the loss of Ext Auth-aware rate limiting features, like providing different rate limits for authenticated
vs non-authenticated users.
{{% /notice %}}

Gloo Enterprise provides an enhanced version of [Lyft's rate limit service](https://github.com/lyft/ratelimit) that
supports the full Envoy rate limit server API, as well as a simplified API built on top of this service. Gloo uses
this rate-limit service to enforce rate-limits. The rate-limit service can work in tandem with the Gloo external auth
service to define separate rate-limit policies for authorized & unauthorized users. The Gloo Enteprise rate-limit service
enabled and configured by default, no configuration is needed to point Gloo toward the rate-limit service.

### Rate Limit Configuration

Check out the guides for each of the Gloo rate-limit APIs and configuration options for Gloo Enterprise's rate-limit
service:

{{% children description="true" %}}
