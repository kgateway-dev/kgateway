---
title: GraphQL (Enterprise)
weight: 120
description: Enables graphql resolution
---

{{% notice note %}}
This feature is available only in Gloo Edge Enterprise version 1.10.0-beta1 and later.
{{% /notice %}}

## Why GraphQL?
GraphQL is a server-side query language and runtime you can use to expose your APIs as an alternative to REST APIs.
GraphQL allows you to request only the data you want and handle any subsequent requests on
the server side, saving numerous expensive origin-to-client requests by instead handling requests in your
internal network.

## Why GraphQL in an API gateway?
API gateways solve the problem of exposing multiple microservices with differing implementations from a single
location and scheme, and by talking to a single owner. GraphQL integrates well with API gateways by exposing
your API without versioning and allowing clients to interact with backend APIs on their own terms. Additionally, you can
mix and match your GraphQL graph with your existing REST routes to test GraphQL integration features and
migrate to GraphQL at a pace that makes sense for your organization.

Gloo Edge solves the problems that other API gateways face when exposing GraphQL services by allowing you
to configure GraphQL at the route level. API gateways are often used to rate limit, authorize and authenticate, and inject
other centralized edge networking logic at the route level. However, because most GraphQL servers are exposed as a single endpoint
within an internal network behind API gateways, you cannot add route-level customizations.
With Gloo Edge, route-level customization logic is embedded into the API gateway.

## Example: GraphQL with Gloo Edge

**Before you begin**: Deploy the sample pet store application, which you will expose behind a GraphQL server embedded in Envoy.
```shell
kubectl apply -f https://raw.githubusercontent.com/solo-io/gloo/v1.2.9/example/petstore/petstore.yaml
```
Note that any `/GET` requests to `/api/pets` of this service return the following JSON output:
```json
[{"id":1,"name":"Dog","status":"available"},{"id":2,"name":"Cat","status":"pending"}]
```

1. Create a virtual service that defines a `Route` with a `graphqlSchemaRef` as the
destination. In this example, all traffic to `/graphql` is handled by the GraphQL server in the Envoy proxy. 
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
    - matchers:
       - prefix: '/graphql'
      graphqlSchemaRef:
        name: 'gql'
        namespace: 'gloo-system'
EOF
{{< /highlight >}}

2. Create the `GraphQLSchema` CR, which contains the schema and information required to resolve it.
{{< highlight yaml "hl_lines=25-25" >}}
cat << EOF | kubectl apply -f -
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
              value: 'GET'
          ':path':
            typedProvider:
              value: '/api/pets'
      upstreamRef:
        name: default-petstore-8080
        namespace: gloo-system
  schema: "schema { query: Query } type Query { pets: [Pet] } type Pet { name: String }"
EOF
{{< /highlight >}}

3. Send a request to the endpoint to verify that the request is successfully resolved by Envoy.
   ```shell
   curl "$(glooctl proxy url)/graphql" -H 'Content-Type: application/json' -d '{"query":"{pets{name}}"}'
   ```
   Example successful response:
   ```json
   {"data":{"pets":[{"name":"Dog"},{"name":"Cat"}]}}
   ```

Remember that previously the REST request returned the following JSON to the GraphQL server:
```json
[{"id":1,"name":"Dog","status":"available"},{"id":2,"name":"Cat","status":"pending"}]
```
Data filtering is one advantage of using GraphQL instead of querying the upstream directly. Because the GraphQL query is issued for only the name of the pets, GraphQL is able to filter out any data in the response that is irrelevant to the query, and return only the data that is specifically requested.

To learn more about the advantages of using GraphQL, see the [Apollo documentation](https://www.apollographql.com/docs/intro/benefits/#graphql-provides-declarative-efficient-data-fetching).