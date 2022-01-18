---
title: GraphQL (Enterprise)
weight: 120
description: Enables GraphQL resolution
---

Set up API gateway and GraphQL server functionality for your apps in the same process as Gloo Edge.

{{% notice note %}}
This feature is available only in Gloo Edge Enterprise version 1.10.0-beta1 and later.
{{% /notice %}}

{{% notice warning %}}
This feature is experimental. Do not use this feature in a production environment.
{{% /notice %}}

## About

### Why GraphQL?
GraphQL is a server-side query language and runtime you can use to expose your APIs as an alternative to REST APIs. GraphQL allows you to request only the data you want and handle any subsequent requests on the server side, saving numerous expensive origin-to-client requests by instead handling requests in your internal network.

### Using GraphQL in an API gateway
API gateways solve the problem of exposing multiple microservices with differing implementations from a single location and scheme, and by talking to a single owner. GraphQL integrates well with API gateways by exposing your API without versioning and allowing clients to interact with backend APIs on their own terms. Additionally, you can mix and match your GraphQL graph with your existing REST routes to test GraphQL integration features and migrate to GraphQL at a pace that makes sense for your organization.

Gloo Edge solves the problems that other API gateways face when exposing GraphQL services by allowing you to configure GraphQL at the route level. API gateways are often used to rate limit, authorize and authenticate, and inject other centralized edge networking logic at the route level. However, because most GraphQL servers are exposed as a single endpoint within an internal network behind API gateways, you cannot add route-level customizations. With Gloo Edge, route-level customization logic is embedded into the API gateway.

## Step 1: Install GraphQL

GraphQL resolution is an experimental feature included in Gloo Edge Enterprise version 1.10.0-beta1 and later.

To try out GraphQL, install Gloo Edge in a development environment. Note that you currenty cannot update an existing installation to use GraphQL. Be sure to specify version 1.10.0-beta1 or later. For the latest available version, see the [Gloo Edge Enterprise changelog]({{% versioned_link_path fromRoot="/reference/changelog/enterprise/" %}}).
```
glooctl install gateway enterprise --version 1.11.0-beta3 --license-key=<LICENSE_LEY>
```

Next, you can try out GraphQL filtering with sample apps such as [Pet Store](#pet-store) and [Bookinfo](#bookinfo).

## Step 2: GraphQL service discovery with Pet Store {#pet-store}

Explore GraphQL service discovery with the Pet Store sample application.

1. Start by deploying the Pet Store sample application, which you will expose behind a GraphQL server embedded in Envoy.
   ```sh
   kubectl apply -f https://raw.githubusercontent.com/solo-io/gloo/master/example/petstore/petstore.yaml
   ```
   Note that any `/GET` requests to `/api/pets` of this service return the following JSON output:
   ```json
   [{"id":1,"name":"Dog","status":"available"},{"id":2,"name":"Cat","status":"pending"}]
   ```

2. To allow Gloo Edge to automatically discover API specifications and create GraphQL schemas, turn on FDS discovery.
   ```sh
   kubectl patch settings -n gloo-system default --type=merge --patch '{"spec":{"discovery":{"fdsMode":"BLACKLIST"}}}'
   ```

3. Verify that OpenAPI specification discovery is enabled, and that Gloo Edge created a corresponding GraphQL custom resource.
   ```sh
   kubectl get graphqlschemas -n gloo-system
   ```

   Example output:
   ```
   NAME                    AGE
   default-petstore-8080   2m58s
   ```

4. Optional: Check out the generated GraphQL schema.
   ```sh
   kubectl get graphqlschemas default-petstore-8080 -o yaml -n gloo-system
   ```

5. Create a virtual service that defines a `Route` with a `graphqlSchemaRef` as the destination. In this example, all traffic to `/graphql` is handled by the GraphQL server in the Envoy proxy. 
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

6. Send a request to the endpoint to verify that the request is successfully resolved by Envoy.
   ```sh
   curl "$(glooctl proxy url)/graphql" -H 'Content-Type: application/json' -d '{"query": "query {getPetById(petId: 2) {name}}"}'
   ```
   Example successful response:
   ```json
   {"data":{"getPetById":{"name":"Cat 2"}}}
   ```

This JSON output is filtered only for the desired data, as compared to the unfiltered response that the Pet Store app returned to the GraphQL server:
```json
[{"id":1,"name":"Dog","status":"available"},{"id":2,"name":"Cat","status":"pending"}]
```
Data filtering is one advantage of using GraphQL instead of querying the upstream directly. Because the GraphQL query is issued for only the name of the pets, GraphQL is able to filter out any data in the response that is irrelevant to the query, and return only the data that is specifically requested.

## Step 3: GraphQL resolvers with Bookinfo {#bookinfo}

Next, explore GraphQL resolution with the Bookinfo sample application.

In Gloo Edge, you can create GraphQL resolvers to fetch the data from your backend. Today Gloo Edge supports REST and gRPC resolvers. In the following steps, you create resolvers that point to Bookinfo services and use the resolvers in a GraphQL schema.

1. Start by installing the Istio sample app, Bookinfo.
   1. Download and install Istio. For more information, see the [Istio getting started documentation](https://istio.io/docs/setup/getting-started/).
   ```sh
   curl -L https://istio.io/downloadIstio | ISTIO_VERSION=1.11.4 sh -
   cd istio-1.11.4
   istioctl install --set profile=demo
   ```

   1. Verify that all Istio pods have a status of **Running** or **Completed**.
   ```sh
   kubectl get pods -n istio-system
   ```

   1. Enable Istio injection for the default namespace.
   ```sh
   kubectl label namespace default istio-injection=enabled
   ```

   1. Deploy the Bookinfo sample application to the default namespace, which you will expose behind a GraphQL server embedded in Envoy.
   ```sh
   kubectl apply -f samples/bookinfo/platform/kube/bookinfo.yaml
   ```

2. Verify that Gloo Edge automatically discovered the Bookinfo services and created corresponding `default-productpage-9080` upstream, which you will use in the REST resolver.
```sh
kubectl get upstream -n gloo-system
```

3. Check out the contents of the following Gloo Edge GraphQL schema CRD. Specifically, take a look at the `restResolver` and `schema_definition` sections.
   ```sh
   curl https://raw.githubusercontent.com/kcbabo/graphql-bookinfo/main/kubernetes/bookinfo-gql.yaml
   ```
   * `restResolver`: A resolver is defined by a name (ex: Query|productsForHome) and whether it is a REST or a gRPC resolver. This example is a REST resolver, so the path and the method that are needed to request the data are specified. The path can reference a parent attribute, such as `/details/{$parent.id}.`
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
   * `schema_definition`: A schema definition defines queries and types that reference the resolvers.
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

4. Create the GraphQL schema CRD in your cluster to expose the GraphQL API that fetches data from the three Bookinfo services.
   ```sh
   kubectl apply -f https://raw.githubusercontent.com/kcbabo/graphql-bookinfo/main/kubernetes/bookinfo-gql.yaml -n gloo-system
   ```

5. Update the `default` virtual service that you previously created to route traffic to `/graphql` to the new `bookinfo-graphql` GraphQL schema. 
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
        name: bookinfo-graphql
        namespace: gloo-system
      matchers:
      - prefix: /graphql
EOF
{{< /highlight >}}

6. Send a request to the endpoint to verify that the request is successfully resolved by Envoy.
   ```sh
   curl "$(glooctl proxy url)/graphql" -H 'Content-Type: application/json' -d '{"query": "query {productsForHome {id, title, author, pages, year}}"}'
   ```
   Example successful response:
   ```json
   {"data":{"productsForHome":[{"id":"0","title":"The Comedy of Errors","author":"William Shakespeare","pages":200,"year":1595}]}}
   ```

## Try it yourself

TODO: need guidance on how to develop the `GraphQLSchema` CR for their own use cases and apps

## Next steps

To learn more about the advantages of using GraphQL, see the [Apollo documentation](https://www.apollographql.com/docs/intro/benefits/#graphql-provides-declarative-efficient-data-fetching).