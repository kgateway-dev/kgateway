---
title: Gloo Gateway
description: Guides for installing the Gloo Gateway.
weight: 30
---

## Install the Gloo Gateway

There are two primary ways to install the Gloo Gateway in production:

|    |    |
|----|----|
|![Kubernetes]({{< versioned_link_path fromRoot="/img/kube.png" >}}) | Install the Gloo Gateway on [Kubernetes]({{< versioned_link_path fromRoot="/installation/gateway/kubernetes" >}}), using Kubernetes Custom Resources to configure routing. |
| ![HashiCorp]({{< versioned_link_path fromRoot="/img/nomad.png" >}}) | Run Gloo on a [HashiCorp Nomad Cluster]({{< versioned_link_path fromRoot="/installation/gateway/nomad" >}}), using Consul for configuration and Vault for secret storage. |

The Enterprise version of Gloo can be installed using the following guide:

|    |    |
|----|----|
| ![Gloo Enterprise]({{< versioned_link_path fromRoot="/img/gloo-ee.png" >}}) | [Gloo Enterprise]({{< versioned_link_path fromRoot="/installation/gateway/enterprise" >}}) is based on the open-source Gloo Gateway with additional (closed source) UI and plugins. |

You also install Gloo Gateway in a development scenario on your local workstation using one of the following guides:

|    |    |
|----|----|
| ![Docker with HashiCorp]({{< versioned_link_path fromRoot="/img/consul.png" >}}) | [Run Gloo locally with Docker Compose]({{< versioned_link_path fromRoot="/installation/gateway/development/docker-compose-consul" >}}), using HashiCorp Consul for configuration and HashiCorp Vault for secret storage. |
| ![Docker with files]({{< versioned_link_path fromRoot="/img/docker.png" >}}) | [Run Gloo locally with Docker Compose]({{< versioned_link_path fromRoot="/installation/gateway/development/docker-compose-file" >}}), using `yaml` files which are mounted to the Gloo container for configuration and secret storage. |
