---
title: Gloo Routing
weight: 30
---

## Motivation

Gloo has a powerful routing engine that can handle simple use cases like API-to-API routing as well as more complex ones like HTTP to gRPC with body and header transformations. Routing can also be done natively to cloud-function providers like AWS Lambda, Google Cloud Functions and Azure Functions.

### Gloo Configuration

Let's see what underpins Gloo routing with a high-level look at the layout of the Gloo configuration. This can be seen as 3 layers: the Gateway listeners, Virtual Services, and Upstreams. Mostly you'll be interacting with [Virtual Services](../introduction/concepts#virtual-services), which allow you to configure the details of the API you wish to expose on the Gateway and how routing happens to backends. [Upstreams](../introduction/concepts#upstreams) represent those backends. [Gateway](../introduction/concepts#gateways) objects help you control the listeners for incoming traffic.

![Structure of gateway configurations with virtual service]({{% versioned_link_path fromRoot="/img/gloo-concept-overview.png" %}})

### Route Rules

To configure the details of the routing engine, we define predicates that match on incoming requests (things like headers, path, method, etc) and then route them to Upstream destinations (like REST or gRPC services running in Kubernetes, EC2, Consul, etc or Cloud Functions like Lambda).

![Structure of gateway configurations with virtual service]({{% versioned_link_path fromRoot="/img/gloo-routing-overview.png" %}})

Take a look at getting started with the [hello world](./hello_world) guide and move to more advanced use cases by understanding the [Virtual Service](../introduction/concepts#virtual-services) concept.

{{% children description="true" %}}
