---
title: "Deployment Options"
weight: 20
description: Infrastructure Options for Installing Gloo Edge
---

Gloo Edge is a flexible architecture that can be deployed on a range of infrastructure stacks. If you'll recall from the Architecture document, Gloo Edge contains the following components at a logical level.

![Component Architecture]({{% versioned_link_path fromRoot="/img/component_architecture.png" %}})

In an actual deployment of Gloo Edge, components like storage, secrets, and endpoint discovery must be supplied by the infrastructure stack. Gloo Edge also requires a place to launch the containers that comprise both Gloo Edge and Envoy. The following sections detail potential deployment options along with links to the installation guide for each option.

The options included are:

* [Kubernetes using Kubernetes primitives](#kubernetes-using-kubernetes-primitives)
* [HashiCorp Consul, Vault, and Nomad](#hashicorp-consul-vault-and-nomad)

It is also possible to use Docker Compose for container management and the local file system for configuration and secrets management. These are development scenarios and should not be considered for a production deployment. Using Kubernetes or HashiCorp products are the two major ways to provide the necessary storage, secrets, and container management components in a production scenario.

---

## Kubernetes using Kubernetes primitives

The simplest and most common deployment option for Gloo Edge is using Kubernetes to orchestrate the deployment of Gloo Edge, and using Kubernetes primitives like *Custom Resources* and *Config Maps*. The diagram below shows an example of how Gloo Edge might be deployed on Kubernetes and how each primitive is leveraged to match the component architecture.

![Kubernetes Architecture]({{% versioned_link_path fromRoot="/img/deployments/kubernetes-deployment-architecture.png" %}})

### Pods and Deployments

The following components of Gloo Edge are deployed as separate pods and deployments:

* Gloo
* Gateway
* Discovery
* Envoy

Each deployment creates a replica set for the pods, which can be used to scale the number of pods and perform rolling upgrades.

### Services

Along with the pods and deployments, three services are created.

* `gloo`: Type ClusterIP exposing the ports 9966 (grpc metrics), 9977 (grpc-xds), 9979 (wasm-cache), and 9988 (grpc-validation)
* `gateway`: Type ClusterIP exposing the port 443
* `gateway-proxy`: Type LoadBalancer exposing the ports 80, 443

The *gloo* service is what exposes the xDS Server running in Gloo Edge.

### ConfigMaps

There are two ConfigMaps created by default:

* `gateway-proxy-envoy-config`: Contains the YAML for the Envoy.
* `gloo-usage`: Records usage data about the Envoy proxy.

The `gateway-proxy-envoy-config` ConfigMap does not contain information about the routing, Upstreams, or Virtual Services. It only contains information about the Envoy configuration itself. This ConfigMap is mounted as a volume on any `gateway-proxy` pods.

### Secrets

Gloo Edge makes use of secrets in Kubernetes to store tokens, certificates, and Helm release info. The following secrets should be present by default.

* default-token
* discovery-token: Mounted as a volume on `discovery` pods.
* gateway-proxy-token: Mounted as a volume on `gateway-proxy` pods.
* gateway-token: Mounted as a volume on `gateway` pods.
* gateway-validation-certs: Mounted as a volume on `gateway` pods.
* gloo-token: Mounted as a volume on `gloo` pods.
* sh.helm.release.v1.gloo.v1

Gloo Edge makes use of certificates for validation and authentication. When Gloo Edge in gateway mode is installed, it runs a job to generate certificates. The resulting certificate is stored in a Kubernetes secret called `gateway-validation-certs`, and mapped as a volume to the `gateway` pods.

### Custom Resource Definitions

When Gloo Edge is installed on Kubernetes, it creates a number of Custom Resource Definitions that Gloo Edge can use to store data. The following table describes each Custom Resource Definition, its grouping, and its purpose.

| Name | Grouping | Purpose |
|------|----------|---------|
| {{< protobuf name="gloo.solo.io.Settings" display="Settings">}} | gloo.solo.io | Global settings for all Gloo Edge components. |
| {{< protobuf name="gateway.solo.io.Gateway" display="Gateway">}} | gateway.solo.io | Describes a single Listener and the routing Upstreams reachable via the Gateway Proxy. |
| {{< protobuf name="gateway.solo.io.VirtualService" display="VirtualService">}} | gateway.solo.io | Describes the set of routes to match for a set of domains. |
| {{< protobuf name="gateway.solo.io.RouteTable" display="RouteTable">}} | gateway.solo.io | Child Routing object for the Gloo Edge. |
| {{< protobuf name="gloo.solo.io.Proxy" display="Proxy">}} | gloo.solo.io | A combination of Gateway resources to be pushed by Gloo Edge to the Envoy proxy. |
| {{< protobuf name="gloo.solo.io.Upstream" display="Upstream">}} | gloo.solo.io | Upstreams represent destinations for routing HTTP requests. |
| {{< protobuf name="gloo.solo.io.UpstreamGroup" display="UpstreamGroup">}} | gloo.solo.io | Defining multiple Upstreams or external endpoints for a Virtual Service. |
| {{< protobuf name="enterprise.gloo.solo.io.AuthConfig" display="AuthConfig">}} | enterprise.gloo.solo.io | User-facing authentication configuration |

You can find out more about deploying Gloo Edge on Kubernetes by [following this guide]({{% versioned_link_path fromRoot="/installation/gateway/kubernetes/" %}}).

---

## HashiCorp Consul, Vault, and Nomad

Gloo Edge can use some of the HashiCorp products to provide the necessary primitives for container management, persistent storage, and secrets management. The diagram below provides and example of how HashiCorp products could be used to host a Gloo Edge deployment.

![HashiCorp Example]({{% versioned_link_path fromRoot="/img/gloo-architecture-nomad-consul-vault.png" %}})

### Containers and Jobs

HashiCorp's [Nomad](https://www.nomadproject.io/) is a a popular workload scheduler that can be used in place of, or in combination with Kubernetes as a way of running long-lived processes on a cluster of hosts. Nomad supports native integration with Consul and Vault, making configuration, service discovery, and credential management easy for application developers. 

Nomad is used to deploy the Gloo Edge containers by using Gloo Edge deployment jobs. Similar to a Kubernetes deployment, each Nomad job defines a set of deployment tasks for the various Gloo Edge components. There are four jobs in total which deploy the following container groups:

* gloo
* discovery
* gateway
* gateway-proxy

Within the definition of each task is the port mappings and service names for each group of containers.

### Services and Configuration

HashiCorps's [Consul](https://www.consul.io/) is a service networking solution to connect and secure services across multiple platforms. It can also store arbitrary key/value pairs. In the case of a Gloo Edge deployment, Consul is used to publish and resolve the networking services published by the Gloo Edge container groups and hold configuration information about Gloo Edge objects like Upstreams, Envoy configs, and Virtual Services.

The **Services** component of Consul publishes the services: consul, gateway-proxy, gloo-xds, nomad, and nomad-client. It will also publish other services deployed through Nomad, which the Discovery service can find and push into the Upstream listing.

The **Key/Value** component of Consul holds data at the following paths:

* gloo/gateway.solo.io/v1/Gateway/gateway-proxy
* gloo/gateway.solo.io/v1/Gateway/gateway-proxy-ssl
* gloo/gateway.solo.io/v1/VirtualService
* gloo/gloo.solo.io/v1/Upstream

The configuration data that would typically be housed in a ConfigMap or Custom Resource on Kubernetes is instead held in one of the above paths on Consul's Key/Value store.

Consul also supports service discovery, which is added to Gloo Edge by publishing a Key/Value entry to the path `gloo/gloo.solo.io/v1/Upstream`.

### Secrets

HashiCorp's Vault is secrets lifecycle management solution providing secure, tightly controlled access to tokens, passwords, certificates, and encryption keys.

You can find out more about deploying Gloo Edge using HashiCorp solutions by [following Gateway guides]({{% versioned_link_path fromRoot="/installation/gateway/" %}}).

---

## Next Steps

Now that you have a basic understanding of the deployment options for Gloo Edge, there are number of potential next steps that we'd like to recommend.

* **[Getting Started]({{% versioned_link_path fromRoot="/getting_started/" %}})**: Deploy Gloo Edge yourself or try one of our Katacoda courses.
* **[Deployment Architecture]({{% versioned_link_path fromRoot="/introduction/architecture/deployment_arch/" %}})**: Learn about specific implementations of Gloo Edge on different software stacks.
* **[Concepts]({{% versioned_link_path fromRoot="/introduction/architecture/concepts/" %}})**: Learn more about the core concepts behind Gloo Edge and how they interact.
* **[Developer Guides]({{% versioned_link_path fromRoot="/guides/dev/" %}})**: extend Gloo Edge's functionality for your use case through various plugins.
