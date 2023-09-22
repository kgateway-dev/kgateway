---
title: About local rate limiting
weight: 10
description: Learn about what local rate limiting is and how it works in Gloo Edge.
---

{{% notice note %}}
Local rate limiting requires Gloo Edge version 1.16 or later. 
{{% /notice %}}

Local rate limiting is a coarse-grained rate limiting capability that is primarily used as a first line of defense mechanism to limit the number of requests that are forwarded to your rate limit servers. Without local rate limiting, all requests are directly forwarded to the rate limit server where the request is either denied or allowed based on the [global rate limiting]({{% versioned_link_path fromRoot="/guides/security/rate_limiting/" %}}) settings that you configured. However during an attack, too many requests might be forwarded to your rate limit servers and can cause overload or even failure.

To protect your rate limit servers from being overloaded and to optimize their resource utilization, you can set up local rate limiting in conjunction with global rate limiting. Because local rate limiting is enforced in each Envoy instance that makes up your gateway, no rate limit server is required in this setup. For example, if you have 5 Envoy instances that together represent your gateway, each instance is configured with the limit that you set. In a global rate limiting setup, this limit is shared across all Envoy instances. 

Use the following table to learn more about the differences between local and global rate limiting. 

| Characteristics | Local rate limiting | Global rate limiting | 
| -- | -- | -- | 
| Purpose | First line of defense to limit the number of requests that reach the rate limit server.| Rate limit requests to destinations in the cluster based on components in the request, such as headers or query parameters.  | 
| Rate limits enforced by | Envoy process or Envoy pod | Rate limit server |
| Level of rate limit detail | Coarse-grained, only limits the number of requests per gateway, virtual service, virtual host, or route | Fine-grained, including rate limit actions and descriptors | 
| Rate limit server | No rate limit server is required | Rate limit server is required | 
| Limit | Rate limit applies to each Envoy instance | Rate limit is shared across all Envoy instances | 
| Extauth phase | Pre-auth only | Pre-auth and post-auth |

For more information about local rate limiting, see the [Envoy documentation](https://www.envoyproxy.io/docs/envoy/latest/configuration/http/http_filters/local_rate_limit_filter). 

## Architecture

The following image shows how local rate limiting works in Gloo Edge. As clients send requests to an upstream destination, they first reach the Envoy instance that represents your gateway. Local rate limiting settings are applied to an Envoy pod or process. Note that limits are applied to each pod or process. For example, if you have 5 Envoy instances that are configured with a local rate limit of 10 requests per second, the total number of allowed requests per second is 50 (5*10). In a global rate limiting setup, this limit is shared between all Envoy instances, so the total number of allowed requests per second is 10. 

Depending on your setup, each Envoy instance or pod is configured with a number of tokens in a token bucket. To allow a request, a token must be available in the bucket so that it can be assigned to a downstream connection. Token buckets are refilled occasionally as defined in the refill setting of the local rate limiting configuration. If no token is available, the connection is closed immediately, and a 429 HTTP response code is returned to the client. 

When a token is available in the token bucket it can be assigned to an incoming connection. The request is then forwarded to the rate limit server to enforce any global rate limiting settings. For example, the request might be further rate limited based on headers or query parameters. Only requests that are within the local and global rate limits are forwarded to the upstream destination in the cluster. 

<figure><img src="{{% versioned_link_path fromRoot="/img/local-rate-limiting.svg" %}}"/>
<figcaption style="text-align:center;font-style:italic">Local rate limiting in Gloo Edge</figcaption></figure>

## Order of rate limit priority

The local rate limiting Envoy filter allows you to configure the number of tokens in a token bucket for a gateway, virtual service, virtual host, or route. Depending on where you configure local rate limits, the limit might be enforced on that resource only or be applied to all underlying resources. For example, if you configure a limit on the gateway resource, the limit is automatically shared across all virtual services, hosts, and routes. However, if you also specify a local rate limit on an underlying resource, such as a particular route, the route does not share the limit of the gateway anymore, but instead enforces its own local rate limit as shown in the following diagram. 

<figure><img src="{{% versioned_link_path fromRoot="/img/local-rl-tokens.svg" %}}"/>
<figcaption style="text-align:center;font-style:italic">Token bucket inheritance</figcaption></figure>

The rate limit priority order is defined as follows. Local rate limiting settings on a gateway are always shared with all underlying resources. If a resource defines its own local rate limiting settings, these settings take precendence.

`Gateway > Virtual Service > Virtual Host > Route`

To try out an example, see the [HTTP gateway]({{% versioned_link_path fromRoot="/guides/security/local_rate_limiting/http" %}}) local rate limiting guide. 

## Types of local rate limiting on a gateway

Review your options for configuring local rate limiting on a gateway. 

### HTTP gateway

You can choose between the following options to configure local rate limiting on your HTTP gateway resource. 

- **HTTP local rate limiting**: You can apply local rate limting settings to all Layer 7 traffic by adding the `httpGateway.options.httpLocalRatelimit` setting on the gateway. Settings that are defined in this filter are enforced by the HTTP Envoy filter after the TLS handshake between the client and the gateway is completed successfully. To verify that your rate limiting settings are working as expected and to simplify troubleshooting later, you can optionally set the `enableXRatelimitHeaders: true` option. This option adds rate limiting headers to a response as defined in the [Envoy documentation](https://www.envoyproxy.io/docs/envoy/latest/api-v3/extensions/common/ratelimit/v3/ratelimit.proto#envoy-v3-api-enum-extensions-common-ratelimit-v3-xratelimitheadersrfcversion). 
- **Network local rate limiting**: If you want to configure the HTTP Envoy filter to apply local rate limiting settings to all Layer 4 traffic, you can add the `httpGateway.options.networkLocalRatelimit` option to your gateway resource. If set, local rate limiting is enforced before the TLS handshake between the client and the gateway is started. Because this option is enforced on Layer 4, no HTTP response code, headers, or messages are returned when rate limiting is enforced. 

{{% notice note %}}
If you add both an HTTP and a network local rate limiting filter to your HTTP gateway, the setting that is most restrictive takes precedence. 
{{% /notice %}}

To try out an example, see the [HTTP gateway]({{% versioned_link_path fromRoot="/guides/security/local_rate_limiting/http" %}}) local rate limiting guide. 

### TCP gateway

You can use the `tcpGateway.options.localRatelimit` setting on the TCP gateway to enforce local rate limits for all Layer 4 traffic. To try out an example, see the [TCP gateway]({{% versioned_link_path fromRoot="/guides/security/local_rate_limiting/http" %}}) local rate limiting guide. 


## Limitations

- The current implementation of the local rate limiting filter can be enforced pre-auth only.
- You cannot use the `spec.virtualhost.options.ratelimitRegular.localRatelimit` or `spec.virtualhost.routes.options.ratelimitRegular.localRatelimit` options in the virtual service as this enforces rate limiting settings post-auth. 
- You cannot set `spec.virtualhost.options.ratelimitEarly.localRatelimit` and `spec.virtualhost.options.ratelimit.localRatelimit`, or `spec.virtualhost.routes.options.ratelimitEarly.localRatelimit` and `spec.virtualhost.routes.options.ratelimit.localRatelimit` on the virtual service at the same time. 
