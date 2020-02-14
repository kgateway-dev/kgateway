---
title: Consul Services
weight: 40
description: Routing to services that are registered in Consul service-discovery registry
---

Gloo is capable of discovering services registered with [HashiCorp Consul](https://www.hashicorp.com/products/consul/). 
If this feature has been enabled via the `serviceDiscovery` field in the `consul` section of the
{{< protobuf name="gloo.solo.io.Settings" >}} resource, it is possible to specify Consul services as routing destinations.

A single Consul service usually maps to several service instances, which can have distinct sets of tags, listen on 
different ports, and live in multiple data centers. To give a concrete example, here is a simplified response you might 
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

The {{<
protobuf
name="gloo.solo.io.ConsulServiceDestination"
display="consul destination type"
>}}
allows you to target a subset of these service instances via the optional `tags` and `dataCenters` fields. Gloo will 
detect the correspondent IP addresses and ports and load balance traffic between them. 

If the ports and data centers for all of the endpoints for a Consul service are the same, and you don't need to slice and dice them up into finer-grained subsets, you can just use [Upstreams]({{% versioned_link_path fromRoot="/introduction/concepts/#upstreams" %}}) like you do with any other service to which to route. Also, with using Upstreams instead of the consul-specific config, you can also leverage the fact that Gloo does [function discovery]({{% versioned_link_path fromRoot="/introduction/concepts/#functions" %}}) (ie, REST or gRPC based on swagger or reflection respectively).

{{% notice note %}}
When providing the `tags` option, Gloo will only match service instances that **exactly** match the given tag set.
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
As is the case with [`Subsets`]({{% versioned_link_path fromRoot="/gloo_routing/virtual_services/routes/route_destinations/multiple_upstreams/subsets/" %}}), Gloo will fall back to forwarding the request to all available service 
instances if the given criteria do not match any subset of instances.
{{% /notice %}}
