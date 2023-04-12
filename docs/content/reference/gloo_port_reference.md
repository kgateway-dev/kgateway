---
title: "Gloo Edge Port Reference"
description: Listing of ports used by Gloo Edge and Gloo Edge Enterprise
weight: 35
---

Gloo Edge and Gloo Edge Enterprise both deploy containers that listen on certain ports for incoming traffic. This document lists out the pods and services that make up Gloo Edge and Gloo Edge Enterprise, and the ports which these pods and services listen on. It is also possible to set up mutual TLS (mTLS) for communication between Gloo Edge resources. The addition of mTLS changes the ports and traffic flows slightly, which is addressed in this document as well.

This document is specific to the pods and services deployed in a Kubernetes environment. Deploying Gloo Edge using the HashiCorp stack is also supported, and the port mappings and services should be the same.

{{% notice note %}}
It is possible to customize some port settings by providing custom values to the Helm chart that installs Gloo Edge open-source and Gloo Edge Enterprise. The port reference below is for an installation of Gloo Edge that uses the default settings in the Helm chart.
{{% /notice %}}

---

## Gloo Edge Open-source

Gloo Edge open-source software is the free, open-source version of Gloo Edge. The installation process uses a Helm chart to create the necessary custom resource definitions (CRDs), deployments, services, pods, etc. The services and pods listen on specific ports to enable communication between the components that make up Gloo Edge and outside sources that will consume Upstream resources through Gloo Edge.

### What's included

A standard installation of Gloo Edge includes four primary components:

* **Gateway**
  * Translates Gateway, Virtual Service, and RouteTable custom resources into a Proxy custom resource.
  * Validates proposed configurations before application.
* **Gloo Edge**
  * Creates an Envoy configuration from multiple custom resources.
  * Serves Envoy configurations using xDS.
  * Validates Proxy configurations for the Gateway.
* **Proxy**
  * Receives and loads configuration from Gloo Edge xDS.
  * Proxies incoming traffic.
* **Discovery**
  * Discovers Upstreams in the cluster.
  * Discovers functions with the Function Discovery Service.

### Pods and ports

The four primary components are instantiated using pods and services. The following table lists the deployed pods and ports in use by each pod, as well as the optional `access-log` pod if Access Logging has been enabled.

| Pod | Port | Usage |
|-----|------|-------|
| gateway | 8443 | Validation |
| gloo | 9977 | xDS Server |
| gloo | 9988 | Validation |
| gloo | 9979 | WASM cache |
| gateway-proxy | 8080 | HTTP |
| gateway-proxy | 8443 | HTTPS |
| gateway-proxy | 19000 | Envoy admin |
| access-log | 8083 | Access logging |

The `discovery` pod does not listen on any ports as it uses outbound connections only.

### Services and ports

The following table lists the services backed by the deployed pods.

| Service | Port | Target | Target Port | Usage            |
|---------|------|--------|-------------|------------------|
| gateway | 443 | gateway | 8443 | Validation       |
| gloo | 9977 | gloo | 9977 | xDS Server       |
| gloo | 9988 | gloo | 9988 | Validation       |
| gloo | 9979 | gloo | 9979 | WASM cache       |
| gloo | 9966 | gloo | 9966 | Proxy Debug gRPC |
| gateway-proxy | 80 | gateway-proxy | 8080 | HTTP             |
| gateway-proxy | 443 | gateway-proxy | 8443 | HTTPS            |
| access-log | 8083 | access-log | 8083 | Access logging   |

---

## Gloo Edge Enterprise

Gloo Edge Enterprise adds many pods and services to provide the extra functionality included in the paid offering. More information on what is included in Gloo Edge Enterprise can be found on the [Gloo Edge product page](https://www.solo.io/products/gloo/). 

### What's included

At a high level, the following additional components are available in Gloo Edge Enterprise.

* API and UI server
* External authentication
* Prometheus metrics collection
* Prometheus server
* Grafana dashboard creation and presentation
* Rate-limiting

The Prometheus server and Grafana dashboard are optional components. If you have an existing instance of either, they can be used instead. More information is available in the [Observability section]({{< versioned_link_path fromRoot="/introduction/observability/" >}}) of the docs.

### Pods and ports

The Gloo Edge Enterprise components are instantiated using pods and services. The following table lists the deployed pods and ports in use by each pod.

| Pod | Port | Usage |
|-----|------|-------|
| gloo-fed-console | 8090 | UI server |
| gloo-fed-console | 10101 | API Server |
| gloo-fed-console | 8081 | healthcheck |
| extauth | 8083 | External authentication |
| grafana | 80 | Grafana (unused) |
| grafana | 3000 | Grafana UI |
| prometheus-kube-state-metrics | 8080 | Kubernetes metric collection |
| prometheus-server | 9090 | Prometheus server |
| rate-limit | 18081 | Rate-limiting |
| redis | 6379 | Rate-limiting |

There is an `observability` pod that automatically configures dashboards on the Grafana instance. It does not accept inbound traffic, so it is not included in the table above.

### Services and ports

The following table lists the services backed by the deployed pods.

| Service | Port | Target | Target Port | Usage |
|---------|------|--------|-------------|-------|
| gloo-fed-console | 8090 | UI server |
| gloo-fed-console | 10101 | API Server |
| gloo-fed-console | 8081 | healthcheck |
| extauth | 8083 | extauth | 8083 | External authentication |
| glooe-grafana | 80 | grafana | 3000 | Grafana UI |
| glooe-prometheus-kube-state-metrics-v2 | 80 | prometheus-kube-state-metrics | 8080 | Kubernetes metric collection |
| glooe-prometheus-server | 80 | prometheus-server | 9090 | Prometheus server |
| rate-limit | 18081 | rate-limit | 18081 | Rate-limiting |
| redis | 6379 | redis | 6379 | Rate-limiting |

---

## mTLS considerations

Gloo Edge supports the use of mutual TLS (mTLS) communication between the Gloo Edge pod and other services, including the Envoy proxy, Extauth server, and Rate-limiting server. Enabling mTLS includes the addition of sidecars for multiple pods, Envoy proxy for TLS termination and SDS for certificate rotation and management. More information on the details of mTLS implementation are available in the [mTLS doc]({{< versioned_link_path fromRoot="/guides/security/tls/mtls/" >}}).

### Updated pods

The following pods are updated to support mTLS:
* **Gloo Edge**: Envoy and SDS sidecars are added.
* **Gateway-proxy**: SDS sidecars added and ConfigMap updated for mTLS.
* **ExtAuth**: Envoy and SDS sidecars are added.
* **Rate-limit**: Envoy and SDS sidecars are added.

The additional Envoy sidecar has an admin port listening on 8081 for each pod.

### Updated traffic flow

The Envoy sidecar on the Gloo Edge, Extauth, and Rate-limit pods will be intercepting the inbound traffic for each pod and performing the TLS decryption before passing the traffic to the main container. This does not alter the ports being used by the pods and services, but it does create additional ports that are used internally within the pod for communication. For instance, the Gloo Edge pod continues to listen on 9977 as the xDS server. Internally, the Gloo Edge container is listening on 127.0.0.1:9999 for xDS requests. The Envoy sidecar in the pod accepts requests on 9977, decrypts the request, and sends it to port 9999 on the localhost for processing.

---

## Summary and next steps

This document provides the ports being used by pods and services on a default installation of Gloo Edge and Gloo Edge Enterprise. Some of these ports can be customized, and additional components can be added that introduce more pods and services. To better understand the architecture of Gloo Edge, we recommend reading the following docs:

* [Gloo Edge Architecture]({{< versioned_link_path fromRoot="/introduction/architecture/" >}})
* [Custom Resource Usage]({{< versioned_link_path fromRoot="/introduction/architecture/custom_resources/" >}})
* [Deployment Options]({{< versioned_link_path fromRoot="/introduction/architecture/deployment_options/" >}})
* [mTLS Deployment]({{< versioned_link_path fromRoot="/guides/security/tls/mtls/" >}})
