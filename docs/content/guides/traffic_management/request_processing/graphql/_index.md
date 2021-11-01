---
title: GraphQL (Enterprise)
weight: 120
description: Enables graphql resolution
---

{{% notice note %}}
Available in Gloo Edge Enterprise as of v1.10.0-beta1.
{{% /notice %}}

## Why GraphQL?
GraphQL is a server-side query language and runtime you can use to expose your APIs. As an alternative to REST APIs.
GraphQL is powerful because it allows you to request only the data you want, and handle any following requests on
the server-side, saving potentially numerous expensive origin to client requests and instead handling them in your
internal network.

## Why GraphQL in an API Gateway?
API gateways solve the problem of exposing multiple microservices with perhaps differing implementations from a single
location, scheme, and by talking to a single team/owner. GraphQL integrates beautifully with API gateways by exposing
your API without versioning and allowing clients to interact with backend APIs on their own terms. Developers are
enabled to leverage GraphQL's improved documentation flows to increase developer productivity. Best of all, you can
continue to mix and match your GraphQL graph with your pre-existing REST routes, so you can test the tradeoffs and
migrate between the two at a pace that makes sense in your organization.

Gloo Edge specifically solves the problems other API gateways face when exposing GraphQL services by allowing users
to configure GraphQL at the route level. API Gateways are often used to rate-limit, authorize/authenticate, and inject
other centralized edge networking logic at the route level, but most GraphQL servers are exposed as a single endpoint
within an internal network. Thus, it is impossible to add route-level customizations to most GraphQL servers exposed
behind API Gateways, unless that logic is embedded into the API gateway, as it is with Gloo Edge.

## Example: GraphQL with Gloo Edge

##### Prereqs

First, let's deploy our sample petstore application, which we will expose behind a GraphQL server embedded in envoy:
```shell
kubectl apply -f https://raw.githubusercontent.com/solo-io/gloo/v1.2.9/example/petstore/petstore.yaml
```

Remember from the hello world demo that any `/GET` requests to this service at `/api/pets` will return the following
json:
```json
[{"id":1,"name":"Dog","status":"available"},{"id":2,"name":"Cat","status":"pending"}]
```

##### Gloo Edge Specifics

To use GraphQL to resolve requests in Gloo Edge, we need to define a `Route` that has a `graphqlSchemaRef` as the
destination. We can do this using the following `VirtualService` as seen below.

In the example below, all traffic going to `/graphql` is being handled by the GraphQL server in our envoy proxy.
{{< highlight yaml "hl_lines=11-15" >}}
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
{{< highlight yaml "hl_lines=30-30" >}}
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
{{< /highlight >}}

Now we can make requests to our endpoint and see them resolved by envoy:
```shell
curl "$(glooctl proxy url)/graphql" -H 'Content-Type: application/json' -d '{"query":"{pets{name}}"}'
```

And get the following response:
```json
{"data":{"pets":[{"name":"Dog"},{"name":"Cat"}]}}
```

Remember that the REST request returned to our graphql server was the following json:
```json
[{"id":1,"name":"Dog","status":"available"},{"id":2,"name":"Cat","status":"pending"}]
```
Here we can see one advantage of using GraphQL over querying the upstream directly. We issued a GraphQL query for only the name of the pets, and though the upstream could return a lot more data, GraphQL was able to filter out only the data which was relevant to our query. In this case, we queried for the names of the pets, and we got back only the names of the pets. 

Read more about the advantages of using GraphQL [here](https://www.apollographql.com/docs/intro/benefits/#graphql-provides-declarative-efficient-data-fetching).