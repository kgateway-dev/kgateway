---
title: Schema stitching
weight: 50
description: Use Gloo Edge to stitch together schemas for multiple GraphQL services.
---

When you use GraphQL in Gloo Edge, you can stitch multiple schemas together to expose one unified GraphQL server to your clients.

Consider a cluster that has two existing GraphQL APIs, `user-svc` and `product-svc`. Each service has similar information that you might want to provide as part of a unified data model. Typically, clients must stitch together the services in the frontend. With Gloo Edge, you can instead stitch the GraphQL schemas for these services together in the backend, and expose a unified GraphQL server to your clients. This stitching frees your clients to consider only what data that they want to fetch, not how to fetch the data.

Follow along with the user and product service example.

## Reviewing each service's configuration

To understand how stitching occurs, consider the data model and example queries for both services, starting with the user service. 

**User service**: The `GraphQLApi` resource for the user service defines a partial type definition for the `User` type, and a query for how to get the full name of a user given the username.

{{< tabs >}}
{{< tab name="User type definition" codelang="yaml" >}}
apiVersion: graphql.gloo.solo.io/v1beta1
kind: GraphQLApi
metadata:
  name: user-graphql
  namespace: product-app
spec:
  executableSchema:
    executor:
      local:
        enableIntrospection: true
        resolutions:
          Query|UserService.GetUser:
            grpcResolver:
              requestTransform:
                methodName: GetUser
                outgoingMessageJson:
                  username: '{$args.username}'
                serviceName: user.UserService
              upstreamRef:
                name: user-svc
                namespace: product-app
    schemaDefinition: |
      type User {
        username: String
        fullName: String
        userId: Int
      }

      type Query {
        GetUser(username: String): User @resolve(name: "Query|UserService.GetUser")
      }
{{< /tab >}}
{{< tab name="Query" codelang="yaml">}}
query {
  GetUser(username: "akeith") {
      fullName
      userId
  }
}
{{< /tab >}}
{{< tab name="Response" codelang="json">}}
{
  "GetUser": {
      "fullName": "Abigail Keith"
      "userId": 346
  }
}
{{< /tab >}}
{{< /tabs >}}

**Product service**: The `GraphQLApi` resource for the product service also defines a partial type definition for the `User` type, and a query for how to get the product name and the seller's username given the product ID.

{{< tabs >}}
{{< tab name="Product type definition" codelang="yaml" >}}
apiVersion: graphql.gloo.solo.io/v1beta1
kind: GraphQLApi
metadata:
  name: product-graphql
  namespace: product-app
spec:
  executableSchema:
    executor:
      local:
        enableIntrospection: true
        resolutions:
          Query|ProductService.GetProduct:
            grpcResolver:
              requestTransform:
                methodName: GetProduct
                outgoingMessageJson:
                  id: '{$args.id}'
                  name: '{$args.name}'
                serviceName: product.ProductService
              upstreamRef:
                name: product-svc
                namespace: product-app
    schemaDefinition: |
      type User {
        username: String
      }

      type Product{
        id: Int
        name: String
        seller: User
      }

      type Query {
        GetProduct(id: Int): Product @resolve(name: "Query|ProductService.GetProduct")
      }
{{< /tab >}}
{{< tab name="Query" codelang="yaml">}}
query {
  GetProduct(id: 125) {
    name
    seller {
      userId
    }
  }
}
{{< /tab >}}
{{< tab name="Response" codelang="json">}}
{
  "GetProduct": {
    "name": "Narnia",
    "seller": {
      "userId": 346
    }
  }
}
{{< /tab >}}
{{< /tabs >}}

What if a client wants the full name of the seller for a product, instead of the username? Given the product ID, the client cannot get the seller's full name from the product service. However, the full name of any user _is_ provided by the user service. 

## Stitching together the services

When you have different services with data that you want clients to be able to request, you can stitch the services together. In a separate `GraphQLApi` resource, specify a `stitchedSchema` section that indicates how to merge the types between the services. 

In the user service subschema, you can specify which fields are unique to the `User` type, and how to get these fields. For example, in the following `typeMerge`, Gloo Edge can use the `GetUser` query to provide the full name from the user service.

```yaml
apiVersion: graphql.gloo.solo.io/v1beta1
kind: GraphQLApi
metadata:
  name: stitched-graphql
  namespace: product-app
spec:
  stitchedSchema:
    subschemas:
    - name: user-svc
      namespace: product-app
      typeMerge:
        User:
          selectionSet: '{ username }'
          queryName: GetUser
          args:
            username: username
    - name: product-svc
      namespace: product-app
```

As a result, Gloo Edge generates a **stitched service**. From this one stitched service, a client can provide the product ID, and recieve the product name, the username of the seller, the user ID of the seller, _and_ the full name of the seller.
```yaml
type User {
  username: String
  fullName: String
  userId: Int
}

type Product{
  id: Int
  name: String
  seller: User
}

type Query {
  GetUser(username: String): User @resolve(name: "Query|UserService.GetUser")
  GetProduct(id: Int): Product @resolve(name: "Query|ProductService.GetProduct")
}
```

## Querying the stitched service

Based on the stitched service, Gloo Edge generates the following schema definition, which incorporates all the types and queries from each of the respective services.

Clients can query the stitched service. In the background, Gloo Edge uses this schema to create the requests to the stitched service, and then stitches the responses back together into one response to the client.

{{< tabs >}}
{{< tab name="Query" codelang="yaml">}}
query {
  GetProduct(id: 125) {
    name
    seller {
      username
      fullName
      userId
    }
  }
}
{{< /tab >}}
{{< tab name="Response" codelang="json">}}
{
  "GetProduct": {
    "name": "Narnia",
    "seller": {
      "username": "akeith"
      "fullName": "Abigail Keith"
      "userId": 346
    }
  }
}
{{< /tab >}}
{{< /tabs >}}

## Next steps

For more information, check out the API reference documentation for [`stitchedSchema`]({{% versioned_link_path fromRoot="/reference/api/github.com/solo-io/gloo/projects/gloo/api/v1/enterprise/options/graphql/v1beta1/graphql.proto.sk/#stitchedschema" %}}).