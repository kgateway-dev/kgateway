---
title: Preparation
weight: 10
---

Installing Gloo Edge in your environment and walking through the step-by-step guides requires the installation of utilities on your local system and the selection of a deployment model. This document outlines the common utilities that you should have on your local system, and a brief discussion of deployment options for Gloo Edge.

---

## Utilities List

Below is a list of all the required components and common utilities for use with Gloo Edge as you work through the concepts and guides.

- **kubectl** - Command line utility for Kubernetes
- **glooctl** - Command line utility for Gloo Edge
- **jq** - Utility for manipulating JSON
- **git** - Utility for working with a versioned source control system
- **curl** - Utility for transferring data to/from a server, especially with HTTP/S
- **helm** - Utility for managing charts and deploying applications on Kubernetes
- **openssl** - Cryptography toolkit for working with SSL and TLS

You will also want some type of text editor that understands *YAML*. For real, there is going to be **a lot** of *YAML* and getting the spacing wrong is a huge pain. There are many great tools out there including, but not limited to Visual Studio Code, Sublime Text, and the venerable Vim.

## glooctl

{{< readfile file="installation/glooctl_setup.md" markdown="true" >}}

## Licensing

You must provide the license key during the installation process.

1. [Contact an account representative to get a Gloo Edge license key](https://www.solo.io/company/talk-to-an-expert/). {{% notice tip %}}You can also request a [trial license key](https://www.solo.io/products/gloo/#enterprise-trial) instead. Note that each trial license key is typically valid for **30 days**. When the license key expires, you can request a new license key by contacting your account representative or filling out [this form](https://lp.solo.io/request-trial). For more information, see [Updating Enterprise Licenses]({{< versioned_link_path fromRoot="/operations/updating_license/" >}}).{{% /notice %}}
2. Decide how you want to provide your license keys during installation.
   * **Provide license key directly**: When you install Gloo Edge, you can provide the license key string directly as the value for the `license_key` field in your Helm values file, or provide the `--license-key` flag in your `glooctl install` command. A Kubernetes secret is created for you to store the license key.
   * **Provide license key in a secret**: You can specify your license key by creating a secret before you install Gloo Edge.
     1. Create a secret with your license key in the `gloo-system` namespace of your cluster.
        ```yaml
        cat << EOF | kubectl apply -n gloo-system -f -
        apiVersion: v1
        kind: Secret
        type: Opaque
        metadata:
          name: license-key
          namespace: gloo-system
        data:
          license-key: ""
        EOF
        ```
     2. When you install Gloo Edge, specify the secret name and disable default secret generation in your Helm values file or the `glooctl install` command.
        * **Helm**: In your Helm values file, provide the secret name as the value for the `gloo.license_secret_name` field, and set `create_license_secret` to `false`.
        * **glooctl**: In your `glooctl install` command, include the `--set gloo.license_secret_name=<license>` and `--set create_license_secret=false` flags.

## Deployment Requirements

There are a number of options when it comes to installing Gloo Edge. The requirements for each deployment model are described below.

### Kubernetes Deployments

Not sure how you will deploy Gloo Edge? This [section]({{% versioned_link_path fromRoot="/installation/platform_configuration/cluster_setup/" %}}) is for you. Gloo Edge deploys as a set of containers, and is usually deployed on a Kubernetes cluster. In order to install Gloo Edge, you will need access to a Kubernetes deployment. That could be a local cluster using *minikube* or *minishift*. It could be a hosted cluster on one of the public clouds such as *Google Kubernetes Engine*, *Elastic Kubernetes Service*, or *Azure Kubernetes Service*. You could even host your own Kubernetes cluster in your datacenter! 

As long as you can run *kubectl* and have cluster-admin permissions, you're all set.

### Docker Compose Deployments

A less common option is to use Docker Compose to deploy the Gloo Edge components locally and store the configuration and secrets data in the Gloo Edge containers. This will require that *docker* and *docker-compose* are installed on your local machine. Further instructions for setup can be found [here]({{% versioned_link_path fromRoot="/installation/gateway/development/docker-compose-file/" %}}).

### Consul and Vault Deployments

Similar to the Docker Compose option, this option leverages HashiCorp Consul for configuration data and HashiCorp Vault for secrets data instead of storing the values directly in the Gloo Edge containers. This will require that *docker* and *docker-compose* are installed locally. Further instructions for setup can be found [here]({{% versioned_link_path fromRoot="/installation/gateway/development/docker-compose-consul/" %}}).

### Nomad Deployments

Nomad is a workload scheduler that can be used in place of Docker Compose or Kubernetes. It integrates with Consul and Vault, using them to store configuration and secrets data. Using Nomad will require Levant to be installed locally, and access to a system running Docker, Consul, Vault, and Nomad. Further instructions for setup can be found [here]({{% versioned_link_path fromRoot="/installation/gateway/nomad/" %}}).

---

## Where to Next?

The most common starting point is to [install Gloo Edge]({{% versioned_link_path fromRoot="/installation/" %}}). Once Gloo Edge is installed, [Traffic Management]({{% versioned_link_path fromRoot="/guides/traffic_management/" %}}) is likely your go-to destination.  Otherwise, here are some common paths to learning.

- Do you need a Kubernetes cluster? Start [here]({{% versioned_link_path fromRoot="/installation/platform_configuration/cluster_setup/" %}}).
- Need to install the Gloo Edge? Start [here]({{% versioned_link_path fromRoot="/installation/" %}}).
- Want to know more about Gloo Edge Routing? Start [here]({{% versioned_link_path fromRoot="/introduction/traffic_management/" %}}).
- Concerned about security? Start [here]({{% versioned_link_path fromRoot="/guides/security/" %}}). (*Enterprise Gloo Edge only*)
- Monitoring your thing? Start [here]({{% versioned_link_path fromRoot="/guides/observability/" %}}). (*Enterprise Gloo Edge only*)