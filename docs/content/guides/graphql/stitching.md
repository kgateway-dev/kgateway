<!-- Hiding this content for now until it is fully developed and ready for release.

---
title: Schema stitching
weight: 50
description: Use Gloo Edge to stitch together schemas for multiple GraphQL services.
---

When you use GraphQL in Gloo Edge, you can stitch multiple schemas together to expose one unified GraphQL server to your clients.

For example, consider a cluster to which the `user` and `product` services are deployed. These services are either native GraphQL servers, or have been converted to GraphQL via automatic schema discovery. However, both of these services contribute to a unified data model, which clients must typically stitch together in the frontend. With Gloo Edge, you can instead stitch the GraphQL schemas for these services together in the backend, and expose a unified GraphQL server to your clients. This frees your clients to consider only what data that they want to fetch, not how to fetch the data.

To understand how stitching occurs, consider both of the services, starting with the user service. The user service provides a partial type definition for the `User` type, and a query for how to get the full name of a user given the username.
```yaml
type User {
  username: String!
  fullName: String
}

type Query {
  getUserWithFullName(username: String!): User
}
```

Example query to the user service:
```yaml
query {
  getUserWithFullName(username: "akeith") {
    fullName
  }
}
```

Example response from the user service:
```json
{
  "getUserWithFullName": "Abigail Keith"
}
```

The product service also provides a partial type definition for the `User` type, and a query for how to get the product name and the seller's username given the product ID.
```yaml
type User {
  username: String!
}


type Product{
  id: ID!
  name: String!
  seller: User!
}

type Query {
  getProductById(id: ID!): Product!
}
```

Example query to the product service:
```yaml
query {
  getProductById(id: 125) {
    name
    seller {
      username
    }
  }
}
```

Example response from the product service:
```json
{
  "getProductById": {
    "name": "Narnia",
    "seller": {
      "username": "akeith"
    }
  }
}
```

But consider a client that wants the full name of the seller for a given product, instead the username of the seller. Given the product ID, the client cannot get the seller's full name from the product service. However, the full name of any user _is_ provided by the user service. 

To solve this problem, you can specify a configuration file to merge the types between the services. In the `merge_config` section for a `user-service` configuration file, you can specify which fields are unique to the `User` type, and how to get these fields. If a client provides the username for a user and wants the full name, Gloo Edge can use the `getUserWithFullName` query to provide the full name from the user service.
**QUESTION is the user providing this merging config somewhere? Sounds like in **
```yaml
name: user-service
namespace: products-app
merge_config:
  User:
    query_field: getUserWithFullName
    key: username
```

Similarly, in the `merge_config` section for a `product-service` configuration file, you can specify which fields are unique to the `User` type, and how to get these fields. If a client provides the product ID and wants the product name, Gloo Edge can use the `getProductByID` query to provide the product ID from the product service.
```yaml
name: product-service
namespace: products-app
mergeConfig:
  Product:
    queryName: getProductById
    key: id
```

As a result, Gloo Edge generates a **stitched service**. From this one stitched service, a client can provide the product ID, and recieve the product name, the full name of the seller, and the username of the seller.
```yaml
type User {
  username: String!
  fullName: String
}


type Product{
  id: ID!
  name: String!
  seller: User!
}

type Query {
  getProductById(id: ID!): Product!
}
```

Based on this stitched service information, the following schema definition is generated, which incorporates all the types and queries from each of the respective services. In the background, Gloo Edge uses this schema to create the requests to the stitched service, and then stitches the responses back together into one response to the client.
```yaml
schema_definition: |
  type Query {
    getUserWithFullName(username: String!): User
    getProductById(productId: ID!): Product!
  }

  type User {
    username: String!
    fullName: String
  }

  type Product {
    id: ID!
    name: String!
    seller: User!
  }
```

Example query to the stitched service:
```yaml
query {
  getProductById(id: 125) {
    name
    seller {
      username
      fullName
    }
  }
}
```

Example response from the stitched service:
```json
{
  "getProductById": {
    "name": "Narnia",
    "seller": {
      "username": "akeith"
      "fullName": "Abigail Keith"
    }
  }
}
```

-->