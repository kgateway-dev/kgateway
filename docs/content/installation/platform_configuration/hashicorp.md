---
title: HashiCorp Setup
weight: 20
description: How to prepare a HashiCorp stack for Gloo Edge installation.
---

Installing Gloo Edge will require an environment for installation. In this document we will review how to prepare an environment that uses the HashiCorp family of products for the installation of Gloo Edge.

## Overview

HashiCorp has the following products that can be utilized by Gloo Edge for different functions:

* **Nomad** - A simple and flexible workload orchestrator that can deploy the Gloo Edge containers on managed hosts.
* **Consul** - A service networking solution that can provide both service discovery and key/value storage for Gloo Edge.
* **Vault** - A secrets lifecycle management tool that can provide secure storage of secrets within Gloo Edge.

![HashiCorp Example]({{% versioned_link_path fromRoot="/img/gloo-architecture-nomad-consul-vault.png" %}})

### Nomad

HashiCorp provides excellent [documentation](https://nomadproject.io/docs/) and [examples](https://learn.hashicorp.com/nomad) on how to deploy Nomad in a development or production environment. The Nomad agent is installed on nodes that will host workloads deployed by a Nomad job. In the case of Gloo Edge, the hosts must support the deployment of containers.

You should have the most recent version of Nomad downloaded and the Nomad agent deployed on any worker nodes on which you want to install Gloo Edge. You can also run Nomad locally in development mode.

### Consul

HashiCorp provides [documentation](https://www.consul.io/docs/index.html) and [examples](https://learn.hashicorp.com/consul) on how to deploy Consul in a development or production environment. Consul can be deployed on bare-metal, virtual machines, or containers. It is possible to deploy Consul on Kubernetes, and then use Consul for service discovery and configuration data storage instead of using Custom Resources and Kubernetes service discovery.

You should have the most recent version of Consul downloaded and deployed in a location that is addressable by your target Gloo Edge environment. Consul and Gloo Edge do not have to run on the same Nomad nodes or Kubernetes cluster.  You can also run Consul locally in development mode.

### Vault

HashiCorp provides [documentation](https://www.vaultproject.io/docs/) and [examples](https://learn.hashicorp.com/vault) on how to deploy Vault in a development or production environment. Vault can be deployed on bare-metal, virtual machines, or containers. It is possible to deploy Vault on Kubernetes, and then use Vault for secrets management instead of using Secrets in Kubernetes.

You should have the most recent version of Vault downloaded and deployed in a location that is addressable by your target Gloo Edge environment. Vault and Gloo Edge do not have to run on the same Nomad nodes or Kubernetes cluster.  You can also run Vault locally in development mode.

---

## Next Steps

Once you have prepared a suitable environment for the deployment of Gloo Edge on HashiCorp's products, you can [run through the guide]({{% versioned_link_path fromRoot="/installation/gateway/nomad/" %}}) for getting Gloo Edge deployed.