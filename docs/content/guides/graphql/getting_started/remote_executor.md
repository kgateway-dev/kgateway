---
title: Remote GraphQL resolution
weight: 40
description: Explore remote GraphQL resolution with an example GraphQL server upstream.
---

Explore remote GraphQL resolution with an example GraphQL server upstream.

In the previous section, you used local execution to resolve GraphQL queries to an upstream service. In this example, you deploy an example GraphQL server upstream, which uses _remote execution_. With remote execution, Envoy proxies the requests to the GraphQL server without executing the request first, and the GraphQL upstream both executes the query and provides the requested data. Additionally, Envoy validates the schema of the GraphQL server.

1. Deploy the following `todos` sample application to the default namespace, which you expose behind a GraphQL server embedded in Envoy.
   ```yaml
   kubectl apply -f - <<EOF
   apiVersion: apps/v1
   kind: Deployment
   metadata:
     labels:
       app: todos
     name: todos
   spec:
     replicas: 1
     selector:
       matchLabels:
         app: todos
     template:
       metadata:
         labels:
           app: todos
       spec:
         containers:
           # If your system uses an arm64 or M1 processor, use `gcr.io/solo-public/graphql-todo:0.0.3-arm64` as the image name instead.
         - image: gcr.io/solo-public/graphql-todo:0.0.3-amd64
           imagePullPolicy: Always
           name: todos
           resources: {}
           terminationMessagePath: /dev/termination-log
           terminationMessagePolicy: File     
   ---
   apiVersion: v1
   kind: Service
   metadata:
     labels:
       app: todos
     name: todos
   spec:
     ports:
     - port: 80
       protocol: TCP
       targetPort: 8080
     selector:
       app: todos
   EOF
   ```

2. After the deployment is ready, verify that Gloo Edge created a `graphqlapi` resource for the upstream.
   ```sh
   kubectl get graphqlapis -n gloo-system
   ```

   Example output:
   ```
   NAME                                AGE
   default-todos-80   2m58s
   ```

3. Optional: Check out the generated GraphQL schema. 
   ```sh
   kubectl get graphqlapis default-todos-80 -o yaml -n gloo-system
   ```

4. Update the `default` virtual service that you previously created to route traffic to `/graphql` to the new `default-todos-80` GraphQL API.
   {{< highlight yaml "hl_lines=12-16" >}}
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
    routes:
    - graphqlApiRef:
        name: default-todos-80
        namespace: gloo-system
      matchers:
      - prefix: /graphql
      options:
        prefixRewrite: "/graphql"
EOF
   {{< /highlight >}}

1. Send a request to the GraphQL endpoint to verify that the request is successfully resolved by the upstream.
   ```sh
   curl -X POST -d '{"query":"{todo(id:\"b\"){id,text,done}}"}' "$(glooctl proxy url)/graphql"
   ```
   In the JSON response, note that only the information you queried is returned:
   ```json
   {"data":{"todo":{"done":false,"id":"b","text":"This is the most important"}}}
   ```

2. To see other example of data filtering, you can optionally send other queries to the GraphQL server, such as the following.
   ```sh
   curl -X POST -d '{"query":"mutation _{updateTodo(id:\"b\",done:true){id,text,done}}", "operationName":"Mutation"}' "$(glooctl proxy url)/graphql"
   curl -X POST -d '{"query":"mutation _{createTodo(text:\"My new todo\"){id,text,done}}", "operationName":"Mutation"}' "$(glooctl proxy url)/graphql"
   curl -X POST -d '{"query":"{todo(id:\"b\"){id,text,done}}"}' "$(glooctl proxy url)/graphql"
   curl -X POST -d '{"query":"{todoList{id,text,done}}"}' "$(glooctl proxy url)/graphql"
   ```

**Up next**: [Protect the GraphQL API that you created in the previous sections by using an API key.]({{% versioned_link_path fromRoot="/guides/graphql/getting_started/secure_api" %}})