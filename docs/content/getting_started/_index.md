---
title: Getting Started with Gloo
weight: 15
---

# Gloo Says Go

We know you want to jump right in and start hacking away with Gloo. That's awesome! If you want to try out Gloo in a hosted setting, please check out our [Katacoda](https://katacoda.com/solo-io) courses that will guide you through a number of scenarios using Gloo, all in a sandboxed environment.

If you'd rather start using Gloo on your local machine, make sure you have following list of utilities necessary to work with the Gloo tutorials. After the list are some [deployment requirements](#deployment-requirements) depending on your deployment model and [recommendations](#where-to-next) of where to start.

## Utilities List

Below is a list of all the required components and common utilities for use with Gloo as you work through the concepts and guides.

- **kubectl** - Command line utility for Kubernetes
- **glooctl** - Command line utility for Gloo
- **jq** - Utility for manipulating JSON
- **git** - Utility for working with a versioned source control system
- **curl** - Utility for transferring data to/from a server, especially with HTTP/S
- **helm** - Utility for managing charts and deploying applications on Kubernetes
- **openssl** - Cryptography toolkit for working with SSL and TLS

You will also want some type of text editor that understands *YAML*. For real, there is going to be **a lot** of *YAML* and getting the spacing wrong is a huge pain. There are many great tools out there including, but not limited to Visual Studio Code, Sublime Text, and the venerable Vim.

## Deployment Requirements

There are a number of options when it comes to installing Gloo Gateway. The requirements for each deployment model are described below.

### Kuberenetes Deployments

Not sure how you will deploy Gloo? This [section](../installation/cluster_setup/) is for you. Gloo Gateway deploys as a set of containers, and is usually deployed on a Kubernetes cluster. In order to install Gloo Gateway, you will need access to a Kubernetes deployment. That could be a local cluster using *minikube* or *minishift*. It could be a hosted cluster on one of the public clouds such as *Google Kubernetes Engine*, *Elastic Kubernetes Service*, or *Azure Kuberentes Service*. You could even host your own Kubernetes cluster in your datacenter! 

As long as you can run *kubectl* and have cluster-admin permissions, you're all set.

### Docker Compose Deployments

A less common option is to use Docker Compose to deploy the Gloo Gateway components locally and store the configuration and secrets data in the Gloo containers. This will require that *docker* and *docker-compose* are installed on your local machine. Further instructions for setup can be found [here](../installation/gateway/docker-compose-file/).

### Consul and Vault Deployments

Similar to the Docker Compose option, this option leverages HashiCorp Consul for configuration data and HashiCorp Vault for secrets data instead of storing the values directly in the Gloo containers. This will require that *docker* and *docker-compose* are installed locally. Further instructions for setup can be found [here](../installation/gateway/docker-compose-consul/).

### Nomad Deployments

Nomad is a workload scheduler that can be used in place of Docker Compose or Kubernetes. It integrates with Consul and Vault, using them to store configuration and secrets data. Using Nomad will require Levant to be installed locally, and access to a system running Docker, Consul, Vault, and Nomad. Further instructions for setup can be found [here](../installation/gateway/nomad/).

## Where to Next?

The most common starting point is to [install Gloo Gateway](../installation/). Once Gloo Gateway is installed, [Gloo Routing](../gloo_routing/) is likely your go-to destination.  Otherwise, here are some common paths to learning.

- Do you need a Kubernetes cluster? Start [here](../installation/cluster_setup/).
- Need to install the Gloo Gateway? Start [here](../installation/).
- Want to know more about Gloo Routing? Start [here](../gloo_routing/).
- Concerned about security? Start [here](../security/). (*Enterprise Gloo only*)
- Monitoring your thing? Start [here](../observability/). (*Enterprise Gloo only*)
