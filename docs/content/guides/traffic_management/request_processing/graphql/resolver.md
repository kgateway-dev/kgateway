## Try it yourself

You can deploy your own GraphQL API, which might not leverage automatic service discovery and registration.

To get started, check out the in-depth example in the [`graphql-bookinfo` repository](https://github.com/solo-io/graphql-bookinfo). You can model your own use case based on the contents of this example:
* The `kubernetes` directory contains the Bookinfo sample app deployment, the example GraphQL schema, and the virtual service to route requests to the `/graphql` endpoint.
* The `openapi` directory contains the OpenAPI specifications for the individual BookInfo microservices, along with the original consolidated BookInfo REST API.



## Automatic generation
Sai has a great overview here:
https://docs.google.com/presentation/d/1ArxEdVVAOowz4wYcXIYlg4Wd-psadUTrOYH9DPPWwpk/edit#slide=id.g10a8760f3dd_24_181


## Resolver configuration

In a Gloo Edge GraphQL schema CRD, you define REST or gRPC resolvers, and schema definitions for the types of data to return to graphQL queries.

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

## Caching
There are three parts to this:
How we automatically cache upstream responses in the resolver layer. No user configuration required - we’re just describing the behavior and why it’s good.
Automatic persisted queries - 
https://github.com/solo-io/envoy-gloo-ee/issues/272
Response Caching
This is basically piggybacking on the response caching support we’re doing for Edge https://github.com/solo-io/envoy-gloo-ee/issues/260


## Stitching
We will have a really rough version of this at SoloCon and will be adding more leading up to GA.
https://docs.google.com/presentation/d/1ArxEdVVAOowz4wYcXIYlg4Wd-psadUTrOYH9DPPWwpk/edit#slide=id.g114557a7576_0_488