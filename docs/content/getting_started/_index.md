---
title: Getting Started
weight: 5
---

To jump right in with Gloo Edge, the quickest way is with your own Kubernetes cluster. Your cluster might be on your local workstation or in a cloud environment. For more details, see the [Preparation]({{% versioned_link_path fromRoot="/installation/preparation/" %}}) guide and [Kubernetes cluster setup]({{< versioned_link_path fromRoot="/installation/platform_configuration/cluster_setup/" >}}). 

## Quick start installation {#install}

After you have a cluster, you can install Gloo Edge through the command line with the `glooctl` CLI or Helm.

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

That's it! Now, you can try the [Hello World example]({{% versioned_link_path fromRoot="/guides/traffic_management/hello_world/" %}}) or the following [hands-on demo](#demo) to get an idea of how Gloo Edge can be used.

## From zero to Gloo Edge in 15 minutes {#demo}

Configure your first cloud-native application with the Gloo Edge API gateway by working your way through the following Instruqt sandbox environment. 

<iframe width="1140" height="640" sandbox="allow-same-origin allow-scripts allow-popups allow-forms allow-modals" src="https://play.instruqt.com/embed/soloio/tracks/zero-to-gloo-edge?token=em_pweesjcvxxsdfzdy" style="border: 0;"></iframe>

