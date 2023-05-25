---
title: Migrate discovered gRPC upstreams to 1.14
weight: 10
description: Guide for migrating from the API used for discovered gRPC upstreams in Gloo Edge 1.13 and earlier to the version used in Gloo Edge 1.14
---

Gloo Edge version 1.14.0 introduced significant [changes to the gRPC API]({{% versioned_link_path fromRoot="/guides/traffic_management/destination_types/grpc/about/#api-changes-1-14" %}}) that changes the behavior for how gRPC upstreams are discovered. If you have existing gRPC services that were discovered by a previous version of Gloo Edge, and you want to continue to automatically discover new gRPC services and keep track of any changes to those services, migrate to the new gRPC API. 

## Before you begin

In previous versions of Gloo Edge, you added any HTTP to gRPC mappings to the virtual service. With Gloo Edge 1.14.0, the mappings on the virtual services are no longer discovered automatically. Instead, HTTP mappings must always be provided in the proto itself. 


{{% notice note %}}
Starting in Gloo Edge OSS version 1.14.4 (Gloo Edge Enterprise version 1.14.2), virtual services that still define the `destinationSpec: grpc` section can route to gRPC upstreams that are already migrated to the new gRPC API. 
{{% /notice %}}

{{% notice note %}}
This migration guide assumes that you want to use Gloo Edge to automatically discover the gRPC upstreams. If you do not want to automatically discover the proto descriptors on your gRPC upstreams, you can manually add the proto descriptors. Follow [this guide]({{% versioned_link_path fromRoot="/guides/traffic_management/destination_types/grpc/grpc-transcoding/" %}}) to learn how to generate proto descriptors and add them to the upstream. 
{{% /notice %}}

1. Check if you have gRPC upstreams that must be migrated to the new API. gRPC upstreams that were discovered by a previous Gloo Edge version define the `serviceSpec: grpc` section and configured any HTTP mappings in the `destinationSpec: grpc` section of the corresponding virtual service.
2. Review how [gRPC transcoding]({{% versioned_link_path fromRoot="/guides/traffic_management/destination_types/grpc/about/#grpc-transcoding" %}}) works in Gloo Edge 1.14. 
3. Add the HTTP mappings to your protos. Refer to the [transcoding reference]({{% versioned_link_path fromRoot="/guides/traffic_management/destination_types/grpc/transcoding-reference/" %}}) to learn how to map gRPC functions to HTTP methods. As you update your protos, keep the following things in mind: </br></br>

   * In order for the migration and for Gloo Edge to automatically discover the HTTP mappings, the descriptors that are exposed on your gRPC service must match the routes on your existing virtual services. 
     Using the bookstore example, if `GetShelf` is mapped to `/shelves/{shelf}` with the following `destinationSpec`: 
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
     Then the proto must be defined as follows: 
     ```protobuf
     rpc GetShelf(GetShelfRequest) returns (Shelf) {
         option (google.api.http) = {
           get: "/shelves/{shelf}"
         };
       }
     ```
   * The old API ignored `body:` options in the descriptors and always used a wildcard. To ensure a 1:1 mapping between request bodies when migrating to the new API, your descriptors must also use wildcards for the request body as shown in the `CreateShelf` method in the following Bookstore example. For more information about using a wildcard in the request body, see [Use wildcard in body](https://cloud.google.com/endpoints/docs/grpc/transcoding#use_wildcard_in_body).
     ```protobuf
     // Creates a new shelf in the bookstore.
       rpc CreateShelf(CreateShelfRequest) returns (Shelf) {
         option (google.api.http) = {
           post: "/shelf"
           body: "*"
         };
       }
     ```

## Migrate to the new gRPC API

Migrate your upstreams to the new gRPC API. During the migration, your routes to the gRPC services continue to work. 

1. Review the [upgrade notices for Gloo Edge 1.14]({{% versioned_link_path fromRoot="/operations/upgrading/v1.14/" %}}). 
2. Prepare your Helm chart for the upgrade to 1.14. Make sure to enable the Gloo Edge function discovery (FDS) feature. For more information, see [Configuring the fdsMode Setting]({{% versioned_link_path fromRoot="/installation/advanced_configuration/fds_mode/#configuring-the-fdsmode-setting" %}}).
3. [Upgrade your installation to 1.14]({{% versioned_link_path fromRoot="/operations/upgrading/v1.14/#upgrade" %}}). Make sure to apply any new CRDs. 
4. Delete the existing gRPC upstreams that were discovered by a previous version of Gloo Edge. 
   ```sh
   kubectl delete upstream <upstream_name> -n gloo-system
   ```
5. Wait a few seconds. Then, verify that the gRPC upstreams were automatically discovered by Gloo Edge again. 
   ```sh
   kubectl get upstreams -n gloo-system
   ```
6. Update the corresponding virtual services and remove the `destinationSpec: grpc` section. Note that with Gloo Edge OSS version 1.14.4 (Gloo Edge Enterprise version 1.14.2), virtual services that keep the `destinationSpec: grpc` section can still route to gRPC upstreams that were migrated to the new gRPC API. 


