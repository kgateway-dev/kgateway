---
title: Getting Started
weight: 5
---

We know you want to jump right in and start hacking away with Gloo Edge. That's awesome! The easiest way to do that is with your own Kubernetes cluster, either on your local workstation or in a cloud environment. First, you'll need to install Gloo Edge using either the `glooctl` utility or Helm.

{{< tabs >}}
{{% tab name="glooctl"%}}
1. Install `glooctl`. The steps vary on your operating system.
   * Linux or macOS
     ```shell
     curl -sL https://run.solo.io/gloo/install | sh
     export PATH=$HOME/.gloo/bin:$PATH
     ```
   * Windows
     ```shell
     (New-Object System.Net.WebClient).DownloadString("https://run.solo.io/gloo/windows/install") | iex
     $env:Path += ";$env:userprofile/.gloo/bin/"
     ``` 
2. Install Gloo Edge.
   ```shell
   glooctl install gateway
   ````
{{% /tab %}}
{{% tab name="Helm"%}}
1. Add the Helm repository for Gloo Edge.
   ```shell
   helm repo add gloo https://storage.googleapis.com/solo-public-helm
   helm repo update
   ```
2. Create the namespace for the Gloo Edge components.
   ```shell
   kubectl create namespace gloo-system
   ```
3. Install the Helm chart.
   ```shell
   helm install gloo gloo/gloo --namespace gloo-system
   ```
{{% /tab %}}
{{< /tabs >}}

That's it! With Gloo Edge installed you can try our [Hello World example]({{% versioned_link_path fromRoot="/guides/traffic_management/hello_world/" %}}) to get an idea of how Gloo Edge can be used.

For a more detailed guide on getting your local system configured, installing prerequisites, and deployment options check out our [Preparation]({{% versioned_link_path fromRoot="/installation/preparation/" %}}) doc.
