---
title: About gRPC transcoding
weight: 10
description: 
---

gRPC transcoding is a feature that you can use to map a gRPC method to one or more HTTP REST endpoints. With gRPC transcoding, you can build one service that supports both gRPC and REST API requests and responses. 

## Add gRPC to REST mappings to proto files

To map gRPC to REST methods, you add an `HttpRule` to your proto file. An `HttpRule` describes how a component of a gRPC request is mapped to an HTTP URL path, URL query parameter, or request body. The `HttpRule` also describes how gRPC response messages are mapped to the HTTP response body. 

To define an `HttpRule` in your proto file, you add the `google.api.http` annotation to your gRPC method as shown in the followimg example. 

{{< highlight yaml "hl_lines=3-4" >}}
     service Messaging {
       rpc GetMessage(GetMessageRequest) returns (Message) {
         option (google.api.http) = {
             get: "/v1/{name=messages/*}"
         };
       }
     }
     message GetMessageRequest {
       string name = 1; // Mapped to URL path.
     }
     message Message {
       string text = 1; // The resource content.
     }
{{< /highlight >}}
 
 With this example, you can achieve the following REST to gRPC mapping: 
 
| HTTP | gRPC |
| -----|-----|
|`GET /v1/messages/123456`  | `GetMessage(name: "messages/123456")`|
 
 
For more information about how to set up the REST to gRPC mapping, see [](). 
 
## Discover gRPC to REST mappings

To configure your Gloo Edge proxy to accept incoming REST requests for your gRPC app and to correctly translate the REST request into a gRPC request, you must provide a proto descriptor that contains all your gRPC to REST transcoding rules. Proto descriptors are generated based on the `google.api.http` annotations that you added to your gRPC methods by using the `protoc` tool. 

To configure your Gloo Edge proxy to translate incoming REST requests to gRPC, the proxy requires the proto descriptor binary to be present on the gRPC upstream. You have the following options to add proto descriptors to your upstream: 

- **Automatic discovery with FDS**: You can enable the Gloo Edge Function Discovery Service (FDS) to automatically discover proto descriptors in your gRPC service and to add them to the upstream. 
- **Manual**: If you do not or cannot enable Gloo Edge FDS, you can manually generate proto descriptors, encode them, and add them to the upstream. Note that proto descriptors are overwritten when you enable FDS. 

With the proto descriptor binary, Gloo Edge can accept incoming REST requests, map fields to the gRPC request by using the gRPC to REST mappings in the proto descriptors, and forward the request as a gRPC request to your upstream service.  

