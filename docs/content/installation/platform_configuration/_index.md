---
title: Platform Configuration
weight: 20
---

Gloo Edge production deployments can be deployed using Kubernetes, HashiCorp products, or a combination of both. As detailed in the [Deployment Options]({{% versioned_link_path fromRoot="/introduction/architecture/deployment_options/" %}}) document, at a high level Gloo Edge is a collection of containers, configuration data, and secrets. 

![Component Architecture]({{% versioned_link_path fromRoot="/img/component_architecture.png" %}})

The following documents detail how to prepare a Kubernetes or HashiCorp environment for the deployment of Gloo Edge.

{{% children description="true" %}}