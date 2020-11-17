---
title: Destination Types
weight: 40
description: Once a route is matched, where does the request go?
---

As we saw in the [Destination Selection]({{< versioned_link_path fromRoot="/guides/traffic_management/destination_selection/" >}}) section, *Routes* in Gloo Edge contain a *Matcher* to determine if a request should be passed along the route. If a route is matched, then the route also specifies an action to take: routing to one or more destinations, redirecting, or returning a direct response.

In this section, we'll take a deeper look at different ways to specify route destinations. Most commonly, a route destination is a single Gloo Edge *Upstream*. It's also possible to route to multiple Upstreams, by either specifying a *multi* destination, or by configuring an *Upstream Group*. Finally, it's possible to route directly to Kubernetes or Consul services, without needing to use Gloo Edge Upstreams or discovery. 

When routing to an Upstream, you can take advantage of Gloo Edge's endpoint discovery system, and configure routes to specific functions, such as a [REST endpoint]({{< versioned_link_path fromRoot="/guides/traffic_management/destination_types/rest_endpoint/" >}}), a [gRPC service]({{< versioned_link_path fromRoot="/guides/traffic_management/destination_types/aws_lambda/" >}}), or a cloud function like [AWS Lambda]({{< versioned_link_path fromRoot="/guides/traffic_management/destination_types/aws_lambda/" >}}). 

The full list of destination guides is listed below. We recommend starting with [Static Upstreams]({{< versioned_link_path fromRoot="/guides/traffic_management/destination_types/static_upstream/" >}}), and then evaluating more complex route destination types based on your needs.

{{% children description="true" depth="1" %}}