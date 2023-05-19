---
title: Gloo Edge API (Enterprise)
weight: 30
description: Simplified rate-limit API that covers most use cases.
---

## Overview

Gloo Edge includes a simplified rate limiting model that allows you to specify the number of requests per configurable unit of time that can be made against all routes defined within a virtual host or individual routes. You can set different limits for both authorized and anonymous users. An authorized user is one that the Gloo Edge external authentication server has validated and their user token is included with the request. Authorized users are rate limited on a per user basis. Anonymous users are rate limited on a calling IP basis, i.e., all requests from that incoming IP count towards the requests per time limits.

For a more fine grained approach, take a look at using Gloo Edge with [Envoy's native rate limiting model]({{% versioned_link_path fromRoot="/guides/security/rate_limiting/envoy/" %}})

## Rate Limit

Rate limits are defined on the virtual service or route specification as `spec.virtualHost.options.ratelimitBasic` with the following {{% protobuf name="ratelimit.options.gloo.solo.io.IngressRateLimit" display="format"%}}. There is a full example later in this document that shows the rate limit configuration in context.

```yaml
ratelimitBasic:
  anonymousLimits:
    requestsPerUnit: 1000
    unit: HOUR
  authorizedLimits:
    requestsPerUnit: 200
    unit: MINUTE
```

- Rate limits can be set for anonymous requests, authorized requests, both, or neither.
- `authorizedLimits` represent the rate limits imposed on requests that are associated with a known user id. Note that this user id is included in the [external auth service's]({{% versioned_link_path fromRoot="/guides/security/auth/extauth" %}}) [AuthorizationResponse]({{% versioned_link_path fromRoot="/guides/dev/writing_auth_plugins/#header-propagation" %}}) in the `UserInfo.UserID` field.
- `anonymousLimits` represent the rate limits imposed on requests that are not associated with a known user id. In this case, the limit is applied to the request's remote address.
- `requestsPerUnit` takes an integer value
- `unit` must be one of these strings: `SECOND`, `MINUTE`, `HOUR`, `DAY`

### An example virtual service with rate limits enabled

The minimum required configuration to create a new virtual service for the example petclinic application with service-level anonymous and authorized rate limits enabled is shown below.

First, install the petclinic application.

```shell
kubectl apply \
  --filename https://raw.githubusercontent.com/solo-io/gloo/v1.2.9/example/petclinic/petclinic.yaml
```

Refer to the [Gloo Edge external authentication]({{% versioned_link_path fromRoot="/guides/security/auth" %}}) documentation on how to configure Gloo Edge to authenticate users.

In this example, we restrict authorized users to 200 requests per minute and anonymous users to 1000 requests per hour.

{{< highlight yaml "hl_lines=20-26" >}}
apiVersion: gateway.solo.io/v1
kind: VirtualService
metadata:
  name: default
  namespace: gloo-system
spec:
  displayName: default
  virtualHost:
    domains:
    - '*'
    routes:
    - matchers:
      - prefix: /
      routeAction:
        single:
          upstream:
            name: default-petclinic-8080
            namespace: gloo-system
    options:
      ratelimitBasic:
        anonymousLimits:
          requestsPerUnit: 1000
          unit: HOUR
        authorizedLimits:
          requestsPerUnit: 200
          unit: MINUTE
    # extauth:
    #   oauth:
    #     # your OAuth settings here to authorize users
{{< /highlight >}}

You can also just set rate limits for just anonymous users (rate limit by remote address) or just authorized users (rate limit by user id). For example, to rate limit for anonymous users, you would configure the `anonymousLimits` section like as follows.

{{< highlight yaml "hl_lines=20-23" >}}
apiVersion: gateway.solo.io/v1
kind: VirtualService
metadata:
  name: default
  namespace: gloo-system
spec:
  displayName: default
  virtualHost:
    domains:
    - '*'
    routes:
    - matchers:
      - prefix: /
      routeAction:
        single:
          upstream:
            name: default-petclinic-8080
            namespace: gloo-system
    options:
      ratelimitBasic:
        anonymousLimits:
          requestsPerUnit: 1000
          unit: HOUR
{{< /highlight >}}

### Route-level rate limits

Rate limits can be specified on individual routes within a virtual host in addition to or instead of the virtual host itself. The API for specifying the rate limits is identical to the virtual host version, with one caveat. Any routes with a specified `ratelimitBasic` must also specify a `name` at the top level of the route. These names:
* Must be non-blank.
* Must not match other route names or the name of the virtual host, for all such resources with a `ratelimitBasic` specified.
For example:
{{< highlight yaml "hl_lines=14-19" >}}
apiVersion: gateway.solo.io/v1
kind: VirtualService
metadata:
  name: default
  namespace: gloo-system
spec:
  displayName: default
  virtualHost:
    domains:
    - '*'
    routes:
    - matchers:
      - prefix: /
      name: example-route
      options:
        ratelimitBasic:
          anonymousLimits:
            requestsPerUnit: 1000
            unit: HOUR
      routeAction:
        single:
          upstream:
            name: default-petclinic-8080
            namespace: gloo-system
{{< /highlight >}}
