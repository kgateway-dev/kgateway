---
title: "Optional: Secure the GraphQL API"
weight: 50
description: Protect the GraphQL API that you created in the previous sections by using an API key.
---

Protect the GraphQL API that you created in the previous sections by using an API key. Note that you can also use any other authorization mechanism provided by Gloo Edge to secure your GraphQL endpoint.

1. Create an API key secret that contains an existing API key. If you want `glooctl` to create an API key for you, you can specify the `--apikey-generate` flag instead of the `--apikey` flag.
   ```sh
   glooctl create secret apikey my-apikey \
   --apikey $API_KEY \
   --apikey-labels team=gloo
   ```

2. Verify that the secret was successfully created and contains an API key. If you had Gloo Edge generate the API key, set the value as an environment variable, `export API_KEY=<api-key-value>`.
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

1. Send a request to the GraphQL endpoint. Note that because you enforced API key authorization, the unauthorized request fails, and you get a `401 Unauthorized` response.
   ```sh
   curl "$(glooctl proxy url)/graphql" -H 'Content-Type: application/json' -d '{"query": "query {productsForHome {id, title, author, pages, year}}"}' -v
   ```

2. Add the API key to your request in the `-H 'api-key: $API_KEY'` header, and curl the endpoint again.
   ```sh
   curl "$(glooctl proxy url)/graphql" -H 'Content-Type: application/json' -H 'api-key: $API_KEY' -d '{"query": "query {productsForHome {id, title, author, pages, year}}"}'
   ```
   Example successful response:
   ```json
   {"data":{"productsForHome":[{"id":"0","title":"The Comedy of Errors","author":"William Shakespeare","pages":200,"year":1595}]}}
   ```

## Next steps

Now that you've tried out GraphQL with Gloo Edge, check out the following pages to configure your own services for GraphQL integration.
* [Visualize your GraphQL services in the UI]({{% versioned_link_path fromRoot="/guides/graphql/graphql_ui/" %}})
* [Explore automatic schema generation with GraphQL service discovery]({{% versioned_link_path fromRoot="/guides/graphql/automatic_discovery/" %}})
* [Manually configure resolvers and schema for your GraphQL API]({{% versioned_link_path fromRoot="/guides/graphql/resolver_config/" %}})
* [Stitch GraphQL schemas]({{% versioned_link_path fromRoot="/guides/graphql/stitching/" %}})
* [Monitor your GraphQL services]({{% versioned_link_path fromRoot="/guides/graphql/observability/" %}})
* [Gloo Edge API reference for GraphQL]({{% versioned_link_path fromRoot="/reference/api/github.com/solo-io/gloo/projects/gloo/api/v1/enterprise/options/graphql/v1beta1/graphql.proto.sk/" %}})