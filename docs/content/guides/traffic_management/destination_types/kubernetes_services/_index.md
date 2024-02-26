---
title: Kubernetes Services
weight: 80
description: Routing to services registered as Kubernetes Services through the API
---

To allow for optimal performance in Gloo Edge, it is recommended to use Gloo [static]({{% versioned_link_path fromRoot="/guides/traffic_management/destination_types/static_upstream/" %}}) and [discovered]({{% versioned_link_path fromRoot="/guides/traffic_management/destination_types/discovered_upstream/" %}}) Upstreams as your routing destination. However, if you run Gloo Edge in a Kubernetes cluster, you have the option to route to a Kubernetes service directly. 

Consider the following information before choosing a Kubernetes service as your routing destination: 
- For Gloo Edge to route traffic to a Kubernetes service directly, Gloo Edge scans all services in the cluster and creates in-memory Upstream resources. If you have a large number of services in your cluster, the API snapshot increases which can have a negative impact on the Gloo Edge translation time.
- When using Kubernetes services, load balancing is done in `kube-proxy` which can have further performance impacts. Routing to Gloo Upstreams bypasses `kube-proxy` as the request is routed to the pod directly. 
- Some Gloo Edge functionality, such as policies, might not be available when using Kubernetes services. 

To use Kubernetes services as a routing destination: 

1. Enable routing to Kubernetes services by setting `settings.disableKubernetesDestinations: true` in your Gloo Edge Helm chart. By default, routing to Kubernetes services is disabled in Gloo Edge due to the negative performance impact on translation and load balancing time in clusters with a lot of Kubernetes services. 
2. Configure the Kubernetes service as a routing destination in your VirtualService. The following example configuration forwards all requests to `/petstore` to port `8080` on the `petstore` Kubernetes service in the `default` namespace.

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
  
   The `kube` destination type has two required fields:

   * `ref` is a {{< protobuf name="core.solo.io.ResourceRef">}} to the service that receives the traffic. 
   * `port` is an integer (`int`) and represents the port the service listens on. Note that this port must be defined in the Kubernetes service.
