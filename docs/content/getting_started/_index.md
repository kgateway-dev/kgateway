---
title: Getting Started
weight: 5
---

We know you want to jump right in and start hacking away with Gloo Edge. That's awesome! If you want to try out Gloo Edge in a hosted setting, please check out our [Katacoda](https://katacoda.com/solo-io) courses that will guide you through a number of scenarios using Gloo Edge, all in a sandboxed environment.

If you'd rather use your own Kubernetes cluster, all you need to do is install Gloo Edge using `glooctl` or Helm.

{{< tabs >}}
{{< tab name="glooctl" codelang="shell">}}
# Install glooctl
curl -sL https://run.solo.io/gloo/install | sh
export PATH=$HOME/.gloo/bin:$PATH

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
