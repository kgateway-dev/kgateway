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
resolutions:
- Resolver:
  RestResolver:
  request_transform:
  OutgoingBody: null
  upstream_ref:
  name: local-1
  namespace: default
  matcher:
  Match:
  FieldMatcher:
  field: field1
  type: Query
- Resolver:
  RestResolver:
  upstream_ref:
  name: local-2
  namespace: default
  matcher:
  Match:
  FieldMatcher:
  field: child
  type: SimpleType
  schema: "\n\t\t      schema { query: Query }\n\t\t      input Map {\n\t\t        a:
  Int!\n\t\t      }\n\t\t      type Query {\n\t\t        field1(intArg: Int!, boolArg:
  Boolean!, floatArg: Float!, stringArg: String!, mapArg: Map!, listArg: [Int!]!):
  SimpleType\n\t\t      }\n\t\t      type SimpleType {\n\t\t        simple: String\n\t\t
  \       child: String\n\t\t      }\n"
{{< /highlight >}}