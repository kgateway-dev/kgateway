---
title: "Deployment Options"
weight: 33
description: Infrastructure Options for Installing Gloo
---

Gloo is a flexible architecture that can be deployed on a range of infrastructure stacks. If you'll recall from the Architecture document, Gloo contains the following components at a logical level.

![Component Architecture]({{% versioned_link_path fromRoot="/introduction/component_architecture.png" %}})

In an actual deployment of Gloo, components like storage, secrets, and endpoint discovery must be supplied by the infrastructure stack. Gloo also requires a place to launch the containers that comprise both Gloo and Envoy. The following sections details a potential deployment option along with links to the installation guide for each option.

The options included are:

* [Kubernetes using Kubernetes primitives](#kubernetes-using-kubernetes-primitives)
* [HashiCorp Consul, Vault, and Nomad](#hashicorp-consul-vault-and-nomad)
* Docker Compose with HashiCorp Consul and Vault
* Docker Compose with the local filesystem (development only)

---

## Kubernetes using Kubernetes primitives

The simplest and most common deployment option for Gloo is using Kubernetes to orchestrate the deployment of Gloo, and using Kubernetes primitives like *Custom Resources* and *Config Maps*. The diagram below shows an example of how Gloo might be deployed on Kubernetes and how each primitive is leveraged to match the component architecture.

### Pods and Deployments

The following components of Gloo are deployed as separate pods and deployments:

* Gloo
* Gateway
* Discovery
* Envoy

Each deployment includes a replica set for the pods, which can be used to scale the number of pods and perform rolling upgrades.

### Services

Along with the pods and deployments, three services are created.

* `gloo`: Type ClusterIP exposing the ports 9966 (metrics), 9977 (grpc-xds), 9979 (wasm-cache), and 9988 (grpc-validation)
* `gateway`: Type ClusterIP exposing the port 443
* `gateway-proxy`: Type LoadBalancer exposing the ports 80, 443

The *gloo* service is what exposes the xDS Server running in Gloo.

### ConfigMaps

There are two ConfigMaps created by default:

* `gateway-proxy-envoy-config`: Contains the YAML for the Envoy.
* `gloo-usage`: Records usage data about the Envoy proxy.

The `gateway-proxy-envoy-config` ConfigMap does not contain information about the routing, Upstreams, or Virtual Services. It only contains information about the Envoy configuration itself. This ConfigMap is mounted as a volume on any `gateway-proxy` pods.

### Secrets

Gloo makes use of secrets in Kubernetes to store tokens, certificates, and Helm release info. The following secrets should be present by default.

* default-token
* discovery-token: Mounted as a volume on `discovery` pods.
* gateway-proxy-token: Mounted as a volume on `gateway-proxy` pods.
* gateway-token: Mounted as a volume on `gateway` pods.
* gateway-validation-certs: Mounted as a volume on `gateway` pods.
* gloo-token: Mounted as a volume on `gloo` pods.
* sh.helm.release.v1.gloo.v1

Gloo makes use of certificates for validation and authentication. When Gloo Gateway is installed, it runs a job to generate certificates. The resulting certificate is stored in a Kubernetes secret called `gateway-validation-certs`, and mapped as a volume to the `gateway` pods.

### Custom Resource Definitions

When Gloo is installed on Kubernetes, it creates a number of Custom Resource Definitions that Gloo can use to store data. The following table describes each Custom Resource Definition, its grouping, and its purpose.

| Name | Grouping | Purpose |
|------|----------|---------|
| {{< protobuf name="enterprise.gloo.solo.io.AuthConfig" display="AuthConfig">}} | enterprise.gloo.solo.io | User-facing authentication configuration |
| {{< protobuf name="gloo.solo.io.Proxy" display="Proxy">}} | gloo.solo.io | A combination of Gateway resources to be pushed to the Envoy proxy. |
| {{< protobuf name="gloo.solo.io.Settings" display="Settings">}} | gloo.solo.io | Global settings for all Gloo components. |
| {{< protobuf name="gloo.solo.io.UpstreamGroup" display="UpstreamGroup">}} | gloo.solo.io | Defining multiple Upstreams or external endpoints for a Virtual Service. |
| {{< protobuf name="gloo.solo.io.Upstream" display="Upstream">}} | gloo.solo.io | Upstreams represent destinations for routing HTTP requests. |
| {{< protobuf name="gateway.solo.io.Gateway" display="Gateway">}} | gateway.solo.io | Describes a single Listener and the routing Upstreams reachable via the Gateway Proxy. |
| {{< protobuf name="gateway.solo.io.RouteTable" display="RouteTable">}} | gateway.solo.io | Child Routing object for the Gloo Gateway. |
| {{< protobuf name="gateway.solo.io.VirtualService" display="VirtualService">}} | gateway.solo.io | Describes the set of routes to match for a set of domains. |

You can find out more about deploying Gloo on Kubernetes by [following this guide]({{% versioned_link_path fromRoot="/installation/gateway/kubernetes/" %}}).

---

## HashiCorp Consul, Vault, and Nomad