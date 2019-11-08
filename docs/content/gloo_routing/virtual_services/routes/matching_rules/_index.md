---
title: Matching Rules
weight: 10
description: Various ways to enable routes based on  matching predicates
---

In the basic example, we saw how Gloo uses a **VirtualService** CRD to allow users to specify routes to a particular destination, or **Upstream**.
Each route on a **VirtualService** includes a matcher, specifying the rules to determine if a request should be passed along the route. 
In the basic example, we used an exact match for a particular path. 
We'll now look at how to configure a route on a **VirtualService** with different kinds of matching logic using the {{< protobuf name="gloo.solo.io.Matcher">}}.

The following are the different aspects of the request that you can match against a route rule. Each aspect is `AND`
with others, i.e., all aspects must test `true` for the route to match and the specified route action to be taken.

{{% children description="true" depth="1" %}}
