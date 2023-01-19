---
title: "Custom Resource Usage"
description: An explanation of Custom Resource usage by Gloo Edge.
weight: 40
---

Gloo Edge uses native constructs in Kubernetes to store data, including ConfigMaps, Secrets, and Custom Resources (CRs). This document is meant to summarize what CRs are created by Gloo Edge during installation, and how these CRs interact with the other Gloo Edge objects to store configuration data in the cluster.

---

## Overview

When Gloo Edge is installed on Kubernetes, it creates a number of Custom Resource Definitions that Gloo Edge can use to store data. The following table describes each Custom Resource Definition, its grouping, and its purpose.

| Name | Grouping | Purpose |
|------|----------|---------|
| {{< protobuf name="gloo.solo.io.Settings" display="Settings">}} | gloo.solo.io | Global settings for all Gloo Edge containers. |
| {{< protobuf name="gateway.solo.io.Gateway" display="Gateway">}} | gateway.solo.io | Describes a single Listener and the routing Upstreams reachable via the Gateway Proxy. |
| {{< protobuf name="gateway.solo.io.VirtualService" display="VirtualService">}} | gateway.solo.io | Describes the set of routes to match for a set of domains with a destination of a Route Table, Upstream, or Upstream Group. |
| {{< protobuf name="gateway.solo.io.RouteTable" display="RouteTable">}} | gateway.solo.io | Child Routing object for the Gloo Edge gateway. |
| {{< protobuf name="gloo.solo.io.Proxy" display="Proxy">}} | gloo.solo.io | A combination of Gateway resources to be parsed by Gloo Edge pods. |
| {{< protobuf name="gloo.solo.io.Upstream" display="Upstream">}} | gloo.solo.io | Upstreams represent destinations for routing requests. |
| {{< protobuf name="gloo.solo.io.UpstreamGroup" display="UpstreamGroup">}} | gloo.solo.io | Group multiple Upstreams and/or external endpoints to be referenced by Virtual Service(s). |
| {{< protobuf name="enterprise.gloo.solo.io.AuthConfig" display="AuthConfig">}} | enterprise.gloo.solo.io | User-facing authentication configuration referenced by Virtual Service(s). |

As a quick refresher, Gloo Edge is deployed as pods from three different container images:

* gloo
* discovery
* gateway-proxy/ingress-proxy

The `gloo` and `discovery` pods act as the control plane for Gloo Edge. The data plane is handled by the `gateway-proxy/ingress-proxy` pods running Envoy.

The `gloo` deployment is responsible for:

* Translating Proxy, Upstreams, UpstreamGroups, Secrets, AuthConfigs, ConfigMaps, Endpoints, Gateway, RouteTables, and Virtual Service Custom Resources into cached Envoy configurations
* Serving cached Envoy configurations via xDS
* Validation webhooks in the Gateway validation server are used to validate a configuration before it is applied
* Gloo Edge validation server is hit by the Gateway validation server to validate Proxy from Gloo Edge's point of view

Discovery is responsible for:

* Discovering Upstreams in the cluster
* Discovering functions with the Function Discovery Service (FDS)

The next few sections detail different scenarios where a Custom Resource is used.

---

## Gateway and Proxy Configuration

*Virtual Services*, *Route Tables* and *Gateway* information are all merged together to form a *Proxy* configuration that the Gloo Edge pods can use to prepare a snapshot for the Envoy Proxy clusters using the *translation engine* and *xDS server* on the Gloo Edge pods. 

![Gateway and Proxy Configuration]({{< versioned_link_path fromRoot="/img/gateway-cr.png" >}})

When a user or process wants to perform CRUD (Create, Read, Update, Delete) operations on a Virtual Server, Gateway, or Route Table they may use the `glooctl` command-line tool or `kubectl` directly to make changes. The changes are written to a new or existing Custom Resource matching the resource type that is being altered. The Gateway functionality in the Gloo Edge pods takes the information from all three Custom Resource types, and merges and transforms the data to create a Proxy Custom Resource. The Proxy Custom Resource is watched by the Gloo Edge pods, which use it to generate the snapshot to be pulled by the Envoy Proxy instances.

---

## Upstreams and Upstream Groups

*Upstreams* are destinations for traffic sent to the Gloo Edge gateway. A Virtual Service or Route Table may reference one of more Upstreams as destinations. Multiple Upstreams can be combined into an *Upstream Group* with a list of Upstreams and weights for each Upstream.

### Upstreams

Upstreams can be added manually by a user or process, or they can be added automatically through Service Discovery. In the case of a manual addition, a user or process utilizes the `glooctl` command-line tool or `kubectl` directly to perform CRUD operations on an Upstream. The Gloo Edge pod is constantly watching the Upstream Custom Resources to see if a change has been made.

![Gateway and Proxy Configuration]({{< versioned_link_path fromRoot="/img/manual-upstream-cr.png" >}})

In the case of automatic addition through discovery, the user or process will deploy a new service to the Kubernetes cluster. The Discovery component will watch for new services being introduced using the Kubernetes integration. When the new service is discovered, the Discovery pod will create a new Custom Resource including details about the new service. The Gloo Edge pod is constantly watching the Upstream Custom Resources to see if a change has been made.

![Gateway and Proxy Configuration]({{< versioned_link_path fromRoot="/img/discovery-cr.png" >}})

### Upstream Groups

Upstream Groups are an abstraction used to group multiple Upstreams together and include weights for load-balancing across the Upstreams. 

![Gateway and Proxy Configuration]({{< versioned_link_path fromRoot="/img/upstream-groups-cr.png" >}})

The Upstream Group Custom Resource is created by a user or process utilizing the `glooctl` command-line tool or `kubectl` directly. The Upstream Group will reference existing Upstream Custom Resources that have already been configured. The Gloo Edge pod is constantly watching the Upstream Group Custom Resources to see if a change has been made.

---

## Settings

Gloo Edge keeps global settings stored in a Settings Custom Resource. When a new Gloo Edge or Discovery pod is created, it looks for a Settings Custom Resource to load its configuration.

The Settings Custom Resource is typically created through an installation process using Helm. The values in the CR can be manipulated using the `glooctl` command-line tool or `kubectl` directly. The pods run a periodic sync process that looks for changes to the Settings CR. When a change is detected it is applied after an internal snapshot is taken.

---

## Next Steps

* Learn more about the [`glooctl` command line tool]({{< versioned_link_path fromRoot="/reference/cli/" >}}) used to manipulate these Custom Resources
* Experiment with your own Gloo Edge environment using our [Getting Started guide]({{< versioned_link_path fromRoot="/getting_started/" >}})