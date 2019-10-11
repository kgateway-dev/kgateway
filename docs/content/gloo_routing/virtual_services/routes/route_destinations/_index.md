---
title: Route Destinations
weight: 20
description: Once a route is matched, where does the request go?
---

As we saw in the previous section, **Routes** in Gloo contain a **Matcher** to determine if a request should be passed
along the route. If a route is matched, then the route also specifies an action to take: routing to one or more destinations, 
redirecting, or returning a direct response. 

In this section, we'll take a deeper look at different ways to specify route destinations. Most commonly, a route 
destination is a single Gloo **Upstream**. It's also possible to route to multiple upstreams, by either specifying a 
**multi** destination, or by configuring an **Upstream Group**. Finally, it's possible to route directly to Kubernetes
or Consul services, without needing to use Gloo **Upstreams** or discovery. 

When routing to an upstream, you can take advantage of Gloo's endpoint discovery system, and configure routes to 
specific functions, either on a rest or GRPC service, or on a cloud function. This is covered more in 
[Function Routing](single_upstreams/function_routing). 

{{% children description="true" depth="1" %}}