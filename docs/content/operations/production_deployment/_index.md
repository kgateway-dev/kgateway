---
title: Production Deployments
description: This document shows some tips and tricks for deploying Gloo Edge into a production environment
weight: 20
---

This document shows some of the Production options that may be useful. We will continue to add to this document and welcome users of Gloo Edge to send PRs to this as well.

## Dropping capabilities

One of the more important (and unique) things about Gloo Edge is the ability to significantly lock down the edge proxy. Other proxies require privileges to write to disk or access the Kubernetes API, while Gloo Edge splits those responsibilities between control plane and data plane. The data plane can be locked down with zero privileges while separating out the need to secure the control plane differently. 

For example, Gloo Edge's data plane (the `gateway-proxy` pod) has ReadOnly file system. Additionally it doesn't require any additional tokens mounted in or OS-level privileges. By default some of these options are enabled to simplify developer experience, but if your use case doesn't need them, you should lock them down. 

* **Disable service account token mount**
    - For example, when integrating with Istio's SDS (see integration with Istio), you need to have a service account token mounted. If you're not integrating with Istio, you can eliminate the need for the service account token. When installing Gloo Edge, set the `gateway.proxyServiceAccount.disableAutomount` field. 
* **Disable Kubernetes destinations**
    - Gloo Edge out of the box routes to upstreams. It can also route directly to Kubernetes destinations (bypassing upstreams). Upstreams is the recommended abstraction to which to route in VirtualServices, and you can disable the Kubernetes destinations with the `settings.gloo.disableKubernetesDestinations`. This saves on memory overhead so Gloo Edge pod doesn't cache both upstreams and Kubernetes destinations. 

## Enable replacing invalid routes

* **Configure invalidConfigPolicy**
    - In some cases, it may be desirable to update a virtual service even if its config becomes partially invalid. This is particularly useful when delegating to Route Tables as it ensures that a single Route Table will not block updates for other Route Tables which share the same Virtual Service. More information on why and how to enable this can be found [here]({{% versioned_link_path fromRoot="/traffic_management/configuration_validation/invalid_route_replacement/" %}})

## Enable health checks

{{% notice warning %}}
Liveness/readiness probes on Envoy are disabled by default. This is because Envoy's behavior can be surprising: When there are no
routes configured, Envoy reports itself as un-ready. As it becomes configured with a nonzero number of routes, it will start to
report itself as ready.
{{% /notice %}}

* **Enable liveness/readiness probes for Envoy**
    - To enable liveness and readiness probes, specify `gatewayProxies.gatewayProxy.podTemplate.probes=true` in your Helm installation.
If you are running Gloo Edge Enterprise, you'll need to prefix that Helm values key with `"gloo."`; e.g. `gloo.gatewayProxies.gatewayProxy.podTemplate.probes=true`.
* **Configure your load balancer correctly**
    - If you are running Gloo Edge behind a load balancer, be sure to configure your load balancer properly to consume the readiness probe mentioned above.

## Envoy performance

* **Enable Envoy's gzip filter**
    - Optionally, you may choose to enable Envoy's gzip filter through Gloo Edge. More information on that can be found [here]({{% versioned_link_path fromRoot="/installation/advanced_configuration/gzip/" %}}).
* **Set up an EDS warming timeout**
    - Set up the endpoints warming timeout to some nonzero value. More details [here]({{%versioned_link_path fromRoot="/operations/upgrading/1.3.0/#recommended-settings" %}}).

## Other Envoy-specific guidance

* Envoy has a list of edge proxy best-practices in their docs. You may also want to consult that to see what is applicable for your use case. Find those docs [here](https://www.envoyproxy.io/docs/envoy/latest/configuration/best_practices/edge#best-practices-edge).
    - In particular, you may especially want to set `use_remote_address` to true. More details [here](https://www.envoyproxy.io/docs/envoy/latest/api-v3/extensions/filters/network/http_connection_manager/v3/http_connection_manager.proto#envoy-v3-api-field-extensions-filters-network-http-connection-manager-v3-httpconnectionmanager-use-remote-address)
