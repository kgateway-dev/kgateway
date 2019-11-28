---
title: Gloo Routing
weight: 30
---

### Motivation

Gloo has a powerful routing engine that can handle simple use cases like API-to-API routing as well as more complex ones like HTTP to gRPC with body and header transformations. Routing can also be done natively to cloud-function providers like AWS Lambda, Google Cloud Functions and Azure Functions.

Gloo can route requests directly to functions, which can be: a serverless function call (e.g. Lambda, Google Cloud Function, OpenFaaS function, etc.); an API call on a microservice or a legacy service (e.g. a REST API call, OpenAPI operation, XML/SOAP request etc.); or publishing to a message queue (e.g. NATS, AMQP, etc.). This unique ability is what makes Gloo the only API gateway that supports hybrid apps, as well as the only one that does not tie the user to a specific paradigm.

### Gloo Configuration

Let's see what underpins Gloo routing with a high-level look at the layout of the Gloo configuration. This can be seen as 3 layers: the Gateway listeners, Virtual Services, and Upstreams. Mostly you'll be interacting with [Virtual Services]({{% versioned_link_path fromRoot="/introduction/concepts#virtual-services" %}}), which allow you to configure the details of the API you wish to expose on the Gateway and how routing happens to backends. [Upstreams]({{% versioned_link_path fromRoot="/introduction/concepts#upstreams" %}}) represent those backends. [Gateway]({{% versioned_link_path fromRoot="/introduction/concepts#gateways" %}}) objects help you control the listeners for incoming traffic.

![Structure of gateway configurations with virtual service]({{% versioned_link_path fromRoot="/img/gloo-routing-concepts-overview.png" %}})

### Route Rules

Configuring the routing engine is done with defined predicates that match on incoming requests. These requests, such as headers, path, method, etc., are then routed to Upstream destinations such as like REST or gRPC services running in Kubernetes, EC2, etc. or Cloud Functions like Lambda. In the Virtual Services examples below we'll dig into this further.

![Structure of gateway configurations with virtual service]({{% versioned_link_path fromRoot="/img/gloo-routing-overview.png" %}})

### Implementations

Get started with Hello World and manipulating configurations. Once you're comfortable, move to more advanced use cases for understanding Gloo concepts like Virtual Services.

{{% children description="true" %}}
