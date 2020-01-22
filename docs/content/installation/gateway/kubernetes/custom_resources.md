---
title: "Custom Resource Usage"
description: An explanation of Custom Resource usage by Gloo.
weight: 15
---

Gloo uses native constructs in Kubernetes to store data, including ConfigMaps, Secrets, and Custom Resource Definitions (CRDs). This document is meant to summarize what CRDs are created by Gloo during installation, and how these CRDs interact with the other Gloo objects to store configuration data in the cluster.

---

## Overview

When Gloo is installed on Kubernetes, it creates a number of Custom Resource Definitions that Gloo can use to store data. The following table describes each Custom Resource Definition, its grouping, and its purpose.

| Name | Grouping | Purpose |
|------|----------|---------|
| {{< protobuf name="enterprise.gloo.solo.io.AuthConfig" display="AuthConfig">}} | enterprise.gloo.solo.io | User-facing authentication configuration |
| {{< protobuf name="gloo.solo.io.Proxy" display="Proxy">}} | gloo.solo.io | A combination of Gateway resources to be pushed by Gloo to the Envoy proxy. |
| {{< protobuf name="gloo.solo.io.Settings" display="Settings">}} | gloo.solo.io | Global settings for all Gloo components. |
| {{< protobuf name="gloo.solo.io.UpstreamGroup" display="UpstreamGroup">}} | gloo.solo.io | Defining multiple Upstreams or external endpoints for a Virtual Service. |
| {{< protobuf name="gloo.solo.io.Upstream" display="Upstream">}} | gloo.solo.io | Upstreams represent destinations for routing HTTP requests. |
| {{< protobuf name="gateway.solo.io.Gateway" display="Gateway">}} | gateway.solo.io | Describes a single Listener and the routing Upstreams reachable via the Gateway Proxy. |
| {{< protobuf name="gateway.solo.io.RouteTable" display="RouteTable">}} | gateway.solo.io | Child Routing object for the Gloo Gateway. |
| {{< protobuf name="gateway.solo.io.VirtualService" display="VirtualService">}} | gateway.solo.io | Describes the set of routes to match for a set of domains. |

As a quick refresher, Gloo is deployed from several different container images. The Gateway image is responsible for acting as the API gateway and routing traffic to the appropriate Envoy Proxy. The Gloo image is the control plane for Gloo and runs the xDS server used to interact with the Envoy Proxy clusters. The Discovery image performs service discovery of new services added to Kubernetes or another plugin.


The next few sections detail different scenarios where a Custom Resource is used.

---

## Gateway and Proxy Configuration

*Virtual Services*, *Route Tables* and *Gateway* information are all merged together to form a *Proxy* configuration that Gloo can use to prepare a configuration for the Envoy Proxy clusters using the xDS service. 

![Gateway and Proxy Configuration]({{< versioned_link_path fromRoot="/img/gateway-cr.png" >}})

When a user or process wants to perform CRUD (Create, Read, Update, Delete) operations on a Virtual Server, Gateway, or Route Table they may use the `glooctl` command-line tool or `kubectl` directly to make changes. The changes are written to a new or existing Custom Resource matching the resource type that is being altered. The Gateway component of Gloo takes the information from all three Custom Resource types and merges and transforms the data to create a Proxy Custom Resource. The Proxy Custom Resource is discovered by the Gloo component, which uses it to generate the config to be pushed to the Envoy Proxy cluster.

---

## Upstreams and Upstream Groups

*Upstreams* are destinations for traffic sent to the Gloo Gateway. A Virtual Service or Route Table may reference one of more Upstreams as destinations. Multiple Upstreams can be combined into an *Upstream Group* with a list of Upstreams and weights for each Upstream.

### Upstreams

Upstreams can be added manually be a user or process, or they can be added automatically through Service Discovery. In the case of a manual addition, a user or process utilized the `glooctl` command-line tool or `kubectl` directly to perform CRUD operations on an Upstream. The Gloo component is constantly watching the Upstream Custom Resources to see if a change has been made.

![Gateway and Proxy Configuration]({{< versioned_link_path fromRoot="/img/manual-upstream-cr.png" >}})

In the case of automatic addition through discovery, the user or process will deploy a new service to the Kubernetes cluster. The Discovery component will watch for new services being introduced using the Kubernetes plug-in. When the new service is discovered, the Discovery component will create a new Custom Resource including details about the new service. The Gloo component is constantly watching the Upstream Custom Resources to see if a change has been made.

![Gateway and Proxy Configuration]({{< versioned_link_path fromRoot="/img/discovery-cr.png" >}})

### Upstream Groups

Upstream Groups are an abstraction used to group multiple Upstreams together and include weights for load-balancing across the Upstreams. 

![Gateway and Proxy Configuration]({{< versioned_link_path fromRoot="/img/upstream-groups-cr.png" >}})

The Upstream Group Custom Resource is created by a user or process utilizing the `glooctl` command-line tool or `kubectl` directly. The Upstream Group will reference existing Upstream Custom Resources that have already been configured. The Gloo component is constantly watching the Upstream Group Custom Resources to see if a change has been made.

---

## Settings

Gloo keeps global settings stored in a Settings Custom Resource. When a new Gateway, Gloo, or Discovery pod is created, it will look for a Settings Custom Resource to bootstrap its configuration. 

![Gateway and Proxy Configuration]({{< versioned_link_path fromRoot="/img/settings-cr.png" >}})

The Settings Custom Resource is typically created through an installation process using Helm. The values in the CR can be manipulated using the `glooctl` command-line tool or `kubectl` directly. The pods run a periodic sync process that looks for changes to the Settings CR. When a change is detected it is applied after an internal snapshot is taken.

---

## Next Steps

* Learn more about the [`glooctl` command line tool]({{< versioned_link_path fromRoot="/cli/" >}}) used to manipulate these Custom Resources
* Experiment with your own Gloo environment using our [Getting Started guide]({{< versioned_link_path fromRoot="/getting_started/" >}})