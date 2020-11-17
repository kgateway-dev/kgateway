---
title: Consul Services
weight: 90
description: Routing to services registered in Consul service-discovery
---

Gloo Edge's service discovery leverages existing registry or catalog implementations. A widely deployed service for registering and discovery services is [HashiCorp Consul](https://www.hashicorp.com/products/consul/). If your services already register into Consul, we can use Gloo Edge to read the service catalog from Consul and discover these services automatically.

When services registered into Consul require a bit more fine-grained grouping and routing, Gloo Edge can do that too.

## Configuring Gloo Edge to Discover from Consul

Gloo Edge can automatically discover [Upstreams]({{% versioned_link_path fromRoot="/introduction/architecture/concepts#upstreams" %}}) from Consul. You can also explicitly create upstreams from Consul. 

To enable automatic discovery of Consul services, update your {{< protobuf name="gloo.solo.io.Settings" >}} resource and add the `consul` section:


```shell
kubectl patch settings -n gloo-system default \
    --patch '{"spec": {"consul": {"address": "gloo-consul-server.default:8500", "serviceDiscovery": {}}}}' --type=merge
```

{{< highlight yaml "hl_lines=9-11" >}}
apiVersion: gloo.solo.io/v1
kind: Settings
metadata:
  labels:
    app: gloo
  name: default
  namespace: gloo-system
spec:
  consul:
    address: gloo-consul-server.default:8500
    serviceDiscovery: {}
  discovery:
    fdsMode: WHITELIST

    ...
    ...
{{< / highlight >}}

If you have Discovery enabled, Gloo Edge will automatically discover upstreams from the Consul cluster. After enabling this setting, go check your upstreams:
{{< tabs >}}
{{< tab name="kubectl" codelang="yaml">}}
kubectl get upstream -n gloo-system
{{< /tab >}}
{{< tab name="glooctl" codelang="shell">}}
glooctl get upstreams
{{< /tab >}}
{{< /tabs >}}


### Explicitly creating consul upstreams

Even if you opt not to automatically discovery upstreams from Consul (ie, you disable Discovery), you can create them explicitly. You still need the Consul server settings in the `settings` configuration for Gloo Edge.


{{< tabs >}}
{{< tab name="kubectl" codelang="yaml">}}
apiVersion: gloo.solo.io/v1
kind: Upstream
metadata:
  name: jsonplaceholder
  namespace: gloo-system
spec:
  consul:
    serviceName: jsonplaceholder
    serviceTags:
    - gloo
    - jsonplaceholder
{{< /tab >}}
{{< tab name="glooctl" codelang="shell">}}
glooctl create upstream consul --name jsonplaceholder \
--consul-service jsonplaceholder --consul-service-tags gloo,jsonplaceholder
{{< /tab >}}
{{< /tabs >}}

## Routing to Consul upstreams

A single Consul service usually maps to several service instances, which can have distinct sets of tags, listen on different ports, and live in multiple data centers. To give a concrete example, here is a simplified response you might 
get when querying Consul for a service with a given name:

```json
[
  {
    "ServiceID": "32a2a47f7992:nodea:5000",
    "ServiceName": "my-db",
    "Address": "192.168.1.1",
    "Datacenter": "dc1",
    "ServicePort": 5000,
    "ServiceTags": [
      "primary"
    ]
  },
  {
    "ServiceID": "42a2a47f7992:nodeb:5001",
    "ServiceName": "my-db",
    "Address": "192.168.1.2",
    "Datacenter": "dc1",
    "ServicePort": 5001,
    "ServiceTags": [
      "secondary"
    ]
  },
  {
    "ServiceID": "52a2a47f7992:nodec:6000",
    "ServiceName": "my-db",
    "Address": "192.168.2.1",
    "Datacenter": "dc2",
    "ServicePort": 6000,
    "ServiceTags": [
      "secondary"
    ]
  }
]
```

If the ports and data centers for all of the endpoints for a Consul service are the same, and you don't need to slice and dice them up into finer-grained subsets, you can just use [Upstreams]({{% versioned_link_path fromRoot="/introduction/architecture/concepts#upstreams" %}}) like you do with any other service to which to route. 

Example:

{{< highlight yaml "hl_lines=15-17" >}}
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
      - prefix: /todos      
      routeAction:        
        single:
          upstream:
            name: jsonplaceholder
            namespace: gloo-system
{{< /highlight >}}

Also, with using Upstreams instead of the consul-specific config, you can also leverage the fact that Gloo Edge does [function discovery]({{% versioned_link_path fromRoot="/introduction/architecture/concepts/#functions" %}}) (ie, REST or gRPC based on swagger or reflection respectively). 

### Subset based routing


If you have subsets within the Consul registry for a particular service, you can target them very specifically with the {{<
protobuf
name="gloo.solo.io.ConsulServiceDestination"
display="consul destination type"
>}} settings. This allows you to target a subset of these service instances via the optional `tags` and `dataCenters` fields. Gloo Edge will detect the correspondent IP addresses and ports and load balance traffic between them. 


If the ports and data centers for all of the endpoints for a Consul service are the same, and you don't need to slice and dice them up into finer-grained subsets, you can just use [Upstreams]({{% versioned_link_path fromRoot="/introduction/architecture/concepts/#upstreams" %}}) like you do with any other service to which to route. Also, with using Upstreams instead of the consul-specific config, you can also leverage the fact that Gloo Edge does [function discovery]({{% versioned_link_path fromRoot="/introduction/architecture/concepts/#functions" %}}) (ie, REST or gRPC based on swagger or reflection respectively).

{{% notice note %}}
When providing the `tags` option, Gloo Edge will only match service instances that **exactly** match the given tag set.
{{% /notice %}}

For example, the following configuration will forward all matching requests to the second and third service instances,

{{< highlight yaml "hl_lines=6-9" >}}
routes:
- matchers:
   - prefix: /db
  routeAction:
    single:
      consul:
        serviceName: my-db
        tags:
        - secondary
{{< /highlight >}}

while this next example will forward the same requests only to the first two instances (the ones in data center `dc1`)

{{< highlight yaml "hl_lines=6-9" >}}
routes:
- matchers:
   - prefix: /db
  routeAction:
    single:
      consul:
        serviceName: my-db
        dataCenters:
        - dc1
{{< /highlight >}}

Finally, not specifying any optional filter fields will cause requests to be forwarded to all three service instances:

{{< highlight yaml "hl_lines=6-9" >}}
routes:
- matchers:
   - prefix: /db
  routeAction:
    single:
      consul:
        serviceName: my-db
{{< /highlight >}}

{{% notice note %}}
As is the case with [`Subsets`]({{% versioned_link_path fromRoot="/guides/traffic_management/destination_types/subsets/" %}}), Gloo Edge will fall back to forwarding the request to all available service 
instances if the given criteria do not match any subset of instances.
{{% /notice %}}
