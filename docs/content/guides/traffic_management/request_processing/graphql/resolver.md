---
title: TODO
weight: 20
description: TODO
---

TODO shortdesc

## Automatic schema generation with GraphQL service discovery

TODO do we need to add the behind-the-scenes info for this feature? eg Sai has a great overview here: https://docs.google.com/presentation/d/1ArxEdVVAOowz4wYcXIYlg4Wd-psadUTrOYH9DPPWwpk/edit#slide=id.g10a8760f3dd_24_181

Gloo Edge can automatically discover API specifications and create GraphQL schemas. Gloo Edge supports two modes of discovery: allowlist and blocklist. For more information about these discovery modes, see the [Function Discovery Service (FDS) guide]({{% versioned_link_path fromRoot="/installation/advanced_configuration/fds_mode/#function-discovery-service-fds" %}}).

### Allowlist mode

In allowlist mode, discovery is enabled manually for only specific services by labeling those services with the `function_discovery=enabled` label. This mode gives you full manual control over which services you want to expose as GraphQL services.

First, label services for discovery.
```sh
kubectl label service <service_name> discovery.solo.io/function_discovery=enabled
```

Then, allow automatic generation of GraphQL schemas by enabling FDS discovery in allowlist mode.
```sh
kubectl patch settings -n gloo-system default --type=merge --patch '{"spec":{"discovery":{"fdsMode":"WHITELIST"}}}'
```

### Blocklist mode

In blocklist mode, discovery is enabled for all supported services, unless you explicitly disable discovery for a service by using the `function_discovery=disbled` label.

First, label services that you do not want to be discovered.
```sh
kubectl label service <service_name> discovery.solo.io/function_discovery=disabled
```

Then, allow automatic generation of GraphQL schemas by enabling FDS discovery in blocklist mode.
```sh
kubectl patch settings -n gloo-system default --type=merge --patch '{"spec":{"discovery":{"fdsMode":"BLACKLIST"}}}'
```

### Verifying automatic resolver generation

You can verify that OpenAPI specification discovery is enabled by viewing the GraphQL custom resource that was automatically generated for your service.
```sh
kubectl get graphqlschemas -n gloo-system
```
```sh
kubectl get graphqlschemas <schema_name> -o yaml -n gloo-system
```

## Manual schema configuration

You can deploy your own GraphQL API, which might not leverage automatic service discovery and registration. To manually configure resolvers, you create a Gloo Edge GraphQL schema CRD. The following sections describe the configuration for REST or gRPC resolvers, schema definitions for the types of data to return to graphQL queries, and an in-depth example.

### REST resolvers

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

### gRPC resolvers

TODO need the actual fields for a grpcResolver. Relevant section in the Proto: https://github.com/solo-io/gloo/blob/master/projects/gloo/api/v1/enterprise/options/graphql/v1alpha1/graphql.proto#L171

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

TODO need example grpcResolver

### Schema definitions

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

TODO is this where the information about stitching should go?? Or, should we have the stitching explanation elsewhere (either leave it on this page at the bottom or move it somewhere else) and just link to it from here?

### Sample GraphQL API

To get started with your own GraphQL API, check out the in-depth example in the [`graphql-bookinfo` repository](https://github.com/solo-io/graphql-bookinfo). You can model your own use case based on the contents of this example:
* The `kubernetes` directory contains the Bookinfo sample app deployment, the example GraphQL schema, and the virtual service to route requests to the `/graphql` endpoint.
* The `openapi` directory contains the OpenAPI specifications for the individual BookInfo microservices, along with the original consolidated BookInfo REST API.

### Reference

For more information, see the [Gloo Edge API reference for GraphQL]({{% versioned_link_path fromRoot="/reference/api/github.com/solo-io/gloo/projects/gloo/api/v1/enterprise/options/graphql/v1alpha1/graphql.proto.sk/" %}})

## Routing to the GraphQL server

After you automatically or manually create your GraphQL resolver and schema, create a virtual service that defines a `Route` with a `graphqlSchemaRef` as the destination. This route ensures that all GraphQL queries to a specific path are now handled by the GraphQL server in the Envoy proxy.

In this example, all traffic to `/graphql` is handled by the GraphQL server, which uses the `default-petstore-8080` GraphQL schema.
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
    - graphqlSchemaRef:
        name: default-petstore-8080
        namespace: gloo-system
      matchers:
      - prefix: /graphql
EOF
{{< /highlight >}}

## Caching

There are three parts to this:
- How we automatically cache upstream responses in the resolver layer. No user configuration required - we’re just describing the behavior and why it’s good.
- Automatic persisted queries: https://github.com/solo-io/envoy-gloo-ee/issues/272
- Response Caching: This is basically piggybacking on the response caching support we’re doing for Edge https://github.com/solo-io/envoy-gloo-ee/issues/260


## Stitching

TODO not sure where this info should go, as it is pretty extended. Up where the schema def config section is? Somwhere else?

When you use GraphQL in Gloo Edge, you can stitch multiple schemas together to expose one unified GraphQL server to your clients.

For example, consider a cluster to which the `user` and `product` services are deployed. These services are either native GraphQL servers, or have been converted to GraphQL via automatic schema discovery. However, both of these services contribute to a unified data model, which clients must typically stitch together in the frontend. With Gloo Edge, you can instead stitch the GraphQL schemas for these services together in the backend, and expose a unified GraphQL server to your clients. This frees your clients to consider only what data that they want to fetch, not how to fetch the data.

To understand how stitching occurs, consider both of the services, starting with the user service. The user service provides a partial type definition for the `User` type, and a query for how to get the full name of a user given the username.
```yaml
type User {
  username: String!
  fullName: String
}

type Query {
  getUserWithFullName(username: String!): User
}
```

Example query to the user service:
```yaml
query {
  getUserWithFullName(username: "akeith") {
    fullName
  }
}
```

Example response from the user service:
```json
{
  "getUserWithFullName": "Abigail Keith"
}
```

The product service also provides a partial type definition for the `User` type, and a query for how to get the product name and the seller's username given the product ID.
```yaml
type User {
  username: String!
}


type Product{
  id: ID!
  name: String!
  seller: User!
}

type Query {
  getProductById(id: ID!): Product!
}
```

Example query to the product service:
```yaml
query {
  getProductById(id: 125) {
    name
    seller {
      username
    }
  }
}
```

Example response from the product service:
```json
{
  "getProductById": {
    "name": "Narnia",
    "seller": {
      "username": "akeith"
    }
  }
}
```

But consider a client that wants the full name of the seller for a given product, instead the username of the seller. Given the product ID, the client cannot get the seller's full name from the product service. However, the full name of any user _is_ provided by the user service. 

To solve this problem, you can specify a configuration file to merge the types between the services. In the `merge_config` section for a `user-service` configuration file, you can specify which fields are unique to the `User` type, and how to get these fields. If a client provides the username for a user and wants the full name, Gloo Edge can use the `getUserWithFullName` query to provide the full name from the user service.
TODO is the user providing this merging config somewhere??
```yaml
name: user-service
namespace: products-app
merge_config:
  User:
    query_field: getUserWithFullName
    key: username
```

Similarly, in the `merge_config` section for a `product-service` configuration file, you can specify which fields are unique to the `User` type, and how to get these fields. If a client provides the product ID and wants the product name, Gloo Edge can use the `getProductByID` query to provide the product ID from the product service.
```yaml
name: product-service
namespace: products-app
mergeConfig:
  Product:
    queryName: getProductById
    key: id
```

As a result, Gloo Edge generates a **stitched service**. From this one stitched service, a client can provide the product ID, and recieve the product name, the full name of the seller, and the username of the seller.
```yaml
type User {
  username: String!
  fullName: String
}


type Product{
  id: ID!
  name: String!
  seller: User!
}

type Query {
  getProductById(id: ID!): Product!
}
```

Based on this stitched service information, the following schema definition is generated, which incorporates all the types and queries from each of the respective services. In the background, Gloo Edge uses this schema to create the requests to the stitched service, and then stitches the responses back together into one response to the client.
```yaml
schema_definition: |
  type Query {
    getUserWithFullName(username: String!): User
    getProductById(productId: ID!): Product!
  }

  type User {
    username: String!
    fullName: String
  }

  type Product {
    id: ID!
    name: String!
    seller: User!
  }
```

Example query to the stitched service:
```yaml
query {
  getProductById(id: 125) {
    name
    seller {
      username
      fullName
    }
  }
}
```

Example response from the stitched service:
```json
{
  "getProductById": {
    "name": "Narnia",
    "seller": {
      "username": "akeith"
      "fullName": "Abigail Keith"
    }
  }
}
```