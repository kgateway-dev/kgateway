---
title: Kubernetes Services
weight: 80
description: Routing to services registered as Kubernetes Services through the API
---

If you are running Gloo Edge in a Kubernetes cluster, it is possible to directly specify 
[Kubernetes Services](https://kubernetes.io/docs/concepts/services-networking/service/) as routing destinations. 
The `kube` destination type has two required fields:

* `ref` is a {{< protobuf name="core.solo.io.ResourceRef">}} to the service that should receive traffic
* `port` is an `int` which represents the port on which the service is listening. This must be one of the ports defined in the Kubernetes service spec

The following configuration will forward all requests to `/petstore` to port `8080` on the Kubernetes service named 
`petstore` in the `default` namespace.

{{< highlight yaml "hl_lines=6-10" >}}
routes:
- matchers:
   - prefix: /petstore
  routeAction:
    single:
      kube:
        ref:
          name: petstore
          namespace: default
        port: 8080
{{< /highlight >}}
