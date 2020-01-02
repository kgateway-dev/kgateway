---
title: Envoy API
description: Fine-grained rate limit API.
weight: 20
---

## Overview

In this document, we show how to use Gloo with [Envoy's rate-limit API](https://www.envoyproxy.io/docs/envoy/latest/api-v2/config/filter/http/rate_limit/v2/rate_limit.proto). We make the distinction here that this is "Envoy's" rate-limit API because Gloo [offers a much simpler rate-limit API](../simple) as an alternative.

Gloo Enterprise includes a rate limit server based on [Lyft's Envoy rate-limit server](https://github.com/lyft/ratelimit). It is already installed when doing `glooctl install gateway enterprise --license-key=...` or using the [Helm install]({{< versioned_link_path fromRoot="/installation/enterprise#installing-on-kubernetes-with-helm" >}}). To get your trial license key, go to <https://www.solo.io/gloo-trial>

Two steps are needed to configure Gloo to leverage the full Envoy Rate Limiter.

1. In the Gloo Settings manifest, you need to configure all of your [rate limiting descriptors](https://github.com/lyft/ratelimit#configuration). Descriptors describe your requests and are used to define the rate limits themselves.
2. For each Virtual Service, you need to configure [Envoy rate limiting actions](https://www.envoyproxy.io/docs/envoy/v1.9.0/api-v2/api/v2/route/route.proto#route-ratelimit-action) at the Virtual Service level, for each route, or both. These actions define the relationship between a request and its generated descriptors.

#### Descriptors

Rate limiting descriptors define a set (tuple) of keys that must match for the associated rate limit to be applied. The set of keys are expressed as a hierarchy to make configuration easy, but it's the set of keys matching or not that is important. Each descriptor key can have an associated value that is matched as a literal. If there is no value associated with a key, then each unique value is used for rate limiting. In essence, if one of the keys is `user_id` with no associated value then Envoy does per user rate limiting with each unique `user_id` value being used as the ID for rate limiting. See the [Lyft rate limiting descriptors](https://github.com/lyft/ratelimit#configuration) for full details.

For example, look at following descriptor:

```yaml
descriptors:
- key: account_id
  descriptors:
  - key: plan
    value: BASIC
    rateLimit:
      requestsPerUnit: 1
      unit: MINUTE
  - key: plan
    value: PLUS
    rateLimit:
      requestsPerUnit: 20
      unit: MINUTE
```

This descriptor will match any request with the `account_id` and `plan` keys, such that `('account_id', '<unique value>'), ('plan', 'BASIC | PLUS')`. Since the `account_id` key does not specify a descriptor value, it uses each unique value passed into the rate limiting service to match; i.e., per account rate limiting. The `plan` descriptor key has two values specified and depending on which one matches (`BASIC` or `PLUS`) determines the rate limit, either 1 request per minute for `BASIC` or 20 requests per minute for `PLUS`. You can specify multiple descriptors by nesting descriptors in other descriptors; the most specific match is what determines the rate limit.

The descriptors are defined in the Gloo Settings manifest:

{{< highlight yaml "hl_lines=11-12" >}}
apiVersion: gloo.solo.io/v1
kind: Settings
metadata:
  name: default
  namespace: gloo-system
spec:
  ratelimit:
    descriptors:
    - # list of descriptors here
{{< /highlight >}}

#### Actions

The [Envoy rate limiting actions](https://www.envoyproxy.io/docs/envoy/v1.9.0/api-v2/api/v2/route/route.proto#route-ratelimit-action) associated with the Virtual Service or the individual routes allow you to specify how parts of the request are associated to rate limiting descriptor keys. For example, you can use the `requestHeaders` configuration to pass the value of a request header in as the value of the associated rate limiting descriptor key. Or you can use the `genericKey` to pass in a literal value such as a route identifier to allow you to rate limit differently based on the matched route.

You can specify more than one rate limit action, and the request is throttled if any one of the actions triggers the rate limiting service to signal throttling, i.e., the rate limiting actions are effectively OR'd together.

An example of the rate limiting actions follows:

```yaml
rate_limits:
- actions:
  - requestHeaders:
      descriptorKey: account_id
      headerName: x-account-id
  - requestHeaders:
      descriptorKey: plan
      headerName: x-plan
```
This action says to pass in the request header values as the descriptor values. That is, the `x-account-id` request header is sent to the rate limiting service as the descriptor key `account_id`.

Rate limit actions can be specified both at the Virtual Service level and on a per route basis. For example, the look at the following:

{{< highlight yaml "hl_lines=19-28 37-45" >}}
apiVersion: gateway.solo.io/v1
kind: VirtualService
metadata:
  name: default
  namespace: gloo-system
spec:
  virtualHost:
    domains:
    - '*'
    routes:
    - matchers:
      - prefix: /service/service1
      routeAction:
        single:
          upstream:
            name: default-echo-server-8080
            namespace: gloo-system
      options:
        ratelimit:
          includeVhRateLimits: false
          rateLimits:
          - actions:
            - requestHeaders:
                descriptorKey: account_id
                headerName: x-account-id
            - requestHeaders:
                descriptorKey: plan
                headerName: x-plan-other
    - matchers:
      - prefix: /service/service2
      routeAction:
        single:
          upstream:
            name: default-echo-server-8080
            namespace: gloo-system
    options:
      ratelimit:
        rateLimits:
        - actions:
          - requestHeaders:
              descriptorKey: account_id
              headerName: x-account-id
          - requestHeaders:
              descriptorKey: plan
              headerName: x-plan
{{< /highlight >}}

{{% notice info %}}
The Envoy rate limiter uses an option with name `ratelimit`. The simpler Gloo rate limiter uses an option with name `ratelimitBasic`.
{{% /notice %}}

This virtual service sets up a default (virtual service level) rate limit action that maps header `x-account-id` to descriptor key `account_id` and `x-plan` to key `plan`. Any requests matching route prefix `/service/service2` use this rate limit action. Requests matching prefix `/service/service` route use a different rate limiting action that maps header `x-plan-other` to key `plan`. Note the route also has an `includeVhRateLimits: <boolean>` configuration that defaults to `false`. If `true`, any specified Virtual Service level rate limit actions would be added to (OR'd) the routes list of actions. Each rate limit action is matched to descriptors independently.

## Rate Limiting Example

Install the pet clinic demo app and configure a route to that service in Gloo

```bash
kubectl apply \
  --filename https://raw.githubusercontent.com/solo-io/gloo/v1.2.9/example/petclinic/petclinic.yaml

glooctl add route --name default --namespace gloo-system \
  --path-prefix / \
  --dest-name default-petclinic-8080 \
  --dest-namespace gloo-system
```

And check that everything is working as expected.

```bash
curl --head $(glooctl proxy url)
```

```http
HTTP/1.1 200 OK
content-type: text/html;charset=UTF-8
content-language: en
content-length: 3939
date: Sun, 17 Mar 2019 15:42:04 GMT
x-envoy-upstream-service-time: 13
server: envoy
```

### Configuring Envoy Rate Limits

Edit the rate limit server settings. This opens your default editor controlled by the `EDITOR` environment variable on most operating systems.

```bash
glooctl edit settings --namespace gloo-system --name default ratelimit server-config
```

That command opens the rate limit server configuration in your editor. Paste the following descriptor block into the editor. This descriptor uses `generic_key` key, which works with the `generic_key` rate limiting action in Envoy, effectively passing a literal value to the rate limiting service. For your convenience, you can download the descriptor block [here](serverconfig.yaml).

```yaml
descriptors:
  - key: generic_key
    rate_limit:
      requests_per_unit: 1
      unit: MINUTE
    value: some_value
```

The `glooctl` tool merges those descriptors into the Gloo Settings manifest as follows:

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
  ratelimit:
    descriptors:
    - key: generic_key
      rateLimit:
        requestsPerUnit: 1
        unit: MINUTE
      value: some_value
  ratelimitServer:
    ratelimitServerRef:
      name: rate-limit
      namespace: gloo-system
  refreshRate: 60s
{{< /highlight  >}}

### Edit Virtual Service Rate Limit Settings

Edit the virtual service settings:

```bash
glooctl edit virtualservice --namespace gloo-system --name default ratelimit client-config
```

That command opens the virtual service rate limit configuration in your editor. Paste the following rate limit action block into the editor. For your convenience, you can download the rate limiting action [here](vsconfig.yaml). The structure of the virtual service configuration is as described in the [Envoy documentation](https://www.envoyproxy.io/docs/envoy/v1.9.0/api-v2/api/v2/route/route.proto#route-ratelimit-action). This configuration will be passed to Envoy as is.

```yaml
rate_limits:
- actions:
  - generic_key:
      descriptor_value: "some_value"
```

{{% notice note %}}
You can run the same command for a *route* as well (`glooctl edit route ...`). When providing configuration for a `route`, you can also specify a boolean `include_vh_rate_limits` to include the rate limit actions from the virtual service.
{{% /notice %}}

### Test

Run the following a few times:

```bash
curl --head $(glooctl proxy url)
```

Eventually the curl response shows it is being rate limited:

```http
HTTP/1.1 429 Too Many Requests
x-envoy-ratelimited: true
date: Sun, 17 Mar 2019 15:42:17 GMT
server: envoy
transfer-encoding: chunked
```

## Other Configuration Options

Envoy queries an external server (backed by redis) to achieve global rate limiting. You can set a timeout for the
query, and what to do in case the query fails. By default, the timeout is set to 100ms, and the failure policy is
to allow the request.

{{% notice tip %}}
You can check if envoy has errors with rate limiting by examining its stats that end in `ratelimit.error`.
`glooctl proxy stats` displays the stats from one of the envoys in your cluster.
{{% /notice %}}

To change the timeout to 200ms, use the following command:

```bash
glooctl edit settings --name default --namespace gloo-system ratelimit --request-timeout=200ms
```

To deny requests when there's an error querying the rate limit service, use this command:

```bash
glooctl edit settings --name default --namespace gloo-system ratelimit --deny-on-failure=true
```

## Conclusion

With the custom rate-limit configuration option, you have the full power of Envoy rate limits at your disposal. The downside is that the API is a bit more complicated. To leverage a simpler API that can do true per-user (logged-in, authenticated user) rate limits, take a look at [Gloo's simplified ratelimit API](../simple).
