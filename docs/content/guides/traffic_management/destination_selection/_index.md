---
title: Destination Selection
weight: 30
description: Various ways to select routes based on matching predicates.
---

In the [Hello World]({{% versioned_link_path fromRoot="/guides/traffic_management/hello_world/" %}}) example, we saw how Gloo Edge uses a *Virtual Service* Custom Resource (CR) to allow users to specify routes to a particular destination, or *Upstream*. Each route on a *Virtual Service* includes a matcher, specifying the rules to determine if a request should be passed along the route. In the basic example, we used an exact match for a particular path. We'll now look at how to configure a route on a *Virtual Service* with different kinds of matching logic and matchers.

The following are the different aspects of the request that you can match against a route rule. Each aspect is combined with the others in a logical `AND`, i.e. all aspects must test `true` for the route to match and the specified route action to be taken.

{{% children description="true" depth="1" %}}

We recommend starting with [Path Matching]({{% versioned_link_path fromRoot="/guides/traffic_management/destination_selection/path_matching/" %}}) first and then reviewing the other matching types in turn.
