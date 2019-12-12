---
title: Rule Priority (Enterprise)
description: Rule priority in fine-grained rate limit API.
weight: 20
---

## Overview

In this guide we explore some more advanced configuration options added to Gloo's implementation of Envoy's rate limit
API. If you are not already familiar with Gloo's Envoy rate limit API support and examples, you should read our Envoy
rate limit API [intro]({{% versioned_link_path fromRoot="/security/rate_limiting/envoy" %}}) first.

## Motivation

Remember that rate limit configuration is defined in two places in the Envoy API:

* Envoy client config lives on `VirtualService`s and `Route`s
* The rate limit server config lives on the `Settings` resource

Sample Envoy client config:

{{< readfile file="security/rate_limiting/rulepriority/vsconfig.yaml" markdown="true">}}

Each `RateLimitAction` (the top-level list) generates a descriptor tuple to be sent to the rate limit server to be
checked for rate limiting. Each `Action` within a `RateLimitAction` (i.e., the inner list) gets appended in order to
generate the final rate-limiting descriptor.

For a single request with the `x-type` and `x-number` headers, the generated descriptor tuples for the above Envoy
client config would look like:

- `('type', '<x-type header value>')`
- `('type', '<x-type header value>'), ('number', '<x-number header value>')`

Descriptor tuples that cannot be built aren't sent to the rate limit server in their incomplete form, rather they are
ignored. For the above example, a request with only the `x-type` header generates only the first descriptor tuple.

Sample rate limit server config:

{{< readfile file="security/rate_limiting/rulepriority/serverconfig.yaml" markdown="true">}}

Each descriptor with a rate limit defines a rule to be considered during rate-limiting. When a request comes into Envoy,
rate limit actions are applied to the request to generate a list of descriptor tuples that are sent to the rate limit
server. The rate limit server evaluates each descriptor tuple to its full depth, ignoring any tuples that don't have
matching rules.

If any descriptor tuple matches a rule that requires rate limiting then the entire request returns HTTP 429 Too Many 
Requests. This is a strict requirement that rule priority lets us get around.

In Gloo Enterprise 1.x, Gloo added the `weight` (default 0) and `alwaysApply` (default `false`) configuration options
to the `Descriptor` definition:

- when evaluating matched rules, only rules with the highest weight (of the matched rules) are processed; if any of
these rules trigger rate limiting then the entire request will return a 429. Matched rules that are not considered for
rate-limiting are ignored in the rate limit server, and their request count is not incremented in the rate limit server
cache.
- `alwaysApply` is a boolean override for rule priority via weighted rules. Any rule matching a request with
`alwaysApply` set to `true` will always be considered for rate limiting, regardless of the rule's weight. The matching
rule with the highest weight will still be considered. (this can be a rule that also has `alwaysApply` set to `true`)

# Test the Example

Install the petclinic application and create a virtual service that routes to it:
```bash
kubectl apply \
  --filename https://raw.githubusercontent.com/solo-io/gloo/master/example/petclinic/petclinic.yaml

glooctl add route --name default --namespace gloo-system \
  --path-prefix / \
  --dest-name default-petclinic-8080 \
  --dest-namespace gloo-system
```

Open an editor to modify your rate limit client config:
```shell script
glooctl edit virtualservice --namespace gloo-system --name default ratelimit client-config
```
And paste the example config:

{{< readfile file="security/rate_limiting/rulepriority/vsconfig.yaml" markdown="true">}}

Now open an editor to modify your rate limit server config:
```shell script
glooctl edit settings --namespace gloo-system --name default ratelimit server-config
```

And paste the example config:

{{< readfile file="security/rate_limiting/rulepriority/serverconfig.yaml" markdown="true">}}

Run the following two times; you should get HTTP 429 Too Many Requests on the second request.
```shell script
curl -H "x-type: Messenger" -H "x-number: 311" --head $(glooctl proxy url)
```

Run the following two times; you should get HTTP 429 Too Many Requests on the second request.
```shell script
curl -H "x-type: Whatsapp" -H "x-number: 311" --head $(glooctl proxy url)
```

By changing the number we can match the more specific rule that has a higher priority.

Run the following a couple times:
```shell script
curl -H "x-type: Whatsapp" -H "x-number: 411" --head $(glooctl proxy url)
```

Requests to number `411` have a much higher rate limit, and will not return HTTP 429 unless more than 100 requests
are sent within a minute. Note that this wouldn't work without rule priority (play around with the server settings
to test this!) because our requests would match the inner Whatsapp rule (Rule 2) that limits all requests to 2/min,
regardless of the provided number.

### Cleanup

```bash
kubectl delete -f https://raw.githubusercontent.com/solo-io/gloo/master/example/petclinic/petclinic.yaml
kubectl delete vs default --namespace gloo-system
```
