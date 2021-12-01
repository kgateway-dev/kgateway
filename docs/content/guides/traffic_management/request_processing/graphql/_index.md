---
title: GraphQL (Enterprise)
weight: 120
description: Enables graphql resolution
---

Set up API gateway and GraphQL server functionality for your apps without running in the same process as Gloo Edge.

{{% notice note %}}
This feature is available only in Gloo Edge Enterprise version 1.10.0-beta1 and later.
{{% /notice %}}

{{% notice warning %}}
This feature is experimental. Do not use this feature in a production environment.
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

## Installing GraphQL

GraphQL resolution is an experimental feature included in Gloo Edge Enterprise version 1.10.0-beta1 and later.

To try out GraphQL, install Gloo Edge in a development environment. Note that you currenty cannot update an existing installation to use GraphQL. Be sure to specify version 1.10.0-beta1 or later. For the latest available version, see the [Gloo Edge Enterprise changelog]({{% versioned_link_path fromRoot="/reference/changelog/enterprise/" %}}).
```
glooctl install gateway enterprise --license-key=<LICENESE_LEY> --version v1.10.0-beta1
```

Next, you can try out GraphQL filtering with sample apps such as [Pet Store](#pet-store) and [Bookinfo](#bookinfo).

## Example: GraphQL with Pet Store {#pet-store}

Use GraphQL resolution with the Pet Store sample application.

**Before you begin**: Deploy the Pet Store sample application, which you will expose behind a GraphQL server embedded in Envoy.
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

This JSON output is filtered only for the desired data, as compared to the unfiltered response that the Pet Store app returned to the GraphQL server:
```json
[{"id":1,"name":"Dog","status":"available"},{"id":2,"name":"Cat","status":"pending"}]
```
Data filtering is one advantage of using GraphQL instead of querying the upstream directly. Because the GraphQL query is issued for only the name of the pets, GraphQL is able to filter out any data in the response that is irrelevant to the query, and return only the data that is specifically requested.

## Example: GraphQL with Bookinfo {#bookinfo}

Use GraphQL resolution with the Bookinfo sample application.

1. Download and install Istio. For more information, see the [Istio getting started documentation](https://istio.io/docs/setup/getting-started/).
```bash
curl -L https://istio.io/downloadIstio | ISTIO_VERSION=1.11.4 sh -
cd istio-1.11.4
istioctl install --set profile=demo
```

2. Verify that all Istio pods have a status of **Running** or **Completed**.
```sh
kubectl get pods -n istio-system
```

3. Enable Istio injection for the default namespace.
```bash
kubectl label namespace default istio-injection=enabled
```

4. Deploy the Bookinfo sample application to the default namespace, which you will expose behind a GraphQL server embedded in Envoy.
```bash
kubectl apply -f samples/bookinfo/platform/kube/bookinfo.yaml
```

5. Create a virtual service that defines a `Route` with a `graphqlSchemaRef` as the
destination. In this example, all traffic to `/graphql` is handled by the GraphQL server in the Envoy proxy. 
{{< highlight yaml "hl_lines=23-27" >}}
cat << EOF | kubectl apply -f -
apiVersion: gateway.solo.io/v1
kind: VirtualService
metadata:
  name: default
  namespace: gloo-system
spec:
  virtualHost:
    domains:
    - '*'
    options:
      cors:
        allowCredentials: true
        allowHeaders:
        - apollo-query-plan-experimental
        - content-type
        - x-apollo-tracing
        allowMethods:
        - POST
        allowOriginRegex:
        - \*
    routes:
    - graphqlSchemaRef:
        name: bookinfo-graphql
        namespace: gloo-system
      matchers:
      - prefix: /graphql
EOF
{{< /highlight >}}

6. Create the `GraphQLSchema` CR, which contains the schema and information required to resolve it.
{{< highlight yaml "hl_lines=80-109" >}}
cat << EOF | kubectl apply -f -
apiVersion: graphql.gloo.solo.io/v1alpha1
kind: GraphQLSchema
metadata:
  name: bookinfo-graphql
  namespace: gloo-system
spec:
  enableIntrospection: true
  resolutions:
  - matcher:
      fieldMatcher:
        field: productsForHome
        type: Query
    restResolver:
      requestTransform:
        headers:
          :method:
            typedProvider:
              value: GET
          :path:
            typedProvider:
              value: /api/v1/products
      upstreamRef:
        name: default-productpage-9080
        namespace: gloo-system
  - matcher:
      fieldMatcher:
        field: author
        type: Product
    restResolver:
      requestTransform:
        headers:
          :method:
            typedProvider:
              value: GET
          :path:
            graphqlParent:
              path:
                - key: id
            providerTemplate: /details/{}
      upstreamRef:
        name:  default-details-9080
        namespace: gloo-system
  - matcher:
      fieldMatcher:
        field: review
        type: Product
    restResolver:
      requestTransform:
        headers:
          :method:
            typedProvider:
              value: GET
          :path:
            graphqlParent:
              path:
                - key: id
            providerTemplate: /reviews/{}
      upstreamRef:
        name:  default-reviews-9080
        namespace: gloo-system
  - matcher:
      fieldMatcher:
        field: ratings
        type: Product
    restResolver:
      requestTransform:
        headers:
          :method:
            typedProvider:
              value: GET
          :path:
            graphqlParent:
              path:
                - key: id
            providerTemplate: /ratings/{}
      upstreamRef:
        name:  default-ratings-9080
        namespace: gloo-system
  schema: "
    type Query {
      productsForHome: [Product]
    }

    type Product {
      id: String
      title: String
      descriptionHtml: String
      author: String
      pages: Int
      year: Int
      review: ProductReview
      ratings: [Rating]
    }

    type ProductReview {
      reviews : [Review]
    }

    type Review {
      reviewer: String
      text: String
    }

    type Rating {
      reviewer: String
      numStars: Int
    }
    "
EOF
{{< /highlight >}}

7. Port forward the gateway endpoint.
```sh
kubectl port-forward -n gloo-system deploy/gateway-proxy 8080
```

8. Point your favorite GraphQL client to `http://localhost:8080/graphql`. For example, you might go to `https://studio.apollographql.com/sandbox/explorer` to specify this URL for the GraphQL server.

TODO: any recommendations for playing around in apollo?

## Try it yourself

TODO: need guidance on how to develop the `GraphQLSchema` CR for their own use cases and apps

## Next steps

To learn more about the advantages of using GraphQL, see the [Apollo documentation](https://www.apollographql.com/docs/intro/benefits/#graphql-provides-declarative-efficient-data-fetching).