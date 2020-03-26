---
title: Upstream Groups
weight: 40
description: Upstreams and weights are stored in a separate Upstream Group Custom Resource
---

An {{< protobuf name="gloo.solo.io.UpstreamGroup" display="UpstreamGroup">}} addresses
an issue of how do you have multiple routes or virtual services referencing the same multiple weighted destinations where
you want to change the weighting consistently for all calling routes. This is a common need for Canary deployments
where you want all calling routes to forward traffic consistently across the two service versions.

For example, if I'm doing a Canary deployment of a new shopping cart service, I may want my inventory and ordering services
to call the same weighted destinations consistently, AND I want the ability to update the destination weights, e.g. go
from 90% v3 and 10% v4 => 50% v3 and 50% v4 **without** needing to know what routes are referencing my upstream destinations.

![Upstream Group example]({{% versioned_link_path fromRoot="/img/inv2.png" %}})

There are two steps to using an upstream group. First, you need to create an Upstream Group custom resource, and then you
need to reference that Upstream Group from your one or more route actions. Let's build on our [Multiple Destination]({{% versioned_link_path fromRoot="/guides/traffic_management/destination_types/multi_destination/" %}})
example.

#### Create Upstream Group

{{< highlight yaml >}}
apiVersion: gloo.solo.io/v1
kind: UpstreamGroup
metadata:
  name: my-service-group
  namespace: gloo-system
spec:
  destinations:
  - destination:
      upstream:
        name: default-myservice-v1-8080
        namespace: gloo-system
    weight: 9
  - destination:
      upstream:
        name: default-myservice-v2-8080
        namespace: gloo-system
    weight: 1
{{< /highlight >}}

#### Reference Upstream Group in your Route Actions

{{< highlight yaml "hl_lines=5-7 11-15" >}}
routes:
- matchers:
   - prefix: /myservice
  routeAction:
    upstreamGroup:
      name: my-service-group
      namespace: gloo-system
- matchers:
   - prefix: /some/other/path
  routeAction:
    upstreamGroup:
      name: my-service-group
      namespace: gloo-system
{{< /highlight >}}

Once deployed, you can update the weights in your shared Upstream Group and those changes will be picked up by all routes
that referencing that upstream group instance.
