---
title: "Gloo Edge vs others"
weight: 70
description: A comparison of similar products and how Gloo Edge is different.
---

Gloo Edge is an API Gateway and may come up in discussions about how it's different from "X". Although API Gateways are not new, [API Gateways have been going through an identity crisis recently](https://medium.com/solo-io/api-gateways-are-going-through-an-identity-crisis-d1d833a313d7) in terms of deployment footprint, use cases, and their renaissance in cloud-native architectures vis-a-vis service mesh. We go into a lot of detail in [this blog](https://medium.com/solo-io/api-gateways-are-going-through-an-identity-crisis-d1d833a313d7), but to put it concise, the categories of API Gateways fall loosely into the following:

* Traditional API Management
* Simple reverse proxies
* Integration products
* Software frameworks to build custom
* Other Envoy Proxy vendors

## vs API Management

Gloo Edge can be compared to traditional API Management vendors but is significantly simpler and more powerful. Gloo Edge doesn't require complicated databases, outdated proxies, and high-friction configuration in terms of reams of XML or bespoke, complex APIs etc. Gloo Edge can act as an edge/external API management platform for external APIs.

## Simple reverse proxies

There are excellent reverse proxies out there for moving bytes from one connection to another, but Gloo Edge -- as it's built on Envoy -- provides a lot more features like powerful traffic matching, routing, and load-balancing, circuit breaking, distributed tracing, deep telemetry collection and more. In today's cloud-native environment, these features are not nice to haves -- they're table stakes.

## Integration products

Legacy integration products have been cloud-washed to provide some of the functionality of an API Gateway, but they're typically full of baggage, don't scale, and are not easily configured for use in dynamic cloud environments. Gloo Edge is purpose built for these environments and for these cloud-native API gateway problems.

## Software frameworks 

Building an API Gateway for a simple use case is possible using generic software libraries and frameworks, but will it be battle tested? Will it be flexible enough? When you get past your simple use case, will you afford it the attention it needs? Building a capable gateway is easy for simple use cases, but once you need features that are standard with Gloo Edge, it will be a lot of undifferentiated heavy lifting to get your bespoke proxy up to par with industry standards.


## Other Envoy products

Some other vendors have also chosen Envoy as their data-plane proxy and there will be many more emerging in the near future. Choose Gloo Edge for these reasons:

* Dynamic and pluggable control plane to extend to your environment
* Route to cloud functions like Lambda
* Better runtime architecture: control plane decoupled from data plane
* Can run on Kubernetes or outside of Kubernetes with Consul and Vault (or flat files)
* Runtime catalog of services
* Can automatically discover Swagger / gRPC reflection / Lambdas
* Proven scalability, deployed across large number of community users and customers
