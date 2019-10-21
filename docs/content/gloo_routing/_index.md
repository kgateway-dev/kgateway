---
title: Gloo Routing
weight: 30
---

Gloo has a powerful routing engine that can handle simple use cases like API-to-API routing as well as more complex ones like HTTP to gRPC with body and header transformations. Gloo can also route natively to cloud-function providers like AWS Lambda, Google Cloud Functions and Azure Functions. 

To understand Gloo routing, we should understand the high-level layout of the Gloo configuration. In general, you will be interacting with [Virtual Services](../introduction/concepts#virtual-services). These objects allow you to configure the details of the API you wish to expose on the Gateway as well as how the routing happens to any backends ([Upstreams](../introduction/concepts#upstreams). To get traffic into the Gloo gateway, you need to control the listeners through the [Gateway](../introduction/concepts#gateways) objects.

![Structure of gateway configurations with virtual service]({{% versioned_link_path fromRoot="/img/gloo-concept-overview.png" %}})

To configure the details of the routing engine, we define predicates that match on incoming requests (things like headers, path, method, etc) and then route them to Upstream destinations (like REST or gRPC services running in Kubernetes, EC2, Consul, etc or Cloud Functions like Lambda).

![Structure of gateway configurations with virtual service]({{% versioned_link_path fromRoot="/img/gloo-routing-overview.png" %}})

Take a look at getting started with the [hello world](./hello_world) guide and move to more advanced use cases by understanding the [Virtual Service](../introduction/concepts#virtual-services) concept. 


{{% children description="true" %}}
