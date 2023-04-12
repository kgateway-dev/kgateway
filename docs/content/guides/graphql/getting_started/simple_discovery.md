---
title: Simple service discovery
weight: 20
description: Explore basic GraphQL service discovery with the Pet Store sample application.
---

Explore basic GraphQL service discovery with the Pet Store sample application.

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

2. Optional: Check the unfiltered JSON output for the Pet Store service.
   1. Create a route for the service.
      ```sh
      glooctl add route --name default --namespace gloo-system --path-prefix / --dest-name default-petstore-8080 --dest-namespace gloo-system
      ```

   2. Send a `/GET` request to `/v3/pet/10` of this service.
      ```sh
      curl "$(glooctl proxy url)/v3/pet/10" -H 'Accept: application/json'
      ```
      Example unfiltered JSON output:
      ```json
      {"id":10,"category":{"id":3,"name":"Rabbits"},"name":"Rabbit 1","photoUrls":["url1","url2"],"tags":[{"id":1,"name":"tag3"},{"id":2,"name":"tag4"}],"status":"available"}
      ```

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

1. Send a request to the endpoint to verify that the request is successfully resolved by Envoy. For example, if you want only the name of the pet given the pet's ID:
   ```sh
   curl "$(glooctl proxy url)/graphql" -H 'Content-Type: application/json' -d '{"query": "query {getPetById(petId: 10) {name}}"}' 
   ```
   Example successful response:
   ```json
   {"data":{"getPetById":{"name":"Rabbit 1"}}}
   ```

This JSON output is filtered only for the desired data, as compared to the unfiltered response that the Pet Store app returned to the GraphQL server:
```json
{"id":10,"category":{"id":3,"name":"Rabbits"},"name":"Rabbit 1","photoUrls":["url1","url2"],"tags":[{"id":1,"name":"tag3"},{"id":2,"name":"tag4"}],"status":"available"}
```
Data filtering is one advantage of using GraphQL instead of querying the upstream directly. Because the GraphQL query is issued for only the name of the pets, GraphQL is able to filter out any data in the response that is irrelevant to the query, and return only the data that is specifically requested.

**Up next**: [Explore local GraphQL resolution with the Bookinfo sample application.]({{% versioned_link_path fromRoot="/guides/graphql/getting_started/local_executor" %}})