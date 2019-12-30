---
title: Canary Release
weight: 30
description: Reducing the risk of making changes with canary release
---

One of the most important capabilities enabled by Gloo is reducing the risk of making changes to your services by controlling what traffic goes to which backend services. The term ["canary release"](https://blog.christianposta.com/deploy/blue-green-deployments-a-b-testing-and-canary-releases/) in this case refers to slowly and iteratively introducing a new deployment into your production environment by putting a "canary" into the environment. The idea with the canary is to test whether the intended changes behave well and without issues when taking a small fraction (think, 1% ) of the traffic. If this canary deployment starts to misbehave (as judged by external metric collection like request throughput or latency) then we immediately roll it back and direct traffic to the last known-working deployment.

![Canary traffic]({{% versioned_link_path fromRoot="/img/solo-canary.png" %}})

This technique can be used to progressively deliver new changes into production. If the canary performs well and shows no signs of misbehavior, we can allow a larger chunk of traffic to the new deployment. At each point we increase the traffic load to the new service, we evaluate the service's behavior against the metrics in which we are interested. At any point we may end up rolling to release back. 

This is significantly different than a big bang release where 100% of traffic gets affected. Even the so called "blue/green" deployment with a single cutover suffers from this all or nothing problem. Using fine-grained traffic control with Gloo, we can significantly reduce the risk and exposure of making new changes in our production environment. 

## Canary with Kubernetes
Kubernetes has an incredibly powerful workload selection mechanism which forms the basis of Kubernetes Services. With Kubernetes Services, we can select which pods belong to a routing group. Using this mechanism, we can include pods from more than one Kubernetes Deployment. We can use this approach to slowly introduce a canary deployment. The drawback for this approach is twofold: 

1. How fine-grained you can route traffic depends on how many pods you run from each deployment
2. Out of the box, Kubernetes Services only load-balance L4 traffic (new connections)

With these drawbacks, to achieve 1% traffic routing, you will need 100 pods: 1 with the new (canary) deployment and the other 99 with the existing service. Using an approach with Gloo, you have control down to the request and load-balancing level (not just connection level as in **2.**) coming into the cluster. You can have 1 of the new service and 1 of the old service and still achieve 1% request canarying. 

## Traffic shadowing for Canary

A step you could perform _before_ a percentage based canary of traffic is traffic shadowing. In this model, we don't send a percentage of live traffic to the new service, but we send a percentage of _shadowed_ traffic. With shadowed traffic, we make a copy of a request and send it to the new service, not the actual request. Responses get ignored. [Gloo supports traffic shadowing to add in Canary releases]({{< versioned_link_path fromRoot="/gloo_routing/virtual_services/routes/routing_features/shadowing" >}}).

![Canary traffic]({{% versioned_link_path fromRoot="/img/traffic-shadowing.png" %}})

## Canary with Gloo

Gloo supports a very powerful Canary mechanism through [UpstreamGroups]({{< versioned_link_path fromRoot="/gloo_routing/virtual_services/routes/route_destinations/multiple_upstreams/upstream_groups" >}}). Using `UpstreamGroups` we can specify very fine-grained traffic weight to control request by % of requests. 

## Canary with Gloo+Flagger

[Flagger is an option for automated canary release](https://docs.flagger.app/usage/gloo-progressive-delivery). Flagger supports using Gloo (using `UpstreamGroups` under the covers) to automatically roll out a Canary at increasing traffic weights for the new service at a controlled interval. Flagger also observes traffic metrics and will cancel a canary if it goes outside a specified threshold. 
