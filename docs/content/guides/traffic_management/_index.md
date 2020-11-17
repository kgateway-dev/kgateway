---
title: Traffic Management
weight: 10
description: Managing the traffic flowing through Gloo Edge to Upstream destinations
---

Gloo Edge has a powerful routing engine that can handle simple use cases like API-to-API routing as well as more complex ones like HTTP to gRPC with body and header transformations. Routing can also be done natively to cloud-function providers like AWS Lambda, Google Cloud Functions and Azure Functions.

Gloo Edge can route requests directly to functions, which can be: a serverless function call (e.g. Lambda, Google Cloud Function, OpenFaaS function, etc.); an API call on a microservice or a legacy service (e.g. a REST API call, OpenAPI operation, XML/SOAP request etc.); or publishing to a message queue (e.g. NATS, AMQP, etc.). This unique ability is what makes Gloo Edge the only API gateway that supports hybrid apps, as well as the only one that does not tie the user to a specific paradigm.

Routes are the primary building block of the *Virtual Service*. A route contains matchers and an upstream which could be a single destination, a list of weighted destinations, or an upstream group. 

There are many types of **matchers**, including **Path Matching**, **Header Matching**, **Query Parameter Matching**, and **HTTP Method Matching**. Matchers can be combined in a single rule to further refine which requests will be matched against that rule.

Gloo Edge is capable of sending matching requests to many different types of *Upstreams*, including **Single Upstream**, **Multiple Upstream**, **Upstream Groups**, Kubernetes services, and Consul services. The ability to route a request to multiple *Upstreams* or *Upstream Groups* allows Gloo Edge to load balance requests and perform Canary Releases.

Gloo Edge can also alter requests before sending them to a destination, including **Transformation**, **Fault Injection**, response header editing, and **Prefix Rewrite**. The ability to edit requests on the fly gives Gloo Edge the power specify the proper parameters for a function or transform and error check incoming requests before passing them along.

---

## Gloo Edge Configuration

Let's see what underpins Gloo Edge routing with a high-level look at the layout of the Gloo Edge configuration. This can be seen as 3 layers: the *Gateway listeners*, *Virtual Services*, and *Upstreams*. Mostly, you'll be interacting with [Virtual Services]({{% versioned_link_path fromRoot="/introduction/architecture/concepts#virtual-services" %}}), which allow you to configure the details of the API you wish to expose on the Gateway and how routing happens to the backends. [Upstreams]({{% versioned_link_path fromRoot="/introduction/architecture/concepts#upstreams" %}}) represent those backends. [Gateway]({{% versioned_link_path fromRoot="/introduction/architecture/concepts#gateways" %}}) objects help you control the listeners for incoming traffic.

![Structure of gateway configurations with virtual service]({{% versioned_link_path fromRoot="/img/gloo-routing-concepts-overview.png" %}})

## Route Rules

Configuring the routing engine is done with defined predicates that match on incoming requests. The contents of a request, such as headers, path, method, etc., are examined to see if they match the predicates of a route rule. If they do, the request is processed based on enabled routing features and routed to an Upstream destinations such as REST or gRPC services running in Kubernetes, EC2, etc. or Cloud Functions like Lambda. In the [Traffic Management section]({{% versioned_link_path fromRoot="/guides/traffic_management/" %}}) we'll dig into this process further.

![Structure of gateway configurations with virtual service]({{% versioned_link_path fromRoot="/img/gloo-routing-overview.png" %}})

## Examples and Concepts

Now that you have a basic framework for understanding what Gloo Edge routing does, let's get started with a [Hello World]({{% versioned_link_path fromRoot="/guides/traffic_management/hello_world/" %}}) example. Once you're comfortable implementing a basic configuration, you can move to more advanced use cases and expand your understanding of core concepts in Gloo Edge like [Traffic Management]({{% versioned_link_path fromRoot="/guides/traffic_management/" %}}) and [Network Security]({{% versioned_link_path fromRoot="/guides/security/tls/" %}}).

---

{{% children description="true" depth="2" %}}