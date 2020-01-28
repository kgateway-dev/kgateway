---
title: "Architecture"
weight: 30
description: A description of the high-level architecture behind Gloo.
---

## Overview

Gloo aggregates back-end services and provides function-to-function translation for clients, allowing decoupling from back-end APIs. Gloo sits in the control plane and leverages Envoy to provide the data plane proxy for back-end services.

![Overview]({{% versioned_link_path fromRoot="/img/gloo-architecture-envoys.png" %}})

End users issue requests or [emit events](https://github.com/solo-io/gloo-sdk-go) to routes defined on Gloo. These routes are mapped to functions on *Upstream* services by Gloo's configuration. The routes are provided by clients through the Gloo API.

End users connect to Envoy cluster proxies managed by Gloo, which transform requests into function invocations for a variety of functional back-ends. Non-functional back-ends are supported via a traditional Gateway-to-Service routing model.

Gloo performs the necessary transformation between the routes defined by clients and the back-end functions. Gloo is able to support various upstream functions through its extendable [function plugin interface](https://github.com/solo-io/gloo/blob/master/projects/gloo/pkg/plugins/plugin_interface.go).

Gloo offers first-class API management features on all functions:

* Timeouts
* Metrics & Tracing
* Health Checks
* Retries
* Advanced load balancing
* TLS Termination with SNI Support
* HTTP Header modification

---

## Component Architecture

In the most basic sense, Gloo is a translation engine and [Envoy xDS server](https://github.com/envoyproxy/data-plane-api/blob/master/xds_protocol.rst) providing advanced configuration for Envoy (including Gloo's custom Envoy filters). Gloo follows an event-based architecture, watching various sources of configuration for updates and responding immediately with v2 gRPC updates to Envoy.

![Component Architecture]({{% versioned_link_path fromRoot="/introduction/component_architecture.png" %}})

At the logical layer, Gloo is comprised of several different services that perform unique functions. Gloo sits outside the data path, providing the control layer for Envoy and other services through its transformation plug-in. 

The following sections describe the various logical components of Gloo. The [Deployment Architecture guide]({{% versioned_link_path fromRoot="/introduction/deployment_arch" %}}) provides examples and guidance for specific implementations of Gloo on different software stacks.

### Config Watcher

The *Config Watcher* watches the storage layer for updates to user configuration objects, such as [Upstreams]({{% versioned_link_path fromRoot="/introduction/concepts#upstreams" %}}) and [Virtual Services]({{% versioned_link_path fromRoot="/introduction/concepts#virtual-services" %}}). The storage layer could be a custom resource in Kubernetes or an key/value entry in HashiCorp Consul.

### Secret Watcher

The *Secret Watcher* watches a secret store for updates to secrets (which are required for certain plugins such as the {{% protobuf name="aws.options.gloo.solo.io.DestinationSpec" display="AWS Lambda Plugin"%}}. The secret storage could be using secrets management in Kubernetes, HashiCorp Vault, or some other secure secret storage system.

### Endpoint Discovery

*Endpoint Discovery* watches service registries such as Kubernetes, Cloud Foundry, and Consul for IPs associated with services. Endpoint Discovery is plugin-specific, so each endpoint type will require a plug-in that supports the discovery functionality. For example, the {{< protobuf name="kubernetes.options.gloo.solo.io.UpstreamSpec" display="Kubernetes Plugin">}} runs its own Endpoint Discovery goroutine.

### Translator

The *Translator* receives snapshots of the entire state, composed of the following configuration data:

* Artifacts
* Endpoints
* Proxies
* Upstreams
* UpstreamGroups
* Secrets
* AuthConfigs

user configuration, secrets, and discovery information and initiates a new *translation loop*, creating a new Envoy xDS Snapshot.

1. The translation cycle starts by creating *[Envoy clusters](https://www.envoyproxy.io/docs/envoy/v1.8.0/api-v1/cluster_manager/cluster)* from all configured Upstreams. Each Upstream has a *type*, indicating which Upstream plugin is responsible for processing that Upstream object. Correctly configured Upstreams are converted into Envoy clusters by their respective plugins. Plugins may set cluster metadata on the cluster object.

1. The next step in the translation cycle is to process all the functions on each Upstream. Functional plugins process the functions on an Upstream, setting function-specific cluster metadata, which will be later processed by function-specific Envoy filters.

1. The next step generates all of the *[Envoy routes](https://www.envoyproxy.io/docs/envoy/latest/api-v2/api/v2/route/route.proto.html?highlight=route)* via the route plugins. Routes are generated for each route rule defined on the {{< protobuf name="gateway.solo.io.VirtualService" display="Virtual Service objects">}}. When all of the routes are created, the translator aggregates them into *[Envoy virtual hosts](https://www.envoyproxy.io/docs/envoy/latest/api-v2/api/v2/route/route.proto#route-virtualhost)* and adds them to a new *[Envoy HTTP Connection Manager](https://www.envoyproxy.io/docs/envoy/v1.11.2/intro/arch_overview/http/http_connection_management.html#http-connection-management)* configuration.

1. Filter plugins are queried for their filter configurations, generating the list of HTTP Filters that will go on the *[Envoy listeners](https://www.envoyproxy.io/docs/envoy/latest/configuration/listeners/listeners)*.

1. Finally, a snapshot is composed of the all the valid endpoints, clusters, rds configs, and listeners. The snapshot will be passed to the *xDS Server*.

### Reporter

The *Reporter* receives a validation report for every Upstream and Virtual Service processed by the translator. Any invalid config objects are reported back to the user through the storage layer. Invalid objects are marked as *Rejected* with detailed error messages describing mistakes found in the configuration.

### xDS Server

The final snapshot is passed to the *xDS Server*, which notifies Envoy of a successful config update, updating the Envoy cluster with a new configuration to match the desired state set expressed by Gloo.

--- 

## Discovery Architecture

Gloo is supported by a suite of optional discovery services that automatically discover and configure Gloo with Upstreams and functions to simplify routing for users and self-service.

![Discovery Architecture]({{% versioned_link_path fromRoot="/introduction/discovery_architecture.png" %}})

Discovery services act as automated Gloo clients, automatically populating the storage layer with Upstreams and functions to facilitate easy routing for users. Discovery is optional, but when enabled, it will attempt to discover available Upstreams and functions.

The following discovery methods are currently supported:

* Kubernetes Service-Based Upstream Discovery
* AWS Lambda-Based Function Discovery
* Google Cloud Function-Based Function Discovery
* OpenAPI-Based Function Discovery
* Istio-Based Route Rule Discovery (Experimental)

---

## Next Steps

Now that you have a basic understanding of the Gloo architecture, there are number of potential next steps that we'd like to recommend.

* **[Getting Started]({{% versioned_link_path fromRoot="/getting_started/" %}})**: Deploy Gloo yourself or try one of our Katacoda courses.
* **[Deployment Architecture]({{% versioned_link_path fromRoot="/introduction/deployment_arch" %}})**: Learn about specific implementations of Gloo on different software stacks.
* **[Concepts]({{% versioned_link_path fromRoot="/introduction/concepts/" %}})**: Learn more about the core concepts behind Gloo and how they interact.
* **[Developer Guides]({{% versioned_link_path fromRoot="/dev/" %}})**: extend Gloo's functionality for your use case through various plugins.