---
title: Manual schema configuration
weight: 40
description: Manually configure resolvers and schema for your GraphQL API.
---

You can deploy your own GraphQL API, which might not leverage automatic service discovery and registration. To manually configure GraphQL resolvers, you create a Gloo Edge GraphQL API CRD. 

The following sections describe the configuration for local or remote query resolution, schema definitions for the types of data to return to GraphQL queries, and an in-depth example.

## Define REST and gRPC resolvers for local execution

If your upstream does not define GraphQL resolvers, you can define resolvers in your `GraphQLApi` resource. In this case, Gloo Edge uses _local execution_, which means the Envoy server executes GraphQL queries locally by using the defined resolvers. Then, it proxies the executed requests to the upstreams that provide the data requested in the queries.

### REST resolvers

Configure a REST resolver as a section within your `GraphQLApi` YAML file.

```yaml
...
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
apiVersion: graphql.gloo.solo.io/v1beta1
kind: GraphQLApi
metadata:
  name: bookinfo-graphql
  namespace: product-app
spec:
  executableSchema:
    executor:
      local:
        enableIntrospection: true
        resolutions:
          Query|productsForHome:
            restResolver:
              request:
                headers:
                  :method: GET
                  :path: /api/v1/products
              upstreamRef:
                name: default-productpage-9080
                namespace: product-app
        ...
```

### gRPC resolvers

Configure a gRPC resolver as a section within your `GraphQLApi` YAML file.

```yaml
...
resolutions:
  # Resolver name
  Query|nameOfResolver:
    grpcResolver:
      # Configuration for generating outgoing requests to a gRPC API
      requestTransform:
        # JSON representation of outgoing gRPC message to be sent to JSON service
        outgoingMessageJson:
          <key>: <value>
        # The full name of the JSON service
        serviceName:
        # Method on the gRPC service defined in serviceName to make request to 
        methodName:
      upstreamRef:
        # Name of the upstream resource associated with the REST API  
        name:
        # The namespace the upstream resource
        namespace:
```

This example gRPC resolver, `Query|UserService.GetUser`, specifies the `GetUser` method on the `user.UserService` service, and the `user-svc` upstream service.
```yaml
apiVersion: graphql.gloo.solo.io/v1beta1
kind: GraphQLApi
metadata:
  name: products-graphql
  namespace: product-app
spec:
  executableSchema:
    executor:
      local:
        enableIntrospection: true
        resolutions:
          Query|UserService.GetUser:
            grpcResolver:
              requestTransform:
                methodName: GetUser
                outgoingMessageJson:
                  username: '{$args.username}'
                serviceName: user.UserService
              upstreamRef:
                name: user-svc
                namespace: product-app
        ...
```

## Remote executor configuration for existing GraphQL server upstreams

{{% notice note %}}
Remote execution is supported only in versions 1.14.0 and later.
{{% /notice %}}

When your upstream service is already a GraphQL server that includes its own resolvers, use a `remote` executor in the corresponding `GraphQLApi` resource. The remote executor tells the `GraphQLApi` to use the resolver in the upstream to resolve requests for remote execution. You do not need to define another resolver within the `GraphQLApi`.

```yaml
...
spec:
  executableSchema:
    executor:
      remote:
        upstreamRef:
          # Name of the upstream GraphQL API  
          name:
          # The namespace the upstream API
          namespace: 
```

Example in which a GraphQL API, `bookinfo-graphql`, is referenced as the upstream:

```yaml
apiVersion: graphql.gloo.solo.io/v1beta1
kind: GraphQLApi
metadata:
  name: products-graphql
  namespace: product-app
spec:
  executableSchema:
    executor:
      remote:
        upstreamRef:
          name: bookinfo-graphql
          namespace: product-app
  ...
```

## Schema definitions

A schema definition determines what kind of data can be returned to a client that makes a GraphQL query to your endpoint. The schema specifies the data that a particular `type`, or service, returns in response to a GraphQL query.

In this example, fields are defined for the three Bookinfo services, Product, Review, and Rating. Additionally, the schema definition indicates which services reference the resolvers. In this example, the Product service references the `Query|productForHome` REST resolver.

```yaml
apiVersion: graphql.gloo.solo.io/v1beta1
kind: GraphQLApi
metadata:
  name: bookinfo-graphql
  namespace: product-app
spec:
  executableSchema:
    executor:
      ...
    schemaDefinition: |
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
        namespace: product-app
      matchers:
      - prefix: /graphql
EOF
{{< /highlight >}}

## Reference

For more information, see the [Gloo Edge API reference for GraphQL]({{% versioned_link_path fromRoot="/reference/api/github.com/solo-io/gloo/projects/gloo/api/v1/enterprise/options/graphql/v1beta1/graphql.proto.sk/" %}}).