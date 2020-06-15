---
title: Production Deployments
description: This document shows some tips and tricks for deploying Gloo into a production environment
weight: 20
---

This document shows some of the Production options that may be useful. We will continue to add to this document and welcome users of Gloo to send PRs to this as well.

## Dropping capabilities

One of the more important (and unique) things about Gloo is the ability to significantly lock down the edge proxy. Other proxies require privileges to write to disk or access the Kubernetes API, while Gloo splits those responsibilities between control plane and data plane. The data plane can be locked down with zero privileges while separating out the need to secure the control plane differently. 

For example, Gloo's data plane (the `gateway-proxy` pod) has ReadOnly file system. Additionally it doesn't require any additional tokens mounted in or OS-level privileges. By default some of these options are enabled to simplify developer experience, but if your use case doesn't need them, you should lock them down. 

* **Disable service account token mount**
    - For example, when integrating with Istio's SDS (see integration with Istio), you need to have a service account token mounted. If you're not integrating with Istio, you can elimiate the need for the service account token. When installing Gloo, set the `gateway.proxyServiceAccount.disableAutomount` field. 
* **Dropping NET_BIND**
    - The proxy comes out of the box with security privileges of `NET_BIND` added. This is to simplify the experience for those running the proxy on HostNetwork. If you don't do this, you can drop the `NET_BIND` privileges in the `gateway-proxy` pod. For example, you'll want to do this on OpenShift without giving any additional SCCs. 
* **Disable Kubernetes destinations**
    - Gloo out of the box routes to upstreams. It can also route directly to Kubernetes destinations (bypassing upstreams). Upstreams is the recommended abstraction to which to route in VirtualServices, and you can disable the Kubernetes destinations with the `settings.gloo.disableKubernetesDestinations`. This saves on memory overhead so Gloo pod doesn't cache both upstreams and Kubernetes destinations. 

## Enable edge proxy health checks

{{% notice warning %}}
Liveness/readiness probes on Envoy are disabled by default. This is because Envoy's behavior can be surprising: When there are no
routes configured, Envoy reports itself as un-ready. As it becomes configured with a nonzero number of routes, it will start to
report itself as ready.
{{% /notice %}}

* **Enable liveness/readiness probes**
    - To enable liveness and readiness probes, specify `gatewayProxies.gatewayProxy.podTemplate.probes=true` in your Helm installation.
