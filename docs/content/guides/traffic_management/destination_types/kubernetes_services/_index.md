---
title: Kubernetes Services
weight: 80
description: Routing to services registered as Kubernetes Services through the API
---

To allow for optimal performance in Gloo Edge, it is recommended to use Gloo [static]({{% versioned_link_path fromRoot="/guides/traffic_management/destination_types/static_upstream/" %}}) and [discovered]({{% versioned_link_path fromRoot="/guides/traffic_management/destination_types/discovered_upstream/" %}}) Upstreams as your routing destination. However, if you run Gloo Edge in a Kubernetes cluster, you can choose between the following options to route to a Kubernetes service: 

## Option 1: Route to a Kubernetes service directly

You can configure your VirtualService to route to a Kubernetes service instead of a Gloo Upstream. 

{{% notice note %}}
Consider the following information before choosing a Kubernetes service as your routing destination: 
- For Gloo Edge to route traffic to a Kubernetes service directly, Gloo Edge requires scanning of all services in the cluster to create in-memory Upstream resources to represent them. Gloo uses these resources to validate that the upstream destination is valid and returns an error if the specified Kubernetes service cannot be found. Note that the in-memory Upstream resources are included in the API snapshot. If you have a large number of services in your cluster, the API snapshot increases which can have a negative impact on the Gloo Edge translation time.
- When using Kubernetes services as a routing destination, Gloo Edge relies on `kube-proxy` to perform load balancing which can have further performance impacts. Routing to Gloo Upstreams bypasses `kube-proxy` as the request is routed to the pod directly. 
- Some Gloo Edge functionality, such as policies, might not be available when using Kubernetes services as a routing destination. 
{{% /notice %}}

To use Kubernetes services as a routing destination: 

1. Get the default Gloo Edge settings and verify that `spec.gloo.disableKubernetesDestinations` is set to `false`. This setting is required to allow Gloo Edge to scan all Kubernetes services in the cluster and to create in-memory Upstream resources to represent them. If it is set to `true`, follow the [upgrade guide]({{% versioned_link_path fromRoot="/operations/upgrading/" %}}) and set `settings.disableKubernetesDestinations: false` in your Helm chart. 
   ```sh
   kubectl get settings default -n gloo-system -o yaml
   ```
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
   

## Option 2: Use Kubernetes Upstream resources

Instead of routing to a Kubernetes service directly, you can create [Gloo Kubernetes Upstream]({{% versioned_link_path fromRoot="/reference/api/github.com/solo-io/gloo/projects/gloo/api/v1/options/kubernetes/kubernetes.proto.sk/" %}}) resources that represent your Kubernetes workload. With Kubernetes Upstream resources, you can route requests to a specific pod in the cluster. This process bypasses `kube-proxy` which improves load balancing times for your workloads. 

To use Kubernetes Upstream resources: 

1. Create a Kubernetes Upstream resource for your workload. The following configuration creates an upstream resource for the Petstore app that listens on port 8080 in the default namespace. 
   ```yaml
   apiVersion: gloo.solo.io/v1
   kind: Upstream
   metadata:
     name: petstore
     namespace: gloo-system
   spec:
     kube:
       serviceName: petstore
       serviceNamespace: default
       servicePort: 8080
   ```
   
2. Configure the Kubernetes Upstream as a routing destination in your VirtualService. The following example configuration forwards all requests to `/petstore` to the Petstore upstream in the `gloo-system` namespace.

   {{< highlight yaml "hl_lines=6-8" >}}
routes:
- matchers:
   - prefix: /petstore
  routeAction:
    single:
      upstream:
        name: petstore
        namespace: gloo-system
   {{< /highlight >}}
