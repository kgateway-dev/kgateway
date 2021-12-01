---
title: Traffic Management
weight: 10
description: Managing the traffic flowing through Gloo Edge to Upstream destinations
---

Gloo Edge has a powerful routing engine that can handle simple use cases like API-to-API routing as well as more complex ones like HTTP to gRPC with body and header transformations. Routing can also be done natively to cloud-function providers like AWS Lambda, Google Cloud Functions and Azure Functions.

Gloo Edge can route requests directly to functions, which can be: a serverless function call (e.g. AWS Lambda, Google Cloud Function, Microsoft Azure Function) or an API call on a microservice or a legacy service (e.g. a REST API call, OpenAPI operation, gRPC operation). This unique ability is what makes Gloo Edge the only API gateway that supports hybrid apps, as well as the only one that does not tie the user to a specific paradigm.

## Concepts

Review the following pages to understand the basic concepts for Gloo Edge traffic management.

* [Traffic management]({{% versioned_link_path fromRoot="/introduction/traffic_management/" %}}): Descriptions about the primary Gloo Edge traffic management components, including gateways, virtual services, routes, and upstreams.
* [Traffic processing]({{% versioned_link_path fromRoot="/introduction/traffic_filter/" %}}): Description of the types of transformations that Gloo Edge can apply to traffic, including an overview of the filter flow for policies, external authorization and authentication, rate limiting, and other transformations.

## Examples

Now that you have a basic framework for understanding what Gloo Edge routing does, let's get started with a [Hello World]({{% versioned_link_path fromRoot="/guides/traffic_management/hello_world/" %}}) example. After you're comfortable implementing a basic configuration, you can move to more advanced use cases and expand your understanding of core concepts in Gloo Edge like [Transformations]({{% versioned_link_path fromRoot="/guides/traffic_management/request_processing/transformations/" %}}) and [Network Security]({{% versioned_link_path fromRoot="/guides/security/tls/" %}}).

---

{{% children description="true" depth="2" %}}
