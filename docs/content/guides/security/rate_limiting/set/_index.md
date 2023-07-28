---
title: Set-Style API (Enterprise)
description: Set-style rate limiting API to better limit on descriptor subsets
weight: 15
---

{{% notice note %}}
Set-style rate limiting was introduced with **Gloo Edge Enterprise**, release `v1.6.0-beta9`. 
If you are using an earlier version, this feature will not be available.
{{% /notice %}}

As we saw in the [Envoy API guide]({{% versioned_link_path fromRoot="/guides/security/rate_limiting/envoy/" %}}), 
Gloo Edge Enterprise exposes a fine-grained API that allows you to configure a vast number of rate limiting use cases
by defining [`actions`]({{% versioned_link_path fromRoot="/guides/security/rate_limiting/envoy//#actions" %}})
that specify an ordered tuple of descriptor keys to attach to a request and
[`descriptors`]({{% versioned_link_path fromRoot="/guides/security/rate_limiting/envoy//#descriptors" %}}) that match
an ordered tuple of descriptor keys and apply an associated rate limit.
  
Although powerful, this API has some drawbacks.
We only limit requests whose ordered descriptors match a rule exactly.
If, for example, we want to limit requests with an `x-type` header but limit requests differently
that have an `x-type` header as well as an `x-number` header equal to `5`, we need two sets of actions on each request-
one that gets only the value of `x-type` and another that gets the value of both `x-type` and `x-number`.
While this is certainly doable, it can quickly become verbose with enough descriptor keys.
We might need to enumerate all the combinations of descriptors when we want to rate limit based on several different subsets.

To address these shortcomings, we introduced a new API.
Starting with Gloo Edge Enterprise `v1.6.0-beta9` you can define rate limits using set-style descriptors.
These are treated as an unordered set such that a given rule will apply if all the specified descriptors match,
regardless of the presence and value of the other descriptors and regardless of descriptor order.
For example, a rule may match `type: a` and `number: one` but the `color` descriptor can have any value.
This can also be understood as `color: *` where * is a wildcard.

Set-style rate-limiting can be used alongside the prior implementation and is supported by both
global rate limiting, described in the [Envoy API guide]({{% versioned_link_path fromRoot="/guides/security/rate_limiting/envoy/" %}}),
and `RateLimitConfig` resources, described in the
[RateLimitConfigs guide]({{% versioned_link_path fromRoot="/guides/security/rate_limiting/crds/" %}}).

### SetActions
`setActions` have the same structure as the `actions` already used for rate limiting but must be listed under `setActions`
to indicate to the rate limit server that they should be treated as a set and not an ordered tuple.

### SetDescriptors
`setDescriptors` specify a rate limit along with any number of `simpleDescriptors` which, like `descriptors`, must include a key
and can optionally include a value.

### Simple Example
Let's run through a simple example that uses set-style rate limiting.

#### Initial setup
First, we need to install Gloo Edge Enterprise (minimum version `1.6.0-beta9`). Please refer to the corresponding
[installation guide]({{< versioned_link_path fromRoot="/installation/enterprise" >}}) for details.
 
Let's also deploy a simple application called petstore:

```bash
kubectl apply -f https://raw.githubusercontent.com/solo-io/gloo/v1.14.x/example/petstore/petstore.yaml
```

Now let's create a simple Virtual Service routing to this application. (It may take a few seconds to be Accepted.)

```bash
glooctl add route --name petstore --path-prefix / --dest-name default-petstore-8080
```

To verify that the Virtual Service works, let's send a request:

```bash
curl $(glooctl proxy url)/api/pets
```

It should return the expected response:
```
[{"id":1,"name":"Dog","status":"available"},{"id":2,"name":"Cat","status":"pending"}]
```

#### Add rate limit configuration
Now, let's edit the `Settings` resource to include a `setDescriptor` rate limiting rule:

```bash
kubectl edit settings -n gloo-system
```

and add a ratelimit section that limits requests with `type: a` and `number: one` to 1 per minute:

{{< highlight yaml "hl_lines=8-17" >}}
apiVersion: gloo.solo.io/v1
kind: Settings
metadata:
  name: default
  namespace: gloo-system
  # etc...
spec:
  ratelimit:
    setDescriptors:
    - simpleDescriptors:
      - key: type
        value: a
      - key: number
        value: one
      rateLimit:
        requestsPerUnit: 1
        unit: MINUTE
  # etc...
{{< /highlight >}}

Now edit the Virtual Service to include `setActions`:

```bash
kubectl edit vs petstore -n gloo-system
```

and add `setActions` capturing the `x-number` and `x-type` headers on the virtualHost:

{{< highlight yaml "hl_lines=9-18" >}}
apiVersion: gateway.solo.io/v1
kind: VirtualService
metadata:
  name: petstore
  namespace: gloo-system
  # etc...
spec:
  virtualHost:
    options:
      ratelimit:
        rateLimits:
        - setActions:
          - requestHeaders:
              descriptorKey: number
              headerName: x-number
          - requestHeaders:
              descriptorKey: type
              headerName: x-type
    domains:
    - '*'
    # etc...
{{< /highlight >}}

Note that the descriptor order doesn't match, but this is irrelevant for set-style rate limiting.

#### Test our configuration
Let's verify that our rate limit policy is correctly enforced.

Let's try sending some requests to the petstore Virtual Service. Submit the following command twice:

```bash
curl $(glooctl proxy url)/api/pets -v -H "x-type: a" -H "x-number: one"
```

On the second attempt you should receive the following response:

```shell script
< HTTP/1.1 429 Too Many Requests
< x-envoy-ratelimited: true
< date: Tue, 14 Jul 2020 23:13:18 GMT
< server: envoy
< content-length: 0
```

This demonstrates that the rate limit is enforced.

#### Understanding set-style rate limiting functionality

Now modify the Virtual Service `setActions` to add another descriptor. For example:
```yaml
          - requestHeaders:
              descriptorKey: color
              headerName: x-color
```

Send the following `curl` request a few times.

```bash
curl $(glooctl proxy url)/api/pets -v -H "x-type: a" -H "x-number: one"  -H "x-color: blue"
```
You should see that the request is still rate limited. Since the `setDescriptor` rule only looks for two descriptors,
it still matches whether more descriptors are present or not.

However, if you modify the Virtual Service `setActions` to remove the `type` or `number` descriptor, the request will no
longer be rate limited.

### Rule Priority

By default, `setDescriptor` rules are evaluated in the order they are listed. If a rule matches, later rules are ignored. 

For example, consider the following rules:

```yaml
spec:
  ratelimit:
    setDescriptors:
    - simpleDescriptors:
      - key: type
      - key: number
      rateLimit:
        requestsPerUnit: 10
        unit: MINUTE
    - simpleDescriptors:
      - key: type
      rateLimit:
        requestsPerUnit: 5
        unit: MINUTE
```

If the type and number are both present on a request, we want the limit to be 10 per minute.
However, if only the type is present on a request, we want the limit to be 5 per minute.

You can also specify the `alwaysApply` flag. This tells the server to consider a rule even if an earlier rule has already matched.

For example, if we have the same configuration as above but with the `alwaysApply` flag set to true,
a request with both type and number present will be limited after just 5 requests per minute, as both rules below are now considered.

```yaml
spec:
  ratelimit:
    setDescriptors:
    - simpleDescriptors:
      - key: type
      - key: number
      rateLimit:
        requestsPerUnit: 10
        unit: MINUTE
    - simpleDescriptors:
      - key: type
      rateLimit:
        requestsPerUnit: 5
        unit: MINUTE
      alwaysApply: true
```

### All-Encompassing Rules

We can also create rules that match all requests by omitting `simpleDescriptors` altogether.
Any `setDescriptor` rule should match requests whose descriptors contain the rule's `simpleDescriptors` as a subset.
If `simpleDescriptors` is omitted from the rule, requests whose descriptors contain the empty set as a subset should match,
i.e., all requests.

These rules should be listed after all other rules without `alwaysApply` set to true, or later rules will not be considered
due to rule priority, as explained above.

An all-encompassing rule without `simpleDescriptors` would look like this:

```yaml
spec:
  ratelimit:
    setDescriptors:
    - rateLimit:
        requestsPerUnit: 10
        unit: MINUTE
```

This rule will limit all requests to at most 10 per minute.