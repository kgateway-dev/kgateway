---
title: Getting Started
weight: 5
---

We know you want to jump right in and start hacking away with Gloo Edge. That's awesome! The easiest way to do that is with your own Kubernetes cluster, either on your local workstation or in a cloud environment. First, you'll need to install Gloo Edge using either the `glooctl` utility or Helm.

{{< tabs >}}
{{< tab name="glooctl" codelang="shell">}}
# Install glooctl

## Linux/MacOS
curl -sL https://run.solo.io/gloo/install | sh
export PATH=$HOME/.gloo/bin:$PATH

## Windows
curl -sL https://run.solo.io/gloo/windows/install | pwsh
$env:Path += ";$env:userprofile/.gloo/bin/"

# Install Gloo Edge
glooctl install gateway
{{< /tab >}}
{{< tab name="Helm" codelang="shell">}}
# Add the Helm repository for Gloo Edge
helm repo add gloo https://storage.googleapis.com/solo-public-helm
helm repo update

# Create the namespace and install the Helm chart
kubectl create namespace gloo-system
helm install gloo gloo/gloo --namespace gloo-system
{{< /tab >}}
{{< /tabs >}}

That's it! With Gloo Edge installed you can try our [Hello World example]({{% versioned_link_path fromRoot="/guides/traffic_management/hello_world/" %}}) to get an idea of how Gloo Edge can be used.

For a more detailed guide on getting your local system configured, installing prerequisites, and deployment options check out our [Preparation]({{% versioned_link_path fromRoot="/installation/preparation/" %}}) doc.
