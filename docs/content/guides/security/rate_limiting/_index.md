---
title: Global rate limiting
weight: 30
description: Control the rate of traffic sent to your services.
---

#### Why Rate Limit in API Gateway Environments
API Gateways act as a control point for the outside world to access the various application services
(monoliths, microservices, serverless functions) running in your environment. In microservices or hybrid application
architecture, any number of these workloads will need to accept incoming requests from external end users (clients).
Incoming requests can be numerous and varied -- protecting backend services and globally enforcing business limits
can become incredibly complex being handled at the application level. Using an API gateway we can define client
request limits to these varied services in one place.

#### Rate Limiting in Gloo Edge

Gloo Edge exposes Envoy's rate-limit API, which allows users to provide their own implementation of an Envoy gRPC rate-limit
service. Lyft provides an example implementation of this gRPC rate-limit service
[here](https://github.com/lyft/ratelimit). To configure Gloo Edge to use your rate-limit server implementation,
install Gloo Edge gateway and then modify the settings to use your rate limit server upstream:

Open editor to modify the settings:
```shell script
kubectl --namespace gloo-system edit settings default
```

Update the highlighted portion to point to your rate limit server:
{{< highlight yaml "hl_lines=24-30" >}}
apiVersion: gloo.solo.io/v1
kind: Settings
metadata:
  labels:
    app: gloo
    gloo: settings
  name: default
  namespace: gloo-system
spec:
  discoveryNamespace: gloo-system
  extauth:
    extauthzServerRef:
      name: extauth
      namespace: gloo-system
  gateway:
    validation:
      alwaysAccept: true
      proxyValidationServerAddr: gloo:9988
  gloo:
    xdsBindAddr: 0.0.0.0:9977
  kubernetesArtifactSource: {}
  kubernetesConfigSource: {}
  kubernetesSecretSource: {}
  ratelimitServer:
    ratelimitServerRef:
      name: ...        # rate-limit server upstream name
      namespace: ...   # rate-limit server upstream namespace
    requestTimeout: ...      # optional, default 100ms
    denyOnFail: ...          # optional, default false
    rateLimitBeforeAuth: ... # optional, default false
    enableXRatelimitHeaders: ... # optional, default false
  refreshRate: 60s
{{< /highlight  >}}

{{% notice note %}}
Setting the value `rateLimitBeforeAuth` to true will cause the rate limiting filter to run before the Ext Auth filter.
This necessarily means the loss of extauth-aware rate limiting features, like providing different rate limits for authenticated
vs non-authenticated users.
{{% /notice %}}

Gloo Edge Enterprise provides an enhanced version of [Lyft's rate limit service](https://github.com/lyft/ratelimit) that
supports the full Envoy rate limit server API (with some additional enhancements, e.g. rule priority), as well as a
simplified API built on top of this service. Gloo Edge uses this rate-limit service to enforce rate-limits. The rate-limit
service can work in tandem with the Gloo Edge external auth service to define separate rate-limit policies for authorized &
unauthorized users. The Gloo Edge Enteprise rate-limit service is enabled and configured by default, no configuration is needed
to point Gloo Edge toward the rate-limit service.

### Logging

If Gloo Edge is running on kubernetes, the rate limiting logs can be viewed with:
```
kubectl logs -n gloo-system deploy/rate-limit -f
```

When it starts up correctly, you should see a log line similar to:
```
"caller":"server/server_impl.go:48","msg":"Listening for HTTP on ':18080'"
```

### Rate Limit Configuration

#### Enabling Envoy's X-RateLimit Headers
Setting the value `enableXRatelimitHeaders` to true will configure Envoy to return the headers defined in their [rate limit API](https://www.envoyproxy.io/docs/envoy/latest/api-v3/extensions/filters/http/ratelimit/v3/rate_limit.proto.html#envoy-v3-api-field-extensions-filters-http-ratelimit-v3-ratelimit-enable-x-ratelimit-headers)
to the downstream.

Check out the guides for each of the Gloo Edge rate-limit APIs and configuration options for Gloo Edge Enterprise's rate-limit
service:

{{% children description="true" %}}
