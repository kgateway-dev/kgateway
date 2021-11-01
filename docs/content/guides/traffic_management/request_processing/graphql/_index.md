---
title: GraphQL (Enterprise)
weight: 120
description: Enables graphql resolution
---

## Why GraphQL?
GraphQL is a server-side query language and runtime you can use to expose your APIs. As an alternative to REST APIs.
GraphQL is powerful because it allows you to request only the data you want, and handle any following requests on
the server-side, saving potentially numerous expensive origin to client requests and instead handling them in your
internal network.

## Why GraphQL in an API Gateway?
API gateways solve the problem of exposing multiple microservices with perhaps differing implementations from a single
location, scheme, and by talking to a single team/owner. GraphQL integrates beautifully with API gateways by exposing
your API without versioning and allowing clients to interact.

## How to use GraphQL with Gloo Edge

```shell
kubectl apply -f https://raw.githubusercontent.com/solo-io/gloo/v1.2.9/example/petstore/petstore.yaml
```

To use GraphQL to resolve requests in Gloo Edge, we need to define a `Route` that has a `grahqp_schema_ref` as the
destination. We can do this using the following `VirtualService` as seen below.

In the example below, all traffic going to `/graphql` is being handled by the GraphQL server in our envoy proxy.
{{< highlight yaml "hl_lines=19-23" >}}
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
    - matchers:
       - prefix: '/graphql'
      graphqlSchemaRef:
        name: 'gql'
        namespace: 'gloo-system'
{{< /highlight >}}

Now we need to define the `GraphQLSchema` CR, which contains the schema and information required to resolve it.
For example:
{{< highlight yaml "hl_lines=19-23" >}}
apiVersion: graphql.gloo.solo.io/v1alpha1
kind: GraphQLSchema
metadata:
  name: gql
  namespace: gloo-system
spec:
  resolutions:
  - matcher:
      fieldMatcher:
        type: Query
        field: pets
    restResolver:
      requestTransform:
        headers:
          ':method':
            typedProvider:
              type: STRING
              value: 'GET'
          ':authority':
            typedProvider:
              type: STRING
              value: 'localhost:8080'
          ':path':
            typedProvider:
              type: STRING
              value: '/api-pets'
      upstreamRef:
        name: default-petstore-8080
        namespace: gloo-system
  schema: "schema { query: Query } type Query { pets: [Pet] } type Pet { name: String }"
{{< /highlight >}}

Now we can make requests to our endpoint and see them resolved by envoy:
```shell
curl "$(glooctl proxy url)/graphql" -H 'Content-Type: application/json' -d '{"query":"{pets{name}}"}'
```

And we should get the following response:
```shell
TODO
```

The rest request returned to our graphql server returned the following json:
```json
[{"id":1,"name":"Dog","status":"available"},{"id":2,"name":"Cat","status":"pending"}]
```
note that in our response we trimmed this to only the fields we queried for! (the name) 