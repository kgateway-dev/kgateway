---
title: Installing Gloo Gateway
description: Guides for installing the Gloo Gateway.
weight: 2
---

# Install the Gloo Gateway

<dic markdown=1>
<table>
  <tr height="100">
    <td width="10%">
      <a href='{{% versioned_link_path fromRoot="/installation/gateway/kubernetes" %}}'><img src='{{% versioned_link_path fromRoot="/img/kube.png" %}}' width="60"/></a>
    </td>
    <td>
     Install the Gloo Gateway on Kubernetes, using Kubernetes Custom Resources to configure routing.
    </td>
  </tr>
  <tr height="100">
    <td width="10%">
      <a href='{{% versioned_link_path fromRoot="/installation/gateway/docker-compose-file" %}}'><img src='{{% versioned_link_path fromRoot="/img/docker.png" %}}' width="60"/></a>
    </td>
    <td>
     Run Gloo locally with Docker Compose, using `yaml` files which are mounted to the Gloo container for configuration and secret storage.
    </td>
  </tr>
  <tr height="100">
    <td width="10%">
      <a href='{{% versioned_link_path fromRoot="/installation/gateway/docker-compose-consul" %}}'><img src='{{% versioned_link_path fromRoot="/img/consul.png" %}}' width="60"/></a>
    </td>
    <td>
     Run Gloo locally with Docker Compose, using [HashiCorp Consul](https://www.consul.io/) for configuration and [HashiCorp Vault](https://www.vaultproject.io/) for secret storage.
    </td>
  </tr>
  <tr height="100">
    <td width="10%">
      <a href='{{% versioned_link_path fromRoot="/installation/gateway/nomad" %}}'><img src='{{% versioned_link_path fromRoot="/img/nomad.png" %}}' width="60"/></a>
    </td>
    <td>
     Run Gloo on a [HashiCorp Nomad Cluster](https://www.nomadproject.io/), using [Consul](https://www.consul.io/) for configuration and [Vault](https://www.vaultproject.io/) for secret storage.
    </td>
  </tr>
</table>
</dic>
