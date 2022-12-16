---
title: About caching responses
description: Learn about how caching works in Gloo Edge Enterprise with and without response validation. 
weight: 10
---

With response caching, you can significantly reduce the number of requests Gloo Edge makes to its upstream services.

{{% notice note %}}
This feature is available only for Gloo Edge Enterprise v1.12.x and later.
{{% /notice %}}

The Gloo Edge Enterprise caching filter is an extension that is built on top of the [Envoy cache filter](https://www.envoyproxy.io/docs/envoy/latest/start/sandboxes/cache), and includes all of the functionality that the Envoy cache filter exposes. In addition, Gloo Edge provides the ability to store the cached objects in a Redis instance, including Redis configuration options such as setting a password.

Review the information on this page to learn more about how caching works in Gloo Edge. 

## Caching without response validation {#caching-unvalidated}

The following diagram shows how response caching works without validation. 

![Caching without validation]({{% versioned_link_path fromRoot="/img/caching-unvalidated.svg" %}})

1. When the gateway receives an incoming request from a client, it checks with the caching server if a cached response for the referenced upstream is available. Because no cached response is initially available, the request is forwarded to the upstream service where the request is processed. When the upstream service sends a response to the client, the upstream can indicate that the response is cacheable by providing the `cache-control` response header. When Gloo Edge receives a response with a `cache-control` header, the response is cached by the caching server for the amount of time that is specified in the header. 
5. Subsequent requests from clients are not forwarded to the upstream. Instead, clients receive the cached response with an `age` response header from the caching server directly. The `age` response header shows the number of seconds that passed since the original response was sent. After the time that was specified in the `cache-control` header passes, the cached response becomes stale. Requests are then forwarded to the upstream service again and a new response is sent to the client.

## Caching with response validation {#caching-validated}

The following diagram shows how response caching works when the upstream service supports response validation. 

![Caching with validation]({{% versioned_link_path fromRoot="/img/caching-validated.svg" %}})

1. When the gateway receives an incoming request from a client, it checks with the caching server if a cached response for the referenced upstream is available. Because no cached response is initially available, the request is forwarded to the upstream service where the request is processed. When the upstream service sends a response to the client, the upstream can indicate that the response is cacheable by providing the `cache-control` response header. When Gloo Edge receives a response with a `cache-control` header, the response is cached by the caching server for the amount of time that is specified in the header. 
2. Subsequent requests from clients are not forwarded to the upstream. Instead, clients receive the cached response with an `age` response header from the caching server directly. The `age` response header shows the number of seconds that passed since the original response was sent.  
3. After the time that was specified in the `cache-control` header passes, the cached response becomes stale and the response validation period starts. In order for response validation to work, upstream services must be capable of processing `If-Modified-Since` request headers that are sent from the client. 
   1. If the upstream's response changed since the time that is specified in the `If-Modified-Since` request header, the new response is forwarded to the client and cached by the caching server (3a). Subsequent requests receive the cached response until the cache timeframe passes again (as shown in Step 2). 
   2. If the response did not change, the upstream service sends back a `304 Not Modified` HTTP response code. The gateway then gets the cached response from the caching server and returns it to the client (3b). Response validation continues for subsequent requests until a new response is received from the upstream service. 


