---
title: Migrate discovered gRPC upstreams to 1.14 
weight: 10
description: Guide for migrating from the API used for discovered gRPC upstreams in Gloo Edge 1.13 and earlier to the version used in Gloo Edge 1.14
---

Gloo Edge 1.14 introduced significant changes to the API used for discovered gRPC upstreams. If you have existing gRPC services
that were discovered by Gloo and want to continue discovering new services/changes to those services, you should follow this guide to migrate to the new API.

## Before you begin
The steps in this guide are only applicable if you have upstreams with `serviceSpec: grpc` and virtual services with `destinationSpec: grpc`

## Steps 
1. Upgrade your Gloo Edge installation, being sure to apply the new CRDs.
2. Ensure that FDS is enabled.
3. Delete the existing discovered gRPC upstreams and wait for them to be rediscovered.
4. Update your virtual services to use the new api (link here)

At every step in this process, your routes to gRPC services will continue to work.

## Things to keep in mind

* In order for the migration to work with discovery, the descriptors exposed on your gRPC service should match the routes on your existing virtual services. 
Using the bookstore example, if `GetShelf` is mapped to `/shelves/{shelf}` with the following `destinationSpec`
```yaml
routeAction:
  single:
    destinationSpec:
        grpc:
          function: GetShelf
          package: main
          service: Bookstore
          parameters:
            path: /shelves/{shelf}
```
then the protos should be:
```protobuf
rpc GetShelf(GetShelfRequest) returns (Shelf) {
    option (google.api.http) = {
      get: "/shelves/{shelf}"
    };
  }
```
* The old API ignored `body:` options in the descriptors and always used a wildcard. To ensure there is a 1:1 mapping between request bodies when migrating to the new API, your descriptors should also use wildcards for the request body.
  In the Bookstore example, the `CreateShelf` methods should be defined as follows:
```protobuf
// Creates a new shelf in the bookstore.
  rpc CreateShelf(CreateShelfRequest) returns (Shelf) {
    option (google.api.http) = {
      post: "/shelf"
      body: "*"
    };
  }
```
See: https://cloud.google.com/endpoints/docs/grpc/transcoding#use_wildcard_in_body for an explanation about the difference in the requests with and without a wildcard for the request body.