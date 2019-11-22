---
title: "Architecture"
weight: 30
---

## Overview

Gloo aggregates back end services and provides function-to-function translation for clients, allowing decoupling from back-end APIs

![Overview](../gloo_diagram.png "High Level Architecture")

Clients issue requests or [emit events](https://github.com/solo-io/gloo-sdk-go) to routes defined on Gloo. These routes are mapped
to functions on upstream services by Gloo's configuration (provided by clients of Gloo's API).

Clients connect to proxies managed by Gloo who then transform requests into function invocations
for a variety of functional backends. Non-functional backends are supported via a traditional
Gateway-to-Service routing model.

Gloo performs the necessary transformation between the routes defined by clients and the back end functions. Gloo is able
to support various upstream functions through its extendable [function plugin interface](https://github.com/solo-io/gloo/blob/master/projects/gloo/pkg/plugins/plugin_interface.go).

Gloo offers first-class API management features on all functions:

* Timeouts
* Metrics & Tracing
* Health Checks
* Retries
* Advanced load balancing
* TLS Termination with SNI Support
* HTTP Header modification

## Component Architecture

In the most basic sense, Gloo is a translation engine and [Envoy xDS server](https://github.com/envoyproxy/data-plane-api/blob/master/xds_protocol.rst)
providing advanced configuration for Envoy (including Gloo's custom Envoy filters). Gloo follows an event-based architecture, watching various sources of configuration for updates and responding immediately with v2 gRPC updates to Envoy.

![Component Architecture](../component_architecture.png "Component Architecture")

* The **Config Watcher** watches the storage layer for updates to user configuration objects ([Upstreams](../concepts#upstreams) and [Virtual Services](../concepts#virtual-services)).
* The **Secret Watcher** watches a secret store for updates to secrets (which are required for certain plugins such as the {{% protobuf name="aws.options.gloo.solo.io.DestinationSpec" display="AWS Lambda Plugin"%}}.
* **Endpoint Discovery** watches service registries such as Kubernetes, Cloud Foundry, and Consul for IPs associated with services.
Endpoint Discovery is plugin-specific. For example, the {{< protobuf name="kubernetes.options.gloo.solo.io.UpstreamSpec" display="Kubernetes Plugin">}} runs its own Endpoint Discovery goroutine.
* The **Translator** receives snapshots of the entire state, composed of user configuration, secrets, and discovery information
and initiates a new *translation loop*, creating a new Envoy xDS Snapshot.
  1. The translation cycle starts by creating **[Envoy clusters](https://www.envoyproxy.io/docs/envoy/v1.8.0/api-v1/cluster_manager/cluster)** from all configured upstreams. Each upstream has a **type**, indicating which upstream plugin is responsible for
  processing that upstream object. Correctly configured upstreams are converted into Envoy clusters by their respective
  plugins. Plugins may set cluster metadata on the cluster object.
  1. The next step in the translation cycle is to process all the functions on each upstream. Functional plugins process
  the functions on upstream, setting function-specifc cluster metadata, which will be later processed by
  function-specific Envoy filters.
  1. The next step generates all of the **[Envoy routes](https://www.envoyproxy.io/docs/envoy/latest/api-v2/api/v2/route/route.proto.html?highlight=route)**
  via the route plugins . Routes are generated for each route rule defined on the {{< protobuf name="gateway.solo.io.VirtualService" display="virtual service objects">}}. When all of the routes are created, the translator aggregates them into
  [Envoy virtual hosts](https://www.envoyproxy.io/docs/envoy/latest/api-v2/api/v2/route/route.proto#route-virtualhost)
  and adds them to a new [Envoy HTTP Connection Manager](https://www.envoyproxy.io/docs/envoy/v1.11.2/intro/arch_overview/http/http_connection_management.html#http-connection-management)
  configuration.
  1. Filter plugins are queried for their filter configurations, generating the list of HTTP Filters that will go on the
  [Envoy listeners](https://www.envoyproxy.io/docs/envoy/latest/configuration/listeners/listeners).
  1. Finally, a snapshot is composed of the all the valid endpoints, clusters, rds configs, and listeners
* The **Reporter** receives a validation report for every upstream and virtual service processed by the translator. Any invalid
config objects are reported back to the user through the storage layer. Invalid objects are marked as "Rejected" with
detailed error messages describing mistakes in the user config.
* The final snapshot is passed to the **xDS server**, which notifies Envoy of a successful config update, updating the Envoy
cluster with a new configuration to match the desired state set by Gloo.

## Discovery Architecture

Gloo is supported by a suite of optional discovery services that automatically discover and configure Gloo with
upstreams and functions to simplify routing for users and self-service.

![Discovery Architecture](../discovery_architecture.png "Discovery Architecture")

Discovery services act as automated Gloo clients, automatically populating the storage layer with upstreams and functions
to facilitate easy routing for users. Discovery is optional, but when enabled, it will attempt to discover available
upstreams and functions.

Currently supported:

* Kubernetes Service-Based Upstream Discovery
* AWS Lambda-Based Function Discovery
* Google Cloud Function-Based Function Discovery
* OpenAPI-Based Function Discovery
* Istio-Based Route Rule Discovery (Experimental)
