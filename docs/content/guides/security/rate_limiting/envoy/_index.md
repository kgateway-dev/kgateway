---
title: Envoy API
description: Fine-grained rate limit API.
weight: 10
---

## Table of Contents

- [Table of Contents](#table-of-contents)
- [Overview](#overview)
  - [Descriptors](#descriptors)
  - [Actions](#actions)
- [Simple Examples](#simple-examples)
  - [Generic Key](#generic-key)
  - [Header Values](#header-values)
  - [Remote Address](#remote-address)
- [Advanced Concepts](#advanced-concepts)
  - [Defining limits for tuples of key-value pairs](#defining-limits-for-tuples-of-key-value-pairs)
  - [Nested Limits](#nested-limits)
  - [Rule Priority and Weights](#rule-priority-and-weights)
  - [Customizing Routes](#customizing-routes)
- [Advanced Use Cases](#advanced-use-cases)
  - [Rate limiting by client IP](#rate-limiting-by-client-ip)
    - [Configuring Gloo Edge to properly forward the client address](#configuring-gloo-edge-to-properly-forward-the-client-address)
    - [Configuring multiple limits per remote address](#configuring-multiple-limits-per-remote-address)
  - [Traffic prioritization based on HTTP method](#traffic-prioritization-based-on-http-method)
  - [Securing rate limit actions with JWTs](#securing-rate-limit-actions-with-jwts)
  - [Improving security further with WAF and authorization](#improving-security-further-with-waf-and-authorization)

## Overview

Learn how to use Gloo Edge with [Envoy's rate-limit API](https://www.envoyproxy.io/docs/envoy/latest/configuration/http/http_filters/rate_limit_filter.html#).

{{% notice note %}}
This guide only includes Envoy-style rate limiting examples. To learn about other rate limiting options, install your environment, and set up the components that you need for rate limiting, see [Rate limiting setup]({{< versioned_link_path fromRoot="/guides/security/rate_limiting/setup/" >}}).
{{% /notice %}}

The Envoy API uses two components to define how rate limiting works. For more information on where to define these components in your Gloo Edge custom resources, see [Implement rate limiting]({{< versioned_link_path fromRoot="/guides/security/rate_limiting/setup/#implement" >}}).

1. [Rate limiting descriptors](https://github.com/envoyproxy/ratelimit#configuration) describe your requests and are used to define the rate limits themselves.
2. [Rate limiting actions](https://www.envoyproxy.io/docs/envoy/latest/api-v3/config/route/v3/route_components.proto#envoy-v3-api-msg-config-route-v3-ratelimit-action) define the relationship between a request and its generated descriptors. 

### Descriptors

Rate limiting descriptors define an ordered tuple of keys that must match for the associated rate limit to be applied. 
The tuple of keys are expressed as a hierarchy to make configuration easy, but it's the complete tuple of keys
matching or not that is important. Each descriptor key can have an associated value that is matched as a literal. 
You can define rate limits on a key matching a specific value, or you can omit the value to have the limit applied to 
any unique value for that key. 
See the Envoy rate limiting [configuration doc](https://github.com/envoyproxy/ratelimit#configuration) for full details.

Rate limit descriptors live in the Gloo Edge Settings manifest, so the examples below will reflect a Gloo Edge Settings configuration 
or patch. 

### Actions

The [Envoy rate limiting actions](https://www.envoyproxy.io/docs/envoy/v1.14.1/api-v2/api/v2/route/route_components.proto#envoy-api-msg-route-ratelimit-action) 
associated with the Virtual Service or the individual routes allow you to specify how parts of the request are 
associated to rate limiting descriptor keys defined in the settings. Essentially, these actions tell Gloo Edge which rate limit counters
to increment for a particular request. 

You can specify more than one rate limit action, and the request is throttled if any one of the actions triggers 
the rate limiting service to signal throttling, i.e., the rate limiting actions are effectively OR'd together.

## Simple Examples

Go through a series of simple rate limiting examples to understand the basic options for defining rate limiting descriptors and actions. Later, you can review more complex examples that use nested tuples of keys, to express more realistic use cases. 

### Generic Key

A generic key is a specific string literal that is used to match an action to a descriptor. 

1. {{< readfile file="static/content/rl-setup-before-you-begin" markdown="true">}}
2. Prepare the descriptor that describes your rate limit rule. The following example defines a limit of 1 request per minute for any request that triggers an action on the generic key called `per-minute`. 
   ```yaml
   descriptors:
     - key: generic_key
       value: "per-minute"
       rateLimit:
         requestsPerUnit: 1
         unit: MINUTE  
   ```
3. Prepare the action that matches the descriptor you just created. The following action matches on the `generic_key` descriptor key, as well as the `"per-minute"` descriptor key's value.
   ```yaml
   - actions:
       - genericKey:
           descriptorValue: "per-minute"
   ```
4. {{< readfile file="static/content/rl-setup-implement" markdown="true">}}
   {{< tabs >}} 
{{% tab name="Refer to RateLimitConfig (Enterprise-only)" %}}
1. {{< readfile file="static/content/rl-setup-rlc-step" markdown="true">}} 
   * [RateLimitConfig `rlc.yaml` example](https://github.com/solo-io/gloo-edge-use-cases/blob/main/docs/rate-limit/generic-key/rlc.yaml)
2. {{< readfile file="static/content/rl-setup-rlc-vs-ov" markdown="true">}}
   * {{< readfile file="static/content/rl-setup-rlc-vs-host" markdown="true">}}
     * [VirtualService `rlc-vs-host.yaml` example](https://github.com/solo-io/gloo-edge-use-cases/blob/main/docs/rate-limit/generic-key/rlc-vs-host.yaml#L20-L24)
   * {{< readfile file="static/content/rl-setup-rlc-vs-route" markdown="true">}}
     * [VirtualService `rlc-vs-route.yaml` example](https://github.com/solo-io/gloo-edge-use-cases/blob/main/docs/rate-limit/generic-key/rlc-vs-route.yaml#L15-L18)
{{% /tab %}} 
{{% tab name="Enter directly in resources" %}}
1. {{< readfile file="static/content/rl-setup-separate-settings" markdown="true">}}
   * [Settings `settings.yaml` example](https://github.com/solo-io/gloo-edge-use-cases/blob/main/docs/rate-limit/generic-key/settings.yaml#L62-L68)
2. {{< readfile file="static/content/rl-setup-separate-vs-ov" markdown="true">}}
   * {{< readfile file="static/content/rl-setup-separate-vs-host" markdown="true">}}
     * [VirtualService `vs-host.yaml` example](https://github.com/solo-io/gloo-edge-use-cases/blob/main/docs/rate-limit/generic-key/vs-host.yaml#L20-L25)
   * {{< readfile file="static/content/rl-setup-separate-vs-route" markdown="true">}}
     * [VirtualService `vs-route.yaml` example](https://github.com/solo-io/gloo-edge-use-cases/blob/main/docs/rate-limit/generic-key/vs-route.yaml#L15-L19)
{{% /tab %}} 
   {{< /tabs >}}
5. {{< readfile file="static/content/rl-setup-check-vs" markdown="true">}}
   ```
   kubectl describe vs default -n gloo-system
   ```
6. Verify that your rate limit works.
   1. Verify that you can reach your test app by sending a request.
      ```sh
      curl -v $(glooctl proxy url)/all-pets
      ```
      Example response:
      ```
      HTTP/1.1 200 OK
      ...
      [{"id":1,"name":"Dog","status":"available"},{"id":2,"name":"Cat","status":"pending"}]
      ```
   2. Repeat the request. Because the rate limiting rule that you set up limits the requests to 1 time per minute, the request is rate limited.
      ```sh
      curl -v $(glooctl proxy url)/all-pets
      ```
      Example response:
      ```
      HTTP/1.1 429 Too Many Requests
      ```
### Header Values

It may be desirable to create actions based on the value of a header, which is dynamic based on the request, rather than 
a generic key, that is static based on the route. The following configuration will define a descriptor that limits requests to 2 per minute
for any unique value for `type`:

```yaml
spec:
  ratelimit:
    descriptors:
      - key: type
        rateLimit:
          requestsPerUnit: 2
          unit: MINUTE
```

Now we can create a route that triggers a rate limit action for this descriptor:

{{< highlight yaml "hl_lines=18-24" >}}
apiVersion: gateway.solo.io/v1
kind: VirtualService
metadata:
  name: example
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
              name: default-example-80
              namespace: gloo-system
    options:
      ratelimit:
        rateLimits:
          - actions:
              - requestHeaders:
                  descriptorKey: type
                  headerName: x-type
{{< /highlight >}}

With this config, a rate limit of 2 per minute will be enforced for requests depending on the value of the `x-type` header, 
for every unique value. If we only wanted to enforce this limit for a specific value, we could write that value into our descriptor:

```yaml
spec:
  ratelimit:
    descriptors:
      - key: type
        value: example
        rateLimit:
          requestsPerUnit: 2
          unit: MINUTE
```

Now, requests that are routing using the virtual service above will be rate limited after 2 requests per second only if the
request includes a header `x-type: example`.

### Remote Address

A common use case is to rate limit based on client IP address, also referred to as the downstream remote address.

1. Define a descriptor called `remote_address` in the default Settings resource.
   1. Create a file with your rate limit descriptors.
      ```yaml
      cat > settings-ra-patch.yaml - <<EOF
      spec:
        ratelimit:
          descriptors:
            - key: remote_address
              rateLimit:
                requestsPerUnit: 2
                unit: MINUTE
      EOF
      ```
   2. Patch the default settings in the `gloo-system` namespace.
      ```sh
      kubectl patch -n gloo-system settings default --type merge --patch "$(cat settings-ra-patch.yaml)"
      ```

2. On the route, define an action to count against the remote address descriptor.
   {{< highlight yaml "hl_lines=18-22" >}}
   apiVersion: gateway.solo.io/v1
   kind: VirtualService
   metadata:
     name: example
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
                 name: default-example-80
                 namespace: gloo-system
       options:
         ratelimit:
           rateLimits:
             - actions:
                 - remoteAddress: {}
   {{< /highlight >}}

{{% notice warning %}}
You may need to make additional configuration changes to Gloo Edge in order for the `remote_address` value to be the real 
client IP address, and not an address that is internal to the Kubernetes cluster, or that is from a cloud load balancer. 
To address these, check out the advanced use case below. 
{{% /notice %}}

## Advanced Concepts

Now that you understand the basic ways to define descriptors and link those to rate limit actions on your routes, we can 
dig into some more advanced concepts. 

### Defining limits for tuples of key-value pairs

In the settings, you can define nested descriptors to start to express rules based on tuples instead of a single value. 

```yaml
spec:
  ratelimit:
    descriptors:
      - key: type
        descriptors:
          - key: number
            rateLimit:
              requestsPerUnit: 1
              unit: MINUTE
```

This rule enforces a limit of 1 request per minute for any unique combination of `type` and `number` values. We can define 
multiple actions on our routes to apply this rule:

{{< highlight yaml "hl_lines=18-27" >}}
apiVersion: gateway.solo.io/v1
kind: VirtualService
metadata:
  name: example
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
              name: default-example-80
              namespace: gloo-system
    options:
      ratelimit:
        rateLimits:
          - actions:
              - requestHeaders:
                  descriptorKey: type
                  headerName: x-type
              - requestHeaders:
                  descriptorKey: number
                  headerName: x-number
{{< /highlight >}}

If a request is routed using this virtual service, and the `x-type` and `x-number` headers are both present on the request, 
then it will be counted towards the limit we defined above. 
If one or both headers are not present on the request, then no rate limit will be enforced. 

{{% notice warning %}}
The order of actions must match the order of nesting in the descriptors. So in this example, if the actions were reversed, 
with the number action before the type action, then the request would not count towards the rate limit quota defined above. 
{{% /notice %}}

### Nested Limits

We can define limits at each level of a descriptor tuple. For instance, we may want to enforce the same limit if the type
is provided but the number is not:

```yaml
spec:
  ratelimit:
    descriptors:
      - key: type
        rateLimit: 
          requestsPerUnit: 1
          unit: MINUTE
        descriptors:
          - key: number
            rateLimit:
              requestsPerUnit: 1
              unit: MINUTE
```

This time, on our virtual service, we'll define actions for two separate rate limits - one that increments the counter 
for the `type` limit specifically, and another to increment the counter for the `type` and `number` pair, when present. 

{{< highlight yaml "hl_lines=18-31" >}}
apiVersion: gateway.solo.io/v1
kind: VirtualService
metadata:
  name: example
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
              name: default-example-80
              namespace: gloo-system
    options:
      ratelimit:
        rateLimits:
          - actions:
              - requestHeaders:
                  descriptorKey: type
                  headerName: x-type
          - actions:
              - requestHeaders:
                  descriptorKey: type
                  headerName: x-type
              - requestHeaders:
                  descriptorKey: number
                  headerName: x-number
{{< /highlight >}}

Note that we now have two different rate limits defined for this virtual service. One contributes to the counter for just `type`, 
if the `x-type` header is present. The other contributes to the counter for the `type` and `number` pair, if both headers are present. 
The request will result in a 429 rate limit response if either limit is reached. 

### Rule Priority and Weights

We may run into cases where we need fine-grained control over the order of how rules are evaluated. For example, consider
this set of descriptors:

```yaml
spec:
  ratelimit:
    descriptors:
      - key: type
        rateLimit: 
          requestsPerUnit: 1
          unit: MINUTE
        descriptors:
          - key: number
            rateLimit:
              requestsPerUnit: 10
              unit: MINUTE
```

If the type and number are both present on a request, we want the limit to be 10 per minute. However, with the virtual service
from above, we would observe a limit of 1 per minute - the request would be rate limited based on the first rule, for just matching
on the `type` descriptor. 

Starting in Gloo Edge 1.x, you can now specify weights on rules. For a particular request that has multiple sets of actions, 
it will evaluate each and then increment only the matching rules with the highest weight. By default, the weight is 0, so we could 
fix our config above by adding a weight to the nested descriptor:

```yaml
spec:
  ratelimit:
    descriptors:
      - key: type
        rateLimit: 
          requestsPerUnit: 1
          unit: MINUTE
        descriptors:
          - key: number
            weight: 1
            rateLimit:
              requestsPerUnit: 10
              unit: MINUTE
```

Based on the virtual service defined above, when a request has both the `x-type` and `x-number` headers, then it will evaluate both 
limits - the limit on type alone, and the limit on the combination of type and number. Since the latter has a higher weight, then 
only that counter will be incremented. In that way, requests with a unique `type` and `number` will be allowed 10 requests per minute, 
but requests that only have a type will be limited to 1 per minute. 

This logic can be bypassed by using the `alwaysApply` flag. So this configuration would behave equivalently to the example before we added
the weight:

```yaml
spec:
  ratelimit:
    descriptors:
      - key: type
        alwaysApply: true
        rateLimit: 
          requestsPerUnit: 1
          unit: MINUTE
        descriptors:
          - key: number
            weight: 1
            rateLimit:
              requestsPerUnit: 10
              unit: MINUTE
```

### Customizing Routes

So far, we have been configuring rate limit actions on our virtual services as an option under the `virtualHost`. 
Alternatively, we can define this as an option on the route:
 
{{< highlight yaml "hl_lines=18-33" >}}
apiVersion: gateway.solo.io/v1
kind: VirtualService
metadata:
  name: example
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
              name: default-example-80
              namespace: gloo-system
        # This is now indented to be defined on the route
        options:
          ratelimit:
            includeVhRateLimits: false
            rateLimits:
              - actions:
                  - requestHeaders:
                      descriptorKey: type
                      headerName: x-type
              - actions:
                  - requestHeaders:
                      descriptorKey: type
                      headerName: x-type
                  - requestHeaders:
                      descriptorKey: number
                      headerName: x-number
{{< /highlight >}}

Note that route-level configuration for rate limiting supports an additional parameter, `includeVhRateLimits`, that can be used
to ignore the host-level rate limits. 

## Advanced Use Cases

Here are a few more advanced cases that may be more representative of a realistic configuration. 

### Rate limiting by client IP

A common desire is to limit requests based on the IP address of the client that initiated it. We saw how the 
`remote_address` descriptor and action can be used to express such a rule. However, there were two limitations in the 
simple example above: 1) it relied on Envoy properly determining the remote address, which may require other configuration. 
And 2) it wasn't clear how we could express two different limits for the same descriptor - i.e. 10 per second OR 100 per minute. 

#### Configuring Gloo Edge to properly forward the client address

There are several changes we want to make to the configuration of Gloo Edge so that Envoy will honor and forward the 
remote address of the downstream client (utilizing common conventions around the `x-forwarded-for` header). First, we
can add a configuration to our HTTP connection manager settings to enable the use of remote address:

```yaml
spec:
  httpGateway:
    options:
      httpConnectionManagerSettings:
        useRemoteAddress: true
```

This configuration should live on your `Gateway` object, which manages http connection settings for the Envoy listener.  

{{% notice note %}}
To apply this as a patch, write it to a file called `patch.yaml`, then apply the patch with the following command: 
`kubectl patch -n gloo-system gateway gateway-proxy --type merge --patch "$(cat patch.yaml)"`

<br />

This assumes you are trying to patch the default http gateway in the gloo-system namespace.
{{% /notice %}}

Second, we may need to customize the way that Gloo Edge interacts with the Load Balancer so that Envoy receives a remote address
that is actually coming from outside the load balancer. In GKE, this can be done by patching the `gateway-proxy` service 
so that the `externalTrafficPolicy` is set to `Local`: 

```yaml
spec:
  externalTrafficPolicy: Local
```

{{% notice note %}}
To apply this as a patch, write it to a file called `patch2.yaml`, then apply the patch with the following command: 
`kubectl patch -n gloo-system svc gateway-proxy --type merge --patch "$(cat patch2.yaml)"`

<br />

This assumes you are trying to patch the gateway-proxy service in the gloo-system namespace.
{{% /notice %}}

#### Configuring multiple limits per remote address

Now, using the config from the simple example, we can use the `remote_address` descriptor to rate limit based on the real 
downstream client address. However, in practice, we may want to express multiple rules, such as a per-second and per-minute 
limit. 

We can model this by making `remote_address` a nested descriptor, and using a generic key that is distinct. For instance, 
we could model our settings like this:

```yaml
spec:
  ratelimit:
    descriptors:
      - key: generic_key
        value: "per-minute"
        descriptors:
          - key: remote_address
            rateLimit:
              requestsPerUnit: 20
              unit: MINUTE
      - key: generic_key
        value: "per-second"
        descriptors:
          - key: remote_address
            rateLimit:
              requestsPerUnit: 2
              unit: SECOND
```

And we can configure a route to count towards both limits:

{{< highlight yaml "hl_lines=18-28" >}}
apiVersion: gateway.solo.io/v1
kind: VirtualService
metadata:
  name: example
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
              name: default-example-80
              namespace: gloo-system
        options:
          ratelimit:
            rateLimits:
              - actions:
                  - genericKey:
                      descriptorValue: "per-minute"
                  - remoteAddress: {}
              - actions:
                  - genericKey:
                      descriptorValue: "per-second"
                  - remoteAddress: {}
{{< /highlight >}}

Now, we'll increment a per-minute and per-second rate limit counter based on the client remote address. 

### Traffic prioritization based on HTTP method

A useful tactic for building resilient, distributed systems is to implement different rate limits for different "priorities" or "classes" of traffic. This practice is strongly related to the concept of [_load shedding_](https://landing.google.com/sre/workbook/chapters/managing-load/).

Suppose you have exposed an API that supports both `GET` and `POST` methods for listing data and creating  resources respectively. While both pieces of functionality are important, ultimately the `POST` action is more important to your business, so you want to protect the availability of the `POST` function at the expense of the less important `GET` function.

To implement this, we will build on the previous example and provide a global limit per remote client for all traffic classes as well a smaller limit for the less important `GET` method. This allows our system to drop the lower priority traffic and protect the higher priority traffic.

We can implement this in Gloo Edge using a descriptor for the method of incoming request in conjunction with the remote client:

```yaml
spec:
  ratelimit:
    descriptors:
    # allow 5 calls per minute for any unique host
    - key: remote_address
      rateLimit:
        requestsPerUnit: 5
        unit: MINUTE
    # specifically limit GET requests from unique hosts to 2 per min
    - key: method
      value: GET
      descriptors:
      - key: remote_address
        rateLimit:
          requestsPerUnit: 2
          unit: MINUTE
```

With these limits in place, we are ensuring that the server doesn't get overwhelmed with `GETs`. Now we can add an action to extract the method from the `:method` psuedo-header:

{{< highlight yaml "hl_lines=18-28" >}}
apiVersion: gateway.solo.io/v1
kind: VirtualService
metadata:
  name: example
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
              name: default-example-80
              namespace: gloo-system
        options:
          ratelimit:
            rateLimits:
              - actions:
                - remoteAddress: {}
              - actions:
                - requestHeaders:
                    descriptorKey: method
                    headerName: :method
                - remoteAddress: {}
{{< /highlight >}}

How the route will have a per-client limit for general protection while a smaller limit is in place for `GET` requests to prevent lower priority traffic from overwhelming the system.

### Securing rate limit actions with JWTs 

{{% notice note %}}
The JWT filter used below is an Enterprise feature. 
{{% /notice %}}

Using headers is a convenient way to determine values for rate limit actions, but it shouldn't be considered secure 
unless extra care is taken to ensure the headers are defined by a trusted authority. A good solution for this is to 
encode the values as claims in a JWT that is passed on in the request. In the JWT configuration, you can specify 
the headers to be derived from those extracted claims after the JWT has been verified. Then you can provide users 
with a secure method of acquiring a JWT, such as through an auth negotiation with a trusted identity provider. 

Let's assume these are our descriptors in Gloo Edge settings:
```yaml
spec:
  ratelimit:
    descriptors:
      - key: type
        rateLimit: 
          requestsPerUnit: 1
          unit: MINUTE
        descriptors:
          - key: number
            weight: 1
            rateLimit:
              requestsPerUnit: 10
              unit: MINUTE
```

Here's an example of a virtual service that takes rate limiting actions based on claims extracted from a JWT after 
verification:

{{< highlight yaml "hl_lines=18-44" >}}
apiVersion: gateway.solo.io/v1
kind: VirtualService
metadata:
  name: example
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
              name: default-example-80
              namespace: gloo-system
    options:
      jwt:
        providers:
          solo:
            tokenSource:
              headers:
                - header: x-token
              queryParams:
                - token
            claimsToHeaders:
              - claim: type
                header: x-type
              - claim: number
                header: x-number
            issuer: solo.io
            jwks:
              local:
                key: |
                  -----BEGIN PUBLIC KEY-----
                  MIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKCAQEAxdil+UiTSKYKV90YkeZ/
                  9CWvb4XfUgqYDeW/OG1Le+/BvSVkAFc1s3Fg0l9Zo4yvS4OGQszsNGJNl1mYya/L
                  sSTTD7suKLXY7FBTaBB8CQvvW873yRij1F4EaygOJ1ujuFmpgBGvQLSS5rceNaCl
                  Qzo+bmf3z0UGbhCxgsjDsJK2/aW2D/3dep/kF1GyEOGz8fewnwVp0zVyuS4UMidV
                  2cdnAobX2GvPwpjAeIeqhHG3HX4fen+TwU8rkej3y4efKHNj/GbKQmtt2EoOhEox
                  iK/JALlhQNAJqYn+81amRM7wGWeLEByt0+mwyAfnShOr6MFwrhQjsm4orwAx7yHB
                  jwIDAQAB
                  -----END PUBLIC KEY-----
      ratelimit:
        rateLimits:
          - actions:
              - requestHeaders:
                  descriptorKey: type
                  headerName: x-type
          - actions:
              - requestHeaders:
                  descriptorKey: type
                  headerName: x-type
              - requestHeaders:
                  descriptorKey: number
                  headerName: x-number
{{< /highlight >}}

The virtual service looks the same as before, but now we have an additional JWT configuration section that extracts 
the `x-type` and `x-header` claims from the verified JWT. If an invalid JWT was provided, the request would be considered
invalid. Otherwise, the request will be rate limited just like before when the header values came from the user directly.

### Improving security further with WAF and authorization

{{% notice note %}}
The WAF and auth filters used below are Enterprise features. 
{{% /notice %}}

Now that we are using JWT verification, we can be confident that the request is from an authenticated user, who has been granted
claims from a trusted identity provider. Using those claims provides us confidence that rate limiting determinations will be made
based on this trusted data. However, we want to further enhance security. 

First, we'll add a Web Application Firewall (WAF) configuration, in order to protect our proxy, auth, and rate limit server 
 against DOS or other types of malicious 
or destructive traffic. Gloo Edge exposes this as another option when configuring routes, and provides the powerful modsecurity 
rule set and language to define WAF behavior in Envoy. In this example, we'll just use a very simple rule to show how 
it can be wired up. 

Second, we'll add an authorization check that will block a few types of requests altogether. Our example from before would not 
apply rate limits to requests that had no `x-type` at all. We also may want to explicitly block some specific values. 

Let's add both of these options to our route by modifying our virtual service:

{{< highlight yaml "hl_lines=58-67" >}}
apiVersion: gateway.solo.io/v1
kind: VirtualService
metadata:
  name: example
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
              name: default-example-80
              namespace: gloo-system
    options:
      jwt:
        providers:
          solo:
            tokenSource:
              headers:
                - header: x-token
              queryParams:
                - token
            claimsToHeaders:
              - claim: type
                header: x-type
              - claim: number
                header: x-number
            issuer: solo.io
            jwks:
              local:
                key: |
                  -----BEGIN PUBLIC KEY-----
                  MIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKCAQEAxdil+UiTSKYKV90YkeZ/
                  9CWvb4XfUgqYDeW/OG1Le+/BvSVkAFc1s3Fg0l9Zo4yvS4OGQszsNGJNl1mYya/L
                  sSTTD7suKLXY7FBTaBB8CQvvW873yRij1F4EaygOJ1ujuFmpgBGvQLSS5rceNaCl
                  Qzo+bmf3z0UGbhCxgsjDsJK2/aW2D/3dep/kF1GyEOGz8fewnwVp0zVyuS4UMidV
                  2cdnAobX2GvPwpjAeIeqhHG3HX4fen+TwU8rkej3y4efKHNj/GbKQmtt2EoOhEox
                  iK/JALlhQNAJqYn+81amRM7wGWeLEByt0+mwyAfnShOr6MFwrhQjsm4orwAx7yHB
                  jwIDAQAB
                  -----END PUBLIC KEY-----
      ratelimit:
        rateLimits:
          - actions:
              - requestHeaders:
                  descriptorKey: type
                  headerName: x-type
          - actions:
              - requestHeaders:
                  descriptorKey: type
                  headerName: x-type
              - requestHeaders:
                  descriptorKey: number
                  headerName: x-number
      waf:
        ruleSets:
          - ruleStr: |
              # Turn rule engine on
              SecRuleEngine On
              SecRule REQUEST_HEADERS:User-Agent "scammer" "deny,status:403,id:107,phase:1,msg:'blocked scammer'"
      extauth:
        configRef:
          name: opa-auth
          namespace: gloo-system
{{< /highlight >}}

Note that we have an inline WAF rule that blocks requests with a suspicious `scammer` user agent. We could expand on 
this to support things like IP whitelisting or other common WAF use cases.  

We also added an `extauth` config that references `opa-auth`, an AuthConfig CRD:

```yaml
apiVersion: enterprise.gloo.solo.io/v1
kind: AuthConfig
metadata:
  name: opa-auth
  namespace: gloo-system
spec:
  configs:
    - opaAuth:
        modules:
          - name: allow-jwt
            namespace: gloo-system
        query: "data.test.allow == true"
```

And we can define an OPA policy in the `allow-jwt` config map to block requests that are missing a `type` claim, or that 
have the `SMS` type:

```yaml
apiVersion: v1
data:
  opa-policy.rego: |-
    package test
    default allow = false
    allow {
        [header, payload, signature] = io.jwt.decode(input.http_request.headers["x-token"])
        payload["type"] != "SMS"
        payload["type"] != ""
    }
kind: ConfigMap
metadata:
  name: allow-jwt
  namespace: gloo-system
```

Let's see what happens after deploying these configurations. First, requests with a scammer `User-Agent` are rejected 
with a modsecurity intervention:
```bash
curl $(glooctl proxy url)/ -H "User-Agent: scammer" -i
```

```
HTTP/1.1 403 Forbidden
content-length: 33
content-type: text/plain
date: Thu, 30 Apr 2020 22:42:31 GMT
server: envoy

ModSecurity: intervention occured%
```

Requests without a JWT are blocked:
```bash
curl $(glooctl proxy url)/ -i
```

```
HTTP/1.1 403 Forbidden
date: Thu, 30 Apr 2020 22:45:15 GMT
server: envoy
content-length: 0
```

Requests with a JWT that has an invalid signature are blocked:
```bash
curl $(glooctl proxy url)/ -i -H "x-token: eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9.eyJpc3MiOiJzb2xvLmlvIiwic3ViIjoiMTIzNDU2Nzg5MCIsInR5cGUiOiJNZXNzZW5nZXIifQ.invalid"
```

```
HTTP/1.1 401 Unauthorized
content-length: 22
content-type: text/plain
date: Thu, 30 Apr 2020 22:47:05 GMT
server: envoy

Jwt verification fails%
```

Requests with a valid JWT that has the `SMS` type as a claim are blocked:
```bash
curl $(glooctl proxy url)/ -i -H "x-token: eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9.eyJpc3MiOiJzb2xvLmlvIiwic3ViIjoiMTIzNDU2Nzg5MCIsInR5cGUiOiJTTVMifQ.EviHtVB1mQry8vxraE0tHnuaiaQ_6BWrJmNnyfHmpZWNwWf-RMrljP7KqpuNGtAysFb3YVhsacG0OZ_Ax1A0SBNicsahqlCrTSjIz48Tdz1PNAoGxi9ehLAsqjMMNV05z13YFQHucF1BJ6qaHftVA0AMDm5zXiiMPnW21NNdCghieTiAsyDSS_YXPLV8EfSW8rg0qsb3SvEsEUnF6rR5Ls3jXB-l2hFvRsRHuF4Y79mb_tAYnejoaB4qmVfqy2y9_do-oBnJkI71kfHoF1O-cFT01XOxf5noNvxORQ3GtV3YrWb4fvowWZAPR3Iq4iXkfKnZ2yldth3xrnsVymA3NA"
```

```
HTTP/1.1 403 Forbidden
date: Thu, 30 Apr 2020 22:49:03 GMT
server: envoy
content-length: 0
```

Requests with a valid JWT that has no type in the claims are blocked:
```bash
curl $(glooctl proxy url)/ -i -H "x-token: eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9.eyJpc3MiOiJzb2xvLmlvIiwic3ViIjoiMTIzNDU2Nzg5MCJ9.ILN6momkxhY01adwnClJfuSqoht5WjNTBjInzT6nx9LN8WkSk3Tg2UrdLTTZ-LNjh805Ltthg5Mt4IRGr5lZ_0_atUBlS-vzC9fNeIsIJIbt8Apl-JnC6LrKFU6EFC9okQyBRyDLhwclDTaLbqY2txK8maaFtlvlr5UY0QvopNSkaTc-lJlCSQf63v_W_VkZrISQAVhYb0r0ptWv6zN0NNe5Fzcw45xjuNUtjMSdI4zDNuc-QuNIs6CKX-iN3FR0zE50Y__T182xVhG78Q0v-jGGQMMtuTLwmn5PCdpQbBoGzVATwuAFiA7WE0M8KDRUmMDJYjWSOBqymfh9WkbSGw"
```

```
HTTP/1.1 403 Forbidden
date: Thu, 30 Apr 2020 22:49:54 GMT
server: envoy
content-length: 0
```

Requests with a valid JWT that has an allowed type (`Messenger`) are rate limited after 1 request:
```bash
curl $(glooctl proxy url)/ -i -H "x-token: eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9.eyJpc3MiOiJzb2xvLmlvIiwic3ViIjoiMTIzNDU2Nzg5MCIsInR5cGUiOiJNZXNzZW5nZXIifQ.gKko2fEw6Jlef3pQzQyk9ygz1Gz7gtyd0TzjHtC7ZSbv_8zUlQCP_AI5dCNQJOcy64pLHN_JB0uBJpbCfPkee01zlKEPaPUVFtBUIcdm9ZwB2J9scuRhdF5A8cU5nv3CwKzJFohKMxxhM29kojC8pG5XhI4l3oJ0elf2TN1YoMm8yc0lEYARBRR6gQMfukydHQFodvS0_hHi35LviM16-fEksjjchOTtHs9XxsXKLAmI00T5JwPzYTyHWYcQGnNHZiXr6-K809NyR-jPwdPCktxYvDkl9muZi1SQ5q779WmgZE-SvGfJNkjkld1LI3Ed_ASmgBwboHLxlcyoge3_tA"
```

```
HTTP/1.1 429 Too Many Requests
x-envoy-ratelimited: true
date: Thu, 30 Apr 2020 22:54:01 GMT
server: envoy
content-length: 0
```

Finally, requests with a valid JWT that has an allowed type and a number are rate limited after 10 requests:
```bash 
curl $(glooctl proxy url)/ -i -H "x-token: eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9.eyJpc3MiOiJzb2xvLmlvIiwic3ViIjoiMTIzNDU2Nzg5MCIsInR5cGUiOiJNZXNzZW5nZXIiLCJudW1iZXIiOiIxMjMifQ.xN3_MQU6NXHlG2QuAhZsWZlgZLb0htsitPwbsQeilxWAHer7k77zwT59Dehjw1VevYeyMAydu_gIhrXDu6_9keYDbhbd747SOmxNMLdPAnBSPNzukMckALFiG8HqNYeG_dh6ilWfuNZH_GvvMtDfbVqUDiLWepeK-AGZtztYh9CL8xMzqSe26Xh_2UnMQ1oj6hA11BHm0nsZoT1rxIhWAX7NF8RxH27GdzSNeikNN7_5VTbQpUifCTIjs6suvAktXY0y8KU2xEX6Pi2UuE5lYAgCDKe8BCb8QSquYxRQ3qhbPs2fIbwDEmSyueGd38apU4nQ1ryy-KxlsuuH4-ETuA"
```

```
HTTP/1.1 429 Too Many Requests
x-envoy-ratelimited: true
date: Thu, 30 Apr 2020 22:55:59 GMT
server: envoy
content-length: 0
```

As we can now see, by taking advantage of other Gloo Edge security features, we can ensure rate limits are enforced
while also securing routes against any kind of request that we didn't target with our rate limiting actions.