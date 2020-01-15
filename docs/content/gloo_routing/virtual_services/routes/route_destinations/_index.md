---
title: Route Destinations
weight: 20
description: Once a route is matched, where does the request go?
---

As we saw in the [Matching Rules]({{< versioned_link_path fromRoot="/gloo_routing/virtual_services/routes/matching_rules" >}}) section, *Routes* in Gloo contain a *Matcher* to determine if a request should be passed along the route. If a route is matched, then the route also specifies an action to take: routing to one or more destinations, redirecting, or returning a direct response.

In this section, we'll take a deeper look at different ways to specify route destinations. Most commonly, a route destination is a single Gloo *Upstream*. It's also possible to route to multiple Upstreams, by either specifying a *multi* destination, or by configuring an *Upstream Group*. Finally, it's possible to route directly to Kubernetes or Consul services, without needing to use Gloo Upstreams or discovery. 

When routing to an Upstream, you can take advantage of Gloo's endpoint discovery system, and configure routes to specific functions, either on a REST or gRPC service, or on a cloud function. This is covered more in [Function Routing]({{< versioned_link_path fromRoot="/gloo_routing/virtual_services/routes/route_destinations/single_upstreams/function_routing/" >}}). 

The section listing is shown below. We recommend starting with [Single Upstreams]({{< versioned_link_path fromRoot="/gloo_routing/virtual_services/routes/route_destinations/single_upstreams/" >}}) and more specifically with [Static Upstreams]({{< versioned_link_path fromRoot="/gloo_routing/virtual_services/routes/route_destinations/single_upstreams/static_upstream/" >}}), and then evaluating more complex route destination types.

{{% children description="true" depth="1" %}}