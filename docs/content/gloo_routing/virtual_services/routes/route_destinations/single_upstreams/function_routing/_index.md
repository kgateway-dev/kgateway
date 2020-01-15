---
title: Function Routing
weight: 4
description: Gloo can dynamically discover functions available on an upstream to simplify routing to a specific endpoint. 
---

Gloo builds on top of [Envoy proxy](https://www.envoyproxy.io) by giving it the ability to understand functions belonging to upstream clusters. Envoy (and most other gateways) are great at routing to backend clusters/services, but they don't know what functions (REST, gRPC, SOAP, etc) are exposed at each of those clusters/services. Gloo can dynamically discover and understand the details of a [Swagger](https://github.com/OAI/OpenAPI-Specification) or [gRPC reflection](https://github.com/grpc/grpc-go/blob/master/Documentation/server-reflection-tutorial.md), which can help make routing easier. In this tutorial, we'll take a look at Gloo's function routing and transformation capabilities.

{{% children description="true" depth="1" %}}