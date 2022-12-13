---
title: About caching responses
weight: 10
---

With response caching, you can significantly reduce the number of requests Gloo Edge must make to an upstream service to return a response to a client.  

The Gloo Edge Enterprise caching filter is an extension (implementing filter) of the [Envoy cache filter]() and takes advantage of all the cache-ability checks that are applied. However, Gloo Edge also provides the ability to store the cached objects in a Redis instance, including Redis configuration options such as setting a password.

## Caching without validation

The following diagram shows how response caching works without validation. 

![Caching without validation]()

The gateway forwards incoming requests to the Upstream service where the request is processed. When the Upstream service sends back a response to the client, the response is cached by the gateway. Subsequent requests from clients are not forwarded to the Upstream. Instead, clients receive the cached response from the gateway directly. 

If caching is enabled, the response from the Upstream is stored 


## Caching with validation

The following diagram shows how response caching works when the Upstream service supports response validation. 

![Caching with validation]()
