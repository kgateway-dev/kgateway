---
title: Gloo Gateway
description: Guides for installing the Gloo Gateway.
weight: 30
---

## Install the Gloo Gateway

There are two primary ways to install the Gloo Gateway in production:

<div markdown=1>
<table>
  <tr height="100">
    <td width="10%">
      <a href="{{< versioned_link_path fromRoot="/installation/gateway/kubernetes" >}}"><img src='{{< versioned_link_path fromRoot="/img/kube.png" >}}' width="60"/></a>
    </td>
    <td>
     Install the Gloo Gateway on <a href="{{< versioned_link_path fromRoot="/installation/gateway/kubernetes" >}}">Kubernetes</a>, using Kubernetes Custom Resources to configure routing.
    </td>
  </tr>
  <tr height="100">
    <td width="10%">
      <a href="{{< versioned_link_path fromRoot="/installation/gateway/nomad" >}}"><img src='{{< versioned_link_path fromRoot="/img/nomad.png" >}}' width="60"/></a>
    </td>
    <td>
     Run Gloo on a <a href="{{< versioned_link_path fromRoot="/installation/gateway/nomad" >}}">HashiCorp Nomad Cluster</a>, using Consul for configuration and Vault for secret storage.
    </td>
  </tr>
</table>
</div>

The Enterprise version of Gloo can be installed using the following guide:

<div markdown=1>
<table>
  <tr height="100">
    <td width="10%">
      <a href="{{< versioned_link_path fromRoot="/installation/enterprise/" >}}"><img src='{{< versioned_link_path fromRoot="/img/gloo-ee.png" >}}' width="60"/></a>
    </td>
    <td>
     <a href="{{< versioned_link_path fromRoot="/installation/enterprise/" >}}">Gloo Enterprise</a> is based on the open-source Gloo Gateway with additional (closed source) UI and plugins.
    </td>
  </tr>
</table>
</div>

You also install Gloo Gateway in a development scenario on your local workstation using one of the following guides:

<div markdown=1>
<table>
  <tr height="100">
    <td width="10%">
      <a href="{{< versioned_link_path fromRoot="/installation/gateway/development/docker-compose-consul" >}}"><img src='{{< versioned_link_path fromRoot="/img/consul.png" >}}' width="60"/></a>
    </td>
    <td>
     <a href="{{< versioned_link_path fromRoot="/installation/gateway/development/docker-compose-consul" >}}">Run Gloo locally with Docker Compose</a>, using HashiCorp Consul for configuration and HashiCorp Vault for secret storage.
    </td>
  </tr>
  <tr height="100">
    <td width="10%">
      <a href="{{< versioned_link_path fromRoot="/installation/gateway/development/docker-compose-file" >}}"><img src='{{< versioned_link_path fromRoot="/img/docker.png" >}}' width="60"/></a>
    </td>
    <td>
     <a href="{{< versioned_link_path fromRoot="/installation/gateway/development/docker-compose-file" >}}">Run Gloo locally with Docker Compose</a>, using <code>yaml</code> files which are mounted to the Gloo container for configuration and secret storage.
    </td>
  </tr>
</table>
</div>
