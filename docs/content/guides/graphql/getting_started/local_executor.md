---
title: Local GraphQL resolution
weight: 30
description: Explore local GraphQL resolution with the Bookinfo sample application.
---

Next, explore local GraphQL resolution with the Bookinfo sample application. 

In Gloo Edge, you can create GraphQL resolvers to fetch the data from your backend when using _local execution_. Today Gloo Edge supports _local execution_ with REST and gRPC resolvers, and _remote execution_ for GraphQL servers. In the following steps, you create resolvers that point to Bookinfo services and use the resolvers in a GraphQL schema.

Note that this example uses _local execution_, which means the Envoy server executes GraphQL queries locally before it proxies them to the Bookinfo upstreams that provide the data requested in the queries.

1. Deploy the Bookinfo sample application to the default namespace, which you will expose behind a GraphQL server embedded in Envoy.
   ```sh
   kubectl apply -f https://raw.githubusercontent.com/istio/istio/master/samples/bookinfo/platform/kube/bookinfo.yaml
   ```

2. Verify that Gloo Edge automatically discovered the Bookinfo services and created corresponding `default-productpage-9080` upstream, which you will use in the REST resolver.
   ```sh
   kubectl get upstream -n gloo-system
   ```

3. Check out the contents of the following Gloo Edge GraphQL API CRD. Specifically, take a look at the `restResolver` and `schemaDefinition` sections.
   ```sh
   curl https://raw.githubusercontent.com/solo-io/graphql-bookinfo/main/kubernetes/bookinfo-gql.yaml
   ```
   * `restResolver`: A resolver is defined by a name (ex: `Query|productsForHome`) and whether it is a REST or a gRPC resolver. This example is a REST resolver, so the path and the method that are needed to request the data are specified. The path can reference a parent attribute, such as `/details/{$parent.id}.`
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
   * `schemaDefinition`: A schema definition determines what kind of data can be returned to a client that makes a GraphQL query to your endpoint. The schema specifies the data that a particular `type`, or service, returns in response to a GraphQL query. In this example, fields are defined for the three Bookinfo services, Product, Review, and Rating. Additionally, the schema definition indicates which services reference the resolvers. In this example, the Product service references the `Query|productForHome` resolver. 
     ```yaml
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

4. Create the GraphQL API CRD in your cluster to expose the GraphQL API that fetches data from the three Bookinfo services.
   ```sh
   kubectl apply -f https://raw.githubusercontent.com/solo-io/graphql-bookinfo/main/kubernetes/bookinfo-gql.yaml -n gloo-system
   ```

5. Update the `default` virtual service that you previously created to route traffic to `/graphql` to the new `bookinfo-graphql` GraphQL API.
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
        name: bookinfo-graphql
        namespace: gloo-system
      matchers:
      - prefix: /graphql
EOF
   {{< /highlight >}}

1. Send a request to the GraphQL endpoint to verify that the request is successfully resolved by Envoy.
   ```sh
   curl "$(glooctl proxy url)/graphql" -H 'Content-Type: application/json' -d '{"query": "query {productsForHome {id, title, author, pages, year}}"}'
   ```
   In the JSON response, note that only the information you queried is returned:
   ```json
   {"data":{"productsForHome":[{"id":"0","title":"The Comedy of Errors","author":"William Shakespeare","pages":200,"year":1595}]}}
   ```

**Up next**: [Explore remote GraphQL resolution with an example GraphQL server upstream.]({{% versioned_link_path fromRoot="/guides/graphql/getting_started/remote_executor" %}})
