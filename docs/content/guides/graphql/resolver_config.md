---
title: Manual schema configuration
weight: 40
description: Manually configure resolvers and schema for your GraphQL API.
---

You can deploy your own GraphQL API, which might not leverage automatic service discovery and registration. To manually configure GraphQL resolvers, you create a Gloo Edge GraphQL API CRD. The following sections describe the configuration for REST <!--or gRPC -->resolvers, schema definitions for the types of data to return to graphQL queries, and an in-depth example.

## REST resolvers

```yaml
resolutions:
  # Resolver name
  Query|nameOfResolver:
    restResolver:
      # Configuration for generating outgoing requests to a REST API
      request:
        headers:
          # HTTP method (POST, PUT, GET, DELETE, etc.) 
          :method:
          # Path portion of upstream API URL. Can reference a parent attribute, such as /details/{$parent.id}
          :path:
          # User-defined headers (key/value)
          myHeader: 123
        # URL parameters (key/value)
        queryParams:
        # Request body content (primarily for PUT, POST, PATCH)
        body:
      # Configuration for modifying response from REST API before GraphQL server handles response
      response:
        # Select a child object or field in the API response  
        resultRoot:
        # Resolve naming mismatches between upstream field names and schema field names
        setters:
      upstreamRef:
        # Name of the upstream resource associated with the REST API  
        name:
        # The namespace the upstream resource
        namespace:
```

This example REST resolver, `Query|productsForHome`, specifies the path and the method that are needed to request the data.
```yaml
resolutions:
  Query|productsForHome:
    restResolver:
      request:
        headers:
          :method: GET
          :path: /api/v1/products
      upstreamRef:
        name: default-productpage-9080
        namespace: gloo-system
```
<!--
## gRPC resolvers

**QUESTION need the actual fields for a grpcResolver** Relevant section in the Proto: https://github.com/solo-io/gloo/blob/master/projects/gloo/api/v1/enterprise/options/graphql/v1beta1/graphql.proto#L171

```yaml
resolutions:
  # Resolver name
  Query|nameOfResolver:
    grpcResolver:
      requestTransform: <need fields>
      spanName: <need fields>
      upstreamRef:
        # Name of the upstream resource associated with the REST API  
        name:
        # The namespace the upstream resource
        namespace:
```

**QUESTION need example grpcResolver**
-->
## Schema definitions

A schema definition determines what kind of data can be returned to a client that makes a GraphQL query to your endpoint. The schema specifies the data that a particular `type`, or service, returns in response to a GraphQL query.

In this example, fields are defined for the three Bookinfo services, Product, Review, and Rating. Additionally, the schema definition indicates which services reference the resolvers. In this example, the Product service references the `Query|productForHome` REST resolver.

```yaml
schema_definition: |
  type Query {
    productsForHome: [Product] @resolve(name: "Query|productsForHome")
  }

  type Product {
    id: String
    title: String
    descriptionHtml: String
    author: String @resolve(name: "author")
    pages: Int @resolve(name: "pages")
    year: Int @resolve(name: "year")
    reviews : [Review] @resolve(name: "reviews")
    ratings : [Rating] @resolve(name: "ratings")
  }

  type Review {
    reviewer: String
    text: String
  }

  type Rating {
    reviewer : String
    numStars : Int
  }
```

## Sample GraphQL API

To get started with your own GraphQL API, check out the in-depth example in the [`graphql-bookinfo` repository](https://github.com/solo-io/graphql-bookinfo). You can model your own use case based on the contents of this example:
* The `kubernetes` directory contains the Bookinfo sample app deployment, the example GraphQL API, and the virtual service to route requests to the `/graphql` endpoint.
* The `openapi` directory contains the OpenAPI specifications for the individual BookInfo microservices, along with the original consolidated BookInfo REST API.

## Routing to the GraphQL server

After you automatically or manually create your GraphQL resolver and schema, create a virtual service that defines a `Route` with a `graphqlApiRef` as the destination. This route ensures that all GraphQL queries to a specific path are now handled by the GraphQL server in the Envoy proxy.

In this example, all traffic to `/graphql` is handled by the GraphQL server, which uses the `default-petstore-8080` GraphQL API.
{{< highlight yaml "hl_lines=12-16" >}}
cat << EOF | kubectl apply -f -
apiVersion: gateway.solo.io/v1
kind: VirtualService
metadata:
  name: 'default'
  namespace: 'gloo-system'
spec:
  virtualHost:
    domains:
    - '*'
    routes:
    - graphqlApiRef:
        name: default-petstore-8080
        namespace: gloo-system
      matchers:
      - prefix: /graphql
EOF
{{< /highlight >}}

## Reference

For more information, see the [Gloo Edge API reference for GraphQL]({{% versioned_link_path fromRoot="/reference/api/github.com/solo-io/gloo/projects/gloo/api/v1/enterprise/options/graphql/v1beta1/graphql.proto.sk/" %}}).