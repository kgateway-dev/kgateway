---
title: About caching responses
weight: 10
---

With response caching, you can significantly reduce the number of requests Gloo Edge must make to an Upstream service to return a response to a client.

The Gloo Edge Enterprise caching filter is an extension (implementing filter) of the [Envoy cache filter](https://www.envoyproxy.io/docs/envoy/latest/start/sandboxes/cache) and takes advantage of all the cache-ability checks that are applied. However, Gloo Edge also provides the ability to store the cached objects in a Redis instance, including Redis configuration options such as setting a password.

Use the following links to learn more about how caching works in Gloo Edge: 
- [Caching without response validation](#caching-unvalidated)
- [Caching with response validation](#caching-validated)

## Caching without response validation {#caching-unvalidated}

The following diagram shows how response caching works without validation. 

![Caching without validation]({{% versioned_link_path fromRoot="/img/caching-unvalidated.svg/" %}})

1. The gateway forwards incoming requests to the Upstream service where the request is processed. When the Upstream service sends back a response to the client, the response is cached by the caching server. 
2. Subsequent requests from clients are not forwarded to the Upstream. Instead, clients receive the cached response from the caching server directly. By default, responses are cached for 1 hour, unless the client specified a different time by sending the `cache-control` request header. After the time has passed, requests are forwarded to the Upstream service again and a new response is cached by the caching server. 


## Caching with response validation {#caching-validated}

The following diagram shows how response caching works when the Upstream service supports response validation. 

![Caching with validation]({{% versioned_link_path fromRoot="/img/caching-validated.svg/" %}})

1. The gateway forwards incoming requests to the Upstream service where the request is processed. When the Upstream service sends back a response to the client, the response is cached by the caching server. 
2. Subsequent requests from clients are not forwarded to the Upstream. Instead, clients receive the cached response from the caching server directly. By default, responses are cached for 1 hour, unless the client specified a different time by sending the `cache-control` request header. 
3. After the time has passed, the response validation period starts. In order for response validation to work, Upstream services must be capable of processing `if-modified-since` request headers that are sent from the client. If the Upstream's response changed since the time that is specified in the `if-modified-since` request header, the new response is forwarded to the client and cached by the caching server (3a). Subsequent requests receive the cached response until the cache timeframe has passed again (2). If the response has not changed, the Upstream service sends back a 304 Not Modified HTTP respones code. The gateway then gets the cached response from the caching server and returns it to the client (3b). Response validation continues for subsequent requests until a new response is received from the Upstream service. 


