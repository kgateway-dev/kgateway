---
title: GraphQL (Enterprise)
weight: 70
description: Enable GraphQL resolution.
---

Set up API gateway and GraphQL server functionality for your apps in the same process by using Gloo Edge.

{{% notice note %}}
This feature is available only in Gloo Edge Enterprise.
{{% /notice %}}

## Why GraphQL?

GraphQL is a server-side query language and runtime you can use to expose your APIs as an alternative to REST APIs. GraphQL allows you to request only the data you want and handle any subsequent requests on the server side, saving numerous expensive origin-to-client requests by instead handling requests in your internal network.

## Using GraphQL in an API gateway

API gateways expose microservices with different implementations from a single location and schema. The API gateway acts like a single owner for all requests and responses. As such, it can shape traffic according to consistent policies that you set. When you integrate with GraphQL, you get the benefits of an API gateway and more. GraphQL exposes your API without versioning and lets clients interact with the API on their own terms. Additionally, you can mix and match your GraphQL graph with your existing REST routes. This setup lets you test and migrate to GraphQL at a pace that makes sense for your organization.

Gloo Edge extends API gateway and GraphQL capabilities with route-level control. Usually, API gateways apply edge networking logic at the route level. For example, the gateway might rate limit, authorize, and authenticate requests. Most GraphQL servers are a separate endpoint behind the API gateway. Therefore, you cannot add route-level customizations. In contrast, Gloo Edge embeds route-level customization logic into the API gateway.

For more information, check out the [GraphQL blog post](https://www.solo.io/blog/announcing-gloo-graphql/).

## Get started

Check out the following pages to set up GraphQL in your Gloo Edge environment.

{{% children description="true" %}}