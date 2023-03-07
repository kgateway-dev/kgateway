---
title: About the gRPC API
weight: 10
description: 
---

With Gloo Edge 1.14, a new gRPC API is introduced.

## Summary of changes

Previously, you had to put the REST transcoding mapping into the virtual service directly. Gloo Edge automatically translated these mappings and put them into the proto descriptors. 

With the new API, proto descriptors are not put on the virtual service anymore. Instead, you can use the Gloo Edge FDS feature to automatically discover proto descriptors and add them to the upstream. You can also manually add the proto descriptor to the upstream, but FDS should be enabled in this case as it might overwrite your protos. 

## Discover proto descriptors

The API is based on proto descriptors that are either added to the upstream (recommended) or the gateway. The descriptors include the methods that are available in the gRPC app as well as the REST transcoding so that you can use a REST API to access the gRPC app. 

## Transcoding process

1. Write the protos
2. 
