---
title: "Helm Chart Values"
description: Listing of Helm chart values for the Gloo Edge chart
weight: 30
---

The table below describes all the values that you can override in your custom values file when working with the Helm 
chart for the Gloo Edge. More information on using a Helm chart to install the Gloo Edge can be found 
[here]({{% versioned_link_path fromRoot="/installation/gateway/kubernetes/#installing-on-kubernetes-with-helm" %}}).

{{% notice warning %}}
If you are using the **Gloo Edge Enterprise Helm** chart you will need to add a `gloo.` prefix to all the open source Gloo Edge 
chart values. This is due to the fact that the Gloo Edge Enterprise Helm chart uses the open source one as a dependency; 
therefore the sub-chart values have to be prefixed with the name of the sub-chart itself. 
This applies to all values except:

- `global.*`
- `settings.*`

For example, if you are installing Gloo Edge Enterprise:

- `crds.create` needs to be `gloo.crds.create`
- `gateway.certGenJob.enabled` needs to be `gloo.gateway.certGenJob.enabled`

but `settings.watchNamespaces` or `global.glooRbac.create` remain the same.
{{% /notice %}}

{{% notice note %}}
If you need further customization of the Helm chart, please read our [advanced customization guide]({{% versioned_link_path fromRoot="/installation/gateway/kubernetes/helm_advanced/" %}}).
{{% /notice %}}

{{< readfile file="reference/values.txt" markdown="true" >}}