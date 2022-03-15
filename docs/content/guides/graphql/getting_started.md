---
title: Getting started
weight: 10
description: Get started with GraphQL in Gloo Edge.
---

Set up API gateway and GraphQL server functionality for your apps in the same process by using Gloo Edge.

{{% notice note %}}
This feature is available only in Gloo Edge Enterprise version 1.10.0 and later.
{{% /notice %}}

{{% notice warning %}}
This is an alpha feature. Do not use this feature in a production environment.
{{% /notice %}}

## Step 1: Install GraphQL

GraphQL resolution is an alpha feature included in Gloo Edge Enterprise version 1.10.0 and later.

1. [Contact your account representative](https://www.solo.io/company/talk-to-an-expert/) to request a Gloo Edge Enterprise license that specifically enables the GraphQL capability.

2. To try out GraphQL, install Gloo Edge in a development environment. Note that you currently cannot update an existing installation to use GraphQL. Be sure to specify version 1.10.0 or later. For the latest available version, see the [Gloo Edge Enterprise changelog]({{% versioned_link_path fromRoot="/reference/changelog/enterprise/" %}}).
```sh
glooctl install gateway enterprise --version {{< readfile file="static/content/version_gee_latest.md" markdown="true">}} --license-key=<GRAPHQL_ENABLED_LICENSE_LEY>
```

## Step 2: GraphQL service discovery with Pet Store {#pet-store}

Explore GraphQL service discovery with the Pet Store sample application.

1. Start by deploying the Pet Store sample application, which you will expose behind a GraphQL server embedded in Envoy.
   ```sh
   kubectl apply -f - <<EOF
   apiVersion: apps/v1
   kind: Deployment
   metadata:
     labels:
       app: petstore
     name: petstore
     namespace: default
   spec:
     selector:
       matchLabels:
         app: petstore
     replicas: 1
     template:
       metadata:
         labels:
           app: petstore
       spec:
         containers:
         - image: openapitools/openapi-petstore
           name: petstore
           env:
             - name: DISABLE_OAUTH
               value: "1"
             - name: DISABLE_API_KEY
               value: "1"
           ports:
           - containerPort: 8080
             name: http
   ---
   apiVersion: v1
   kind: Service
   metadata:
     name: petstore
     namespace: default
     labels:
       service: petstore
   spec:
     ports:
     - port: 8080
       protocol: TCP
     selector:
       app: petstore
   EOF
   ```
   Optional: You can [create a route and send a `/GET` request to `/api/pets` of this service]({{% versioned_link_path fromRoot="/guides/security/auth/custom_auth/#setup" %}}), which returns the following unfiltered JSON output:
   ```json
   [{"id":1,"name":"Dog","status":"available"},{"id":2,"name":"Cat","status":"pending"}]
   ```

2. To allow Gloo Edge to automatically discover API specifications and create GraphQL schemas, turn on FDS discovery.
   ```sh
   kubectl patch settings -n gloo-system default --type=merge --patch '{"spec":{"discovery":{"fdsMode":"BLACKLIST"}}}'
   ```
   Note that this setting enables discovery for all upstreams. To enable discovery for only specified upstreams, see the [Function Discovery Service (FDS) guide]({{% versioned_link_path fromRoot="/installation/advanced_configuration/fds_mode/#function-discovery-service-fds" %}}).

3. Verify that OpenAPI specification discovery is enabled, and that Gloo Edge created a corresponding GraphQL custom resource.
   ```sh
   kubectl get graphqlapis -n gloo-system
   ```

   Example output:
   ```
   NAME                    AGE
   default-petstore-8080   2m58s
   ```

4. Optional: Check out the generated GraphQL schema. 
   ```sh
   kubectl get graphqlapis default-petstore-8080 -o yaml -n gloo-system
   ```

5. Create a virtual service that defines a `Route` with a `graphqlApiRef` as the destination. In this example, all traffic to `/graphql` is handled by the GraphQL server in the Envoy proxy.
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

1. Deploy the Bookinfo sample application to the default namespace, which you will expose behind a GraphQL server embedded in Envoy.
   ```sh
   kubectl apply -f https://raw.githubusercontent.com/istio/istio/master/samples/bookinfo/platform/kube/bookinfo.yaml
   ```

2. Verify that Gloo Edge automatically discovered the Bookinfo services and created corresponding `default-productpage-9080` upstream, which you will use in the REST resolver.
   ```sh
   kubectl get upstream -n gloo-system
   ```

3. Check out the contents of the following Gloo Edge GraphQL API CRD. Specifically, take a look at the `restResolver` and `schema_definition` sections.
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
   * `schema_definition`: A schema definition determines what kind of data can be returned to a client that makes a GraphQL query to your endpoint. The schema specifies the data that a particular `type`, or service, returns in response to a GraphQL query. In this example, fields are defined for the three Bookinfo services, Product, Review, and Rating. Additionally, the schema definition indicates which services reference the resolvers. In this example, the Product service references the `Query|productForHome` resolver. 
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

6. Send a request to the GraphQL endpoint to verify that the request is successfully resolved by Envoy.
   ```sh
   curl "$(glooctl proxy url)/graphql" -H 'Content-Type: application/json' -d '{"query": "query {productsForHome {id, title, author, pages, year}}"}'
   ```
   In the JSON response, note that only the information you queried is returned:
   ```json
   {"data":{"productsForHome":[{"id":"0","title":"The Comedy of Errors","author":"William Shakespeare","pages":200,"year":1595}]}}
   ```

## Step 4: Secure the GraphQL API

Protect the GraphQL API that you created in the previous sections by using an API key. Note that you can also use any other authorization mechanism provided by Gloo Edge to secure your GraphQL endpoint.

1. Create an API key secret that contains an existing API key. If you want `glooctl` to create an API key for you, you can specify the `--apikey-generate` flag instead of the `--apikey` flag.
   ```sh
   glooctl create secret apikey my-apikey \
   --apikey $API_KEY \
   --apikey-labels team=gloo
   ```

2. Verify that the secret was successfully created and contains an API key.
   ```sh
   kubectl get secret my-apikey -n gloo-system -o yaml
   ```

3. Create an AuthConfig CR that uses the API key secret.
```sh
kubectl apply -f - <<EOF
apiVersion: enterprise.gloo.solo.io/v1
kind: AuthConfig
metadata:
  name: apikey-auth
  namespace: gloo-system
spec:
  configs:
  - apiKeyAuth:
      headerName: api-key
      labelSelector:
        team: gloo
EOF
```

4. Update the `default` virtual service that you previously created to reference the `apikey-auth` AuthConfig. 
{{< highlight yaml "hl_lines=17-21" >}}
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
      options:
        extauth:
          configRef:
            name: apikey-auth
            namespace: gloo-system
EOF
{{< /highlight >}}

5. Send a request to the GraphQL endpoint. Note that because you enforced API key authorization, the unauthorized request fails.
   ```sh
   curl "$(glooctl proxy url)/graphql" -H 'Content-Type: application/json' -d '{"query": "query {productsForHome {id, title, author, pages, year}}"}'
   ```

6. Add the API key to your request in the `-H 'api-key: $API_KEY'` header, and curl the endpoint again.
   ```sh
   curl "$(glooctl proxy url)/graphql" -H 'Content-Type: application/json' -H 'api-key: $API_KEY' -d '{"query": "query {productsForHome {id, title, author, pages, year}}"}'
   ```
   Example successful response:
   ```json
   {"data":{"productsForHome":[{"id":"0","title":"The Comedy of Errors","author":"William Shakespeare","pages":200,"year":1595}]}}
   ```

## Next steps

Now that you've tried out GraphQL with Gloo Edge, check out the following pages to configure your own services for GraphQL integration.
<!--* [Visualize your GraphQL services in the UI]({{% versioned_link_path fromRoot="/guides/traffic_management/request_processing/graphql/graphql_ui/" %}})-->
* [Explore automatic schema generation with GraphQL service discovery]({{% versioned_link_path fromRoot="/guides/traffic_management/request_processing/graphql/automatic_discovery/" %}})
* [Manually configure resolvers and schema for your GraphQL API]({{% versioned_link_path fromRoot="/guides/traffic_management/request_processing/graphql/resolver_config/" %}})
<!--* [Monitor your GraphQL services]({{% versioned_link_path fromRoot="/guides/traffic_management/request_processing/graphql/observability/" %}})-->
* [Gloo Edge API reference for GraphQL]({{% versioned_link_path fromRoot="/reference/api/github.com/solo-io/gloo/projects/gloo/api/v1/enterprise/options/graphql/v1alpha1/graphql.proto.sk/" %}})