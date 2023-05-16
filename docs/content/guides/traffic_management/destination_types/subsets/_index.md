---
title: Subsets
weight: 85
description: Routing to subsets of an Upstream
---

## Subset

{{< protobuf name="gloo.solo.io.Subset">}} currently lets you
provide a Kubernetes selector to allow request forwarding to a subset of Kubernetes Pods within the upstream associated
Kubernetes Service. There are currently two steps required to get subsetting to work for Kubernetes upstreams, which are
the only upstream type currently supported. 

**First**, you need to edit the {{< protobuf name="kubernetes.options.gloo.solo.io.UpstreamSpec">}}
of the Kubernetes Upstream that you want to define subsets for by adding a {{< protobuf name="options.gloo.solo.io.SubsetSpec">}}. 
The `subsetSpec` contains a list of `selectors`, each of which consist of a set of `keys`. Each key represents a Kubernetes 
label key. These selectors determine how the subsets for the upstream are to be calculated. For example, the following 
`subsetSpec`:

```yaml
subsetSpec:
  selectors:
  - keys:
    - color
    - size
  - keys:
    - size
```

means that the pods for the upstream will be divided into subsets based both on the values of the `color` and `size` 
labels, and on the value of the `size` label alone. Envoy requires this information to limit the combinations of subsets 
that it needs to compute. The [Envoy documentation](https://github.com/envoyproxy/envoy/blob/main/source/docs/subset_load_balancer.md) 
contains a great explanation of how on subset load balancing works and we strongly recommend that you read it if you plan to use this feature.

**Second**, you need to add a {{< protobuf name="gloo.solo.io.Subset">}}
within the {{< protobuf name="gloo.solo.io.Destination">}}
of the Route Action. This will determine which of the upstream subsets should be selected as destination for this route.

Following is an example of using a label, e.g. `color: blue`, to subset pods handling requests.

These are the Upstream changes that allow you to use the label `color` as a subset selector:

{{< highlight yaml "hl_lines=14-17" >}}
apiVersion: gloo.solo.io/v1
  kind: Upstream
    labels:
      discovered_by: kubernetesplugin
      service: petstore
    name: default-petstore-8080
    namespace: gloo-system
  spec:
    kube:
      selector:
        app: petstore
      serviceName: petstore
      serviceNamespace: default
      subsetSpec:
        selectors:
        - keys:
          - color
      servicePort: 8080
      serviceSpec:
        rest:
...
{{< /highlight >}}

And then you need to configure the subset within the Virtual Service route action, e.g. the following will only forward
requests to a subset of the Petstore Service pods that have a label, `color: blue`.

{{< highlight yaml "hl_lines=21-23" >}}
apiVersion: gateway.solo.io/v1
  kind: VirtualService
  metadata:
    name: default
    namespace: gloo-system
  spec:
    virtualHost:
      domains:
      - '*'
      routes:
      - matchers:
         - prefix: /petstore/findPetById
        routeAction:
          single:
            destinationSpec:
              rest:
                functionName: findPetById
                parameters:
                  headers:
                    :path: /petstore/findPetById/{id}
            subset:
              values:
                color: blue
            upstream:
              name: default-petstore-8080
              namespace: gloo-system
{{< /highlight >}}

{{% notice note %}}
If no pods match the selector, i.e. empty set, then the route action will fall back to forwarding the request to all
pods served by that upstream.
{{% /notice %}}
