---
title: gRPC to REST
weight: 130
description: Routing gRPC services to a REST API
---

A growing trend is to use gRPC internally as the communication protocol between micro-services. This has quite a few advantages. Some of those are:

1. Client and server stubs are auto generated
1. Efficient binary protocol (Google's protobufs)
1. Cross-language support as client and server libraries are available in many languages
1. HTTP based which plays well with existing firewalls and load balancers
1. Well supported with tooling around observability

While gRPC works great for internal micro-services, it may be desirable to have the internet facing API be a JSON/REST style API. This can happen for many reasons, among which are:

1. Keeping the API backwards compatible
1. Making the API more Web friendly
1. Supporting low-end devices such as IoT where gRPC is not supported.

Gloo Edge allows you to define JSON/REST to your gRPC API so you can have the best of both worlds - outwards facing REST API and an internal gRPC API with no extra code.

With Gloo Edge, there are two ways to leverage gRPC to JSON functionality. In this guide, we explore Gloo Edge's simplified gRPC to JSON transcoding API, where there is no need to annotate your proto definitions with the `google.api.http` options (as described [here](https://cloud.google.com/service-infrastructure/docs/service-management/reference/rpc/google.api#google.api.HttpRule)). A simple gRPC proto will work.

For more advanced use cases where you will want more control over the mapping of your service from gRPC to REST, take a look at the [advanced gRPC to REST guide]({{% versioned_link_path fromRoot="/guides/traffic_management/destination_types/grpc_to_rest_advanced" %}})
 
---

## Overview

In this guide we will deploy a gRPC micro-service and transform its gRPC API to a REST API via Gloo Edge.

Usually, to understand the details of the binary protobuf, a protobuf descriptor is needed. As this micro-service is built with server reflection enabled; together with Gloo Edge's automatic function discovery functionality the required protobuf descriptor will be automatically discovered.

In this guide we are going to:

1. Deploy a gRPC demo service
1. Verify that the gRPC descriptors were indeed discovered
1. Add a Virtual Service creating a REST API that maps to the gRPC API
1. Verify that everything is working as expected

Let's get started!

### Prereqs

Install Gloo Edge with Function Discovery Service (FDS) [blacklist mode]({{< versioned_link_path fromRoot="/installation/advanced_configuration/fds_mode/#configuring-the-fdsmode-setting" >}}) enabled

---

## Deploy the demo gRPC store

Create a deployment and a service:

```shell
kubectl create deployment grpcstore-demo --image=docker.io/soloio/grpcstore-demo
kubectl expose deployment grpcstore-demo --port 80 --target-port=8080
```

### Verify that gRPC functions were discovered
After a few seconds Gloo Edge should have discovered the service with it's proto descriptor:

```shell
kubectl get upstream -n gloo-system default-grpcstore-demo-80 -o yaml
```

You should see output similar to this:

```yaml
apiVersion: gloo.solo.io/v1
kind: Upstream
metadata:
  labels:
    app: grpcstore-demo
    discovered_by: kubernetesplugin
  name: default-grpcstore-demo-80
  namespace: gloo-system
spec:
  discoveryMetadata: {}
  kube:
    selector:
      app: grpcstore-demo
    serviceName: grpcstore-demo
    serviceNamespace: default
    servicePort: 80
    serviceSpec:
      grpc:
        descriptors: Q3F3RkNoVm5iMjluYkdVdllYQnBMMmgwZE…bTkwYnpNPQ==
        grpcServices:
        - functionNames:
          - CreateItem
          - ListItems
          - DeleteItem
          - GetItem
          packageName: solo.examples.v1
          serviceName: StoreService
status:
  reported_by: gloo
  state: 1
```

{{% notice note %}}
The descriptors field above was truncated for brevity.
{{% /notice %}}

As you can see Gloo Edge's function discovery detected the gRPC functions on that service. 

### Create a REST to gRPC translation

Now we are ready to create the external REST to gRPC API. Please run the following command:

```shell
kubectl create -f - <<EOF
apiVersion: gateway.solo.io/v1
kind: VirtualService
metadata:
  name: default
  namespace: gloo-system
spec:
  virtualHost:
    routes:
    - matchers:
       - methods:
         - GET
         prefix: /items/
      routeAction:
       single:
         destinationSpec:
           grpc:
             function: GetItem
             package: solo.examples.v1
             parameters:
               path: /items/{name}
             service: StoreService
         upstream:
           name: default-grpcstore-demo-80
           namespace: gloo-system
    - matchers:
       - methods:
         - DELETE
         prefix: /items/
      routeAction:
       single:
         destinationSpec:
           grpc:
             function: DeleteItem
             package: solo.examples.v1
             parameters:
               path: /items/{name}
             service: StoreService
         upstream:
           name: default-grpcstore-demo-80
           namespace: gloo-system
    - matchers:
       - methods:
         - GET
         exact: /items
      routeAction:
       single:
         destinationSpec:
           grpc:
             function: ListItems
             package: solo.examples.v1
             service: StoreService
         upstream:
           name: default-grpcstore-demo-80
           namespace: gloo-system
    - matchers:
       - methods:
         - POST
         exact: /items
      routeAction:
       single:
         destinationSpec:
           grpc:
             function: CreateItem
             package: solo.examples.v1
             service: StoreService
         upstream:
           name: default-grpcstore-demo-80
           namespace: gloo-system
EOF
```

An explanation for the Virtual Service above:
We have defined four routes. Each route uses a {{< protobuf name="grpc.options.gloo.solo.io.DestinationSpec" display="gRPC destinationSpec" >}} to define REST routes to a gRPC service. When translating a REST API to a gRPC API the JSON body is automatically used to fill in the proto message fields. If you have some parameters in the path or in headers, your can specify them using the {{< protobuf name="transformation.options.gloo.solo.io.Parameters" display="parameters">}}  block in the {{< protobuf name="grpc.options.gloo.solo.io.DestinationSpec" display="gRPC destinationSpec">}} (as done in the route to `GetItem` and `DeleteItem`). We use HTTP method matching to make sure that our API adheres to the REST semantics. Note that the routes for `CreateItem` and `ListItems` are defined for the exact path `/items` (i.e. no trailing slash).

### Test

To test, we can use `curl` to issue queries to our new REST API:

```shell
URL=$(glooctl proxy url)
# Create an item in the store.
curl $URL/items -d '{"item":{"name":"item1"}}'
# List all items in the store. You should see an object with a list containing the item created above. 
curl $URL/items
# Access a specific item. You should see the item as a single object.
curl $URL/items/item1
# Delete the item created.
curl $URL/items/item1 -XDELETE
# No items - this will return an empty object.
curl $URL/items
```

---

## Conclusion

In this guide we have deployed a gRPC micro-service and created an external REST API that translates to the gRPC API via Gloo Edge. This allows you to enjoy the benefits of using gRPC for your microservices while still having a traditional REST API without the need to maintain two sets of code. 

### Next Steps

Learn more about how Gloo Edge handles [gRPC for web clients]({{% versioned_link_path fromRoot="/guides/traffic_management/listener_configuration/grpc_web/" %}}). Gloo Edge can also use a [REST endpoint]({{% versioned_link_path fromRoot="/guides/traffic_management/destination_types/rest_endpoint/" %}}) as an Upstream. Our [function discovery guide]({{% versioned_link_path fromRoot="/installation/advanced_configuration/fds_mode/" %}}) covers how to set up the Function Discovery Service (FDS) for a Swagger document or gRPC service. For advanced gRPC to JSON use cases, check out our [advanced gRPC to REST guide]({{% versioned_link_path fromRoot="/guides/traffic_management/destination_types/grpc_to_rest_advanced" %}}).
