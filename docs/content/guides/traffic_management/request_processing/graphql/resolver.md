---
title: TODO
weight: 10
description: TODO
---

TODO shortdesc

## Automatic resolver generation with GraphQL service discovery

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

## Manual configuration of resolvers

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

### Sample GraphQL API

To get started with your own GraphQL API, check out the in-depth example in the [`graphql-bookinfo` repository](https://github.com/solo-io/graphql-bookinfo). You can model your own use case based on the contents of this example:
* The `kubernetes` directory contains the Bookinfo sample app deployment, the example GraphQL schema, and the virtual service to route requests to the `/graphql` endpoint.
* The `openapi` directory contains the OpenAPI specifications for the individual BookInfo microservices, along with the original consolidated BookInfo REST API.

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

We will have a really rough version of this at SoloCon and will be adding more leading up to GA.
https://docs.google.com/presentation/d/1ArxEdVVAOowz4wYcXIYlg4Wd-psadUTrOYH9DPPWwpk/edit#slide=id.g114557a7576_0_488

When you use GraphQL in Gloo Edge, you can stitch multiple schemas together to expose one unified GraphQL server to your clients.

For example, consider a cluster in which 3 different services are deployed. These services are all either native GraphQL servers, or have been converted to GraphQL via automatic schema discovery. However, each of these services contribute to a unified data model, which clients must typically stitch together in the frontend. With Gloo Edge, you can instead stitch the GraphQL schemas for these services together in the backend, and expose a unified GraphQL server to your clients. This frees your clients to consider only what data that they want to fetch, not how to fetch the data. Let's dive into what this "schema stitching" would look like.