---
title: Setup
weight: 20
---

Review the following setup paths for the open source and enterprise editions of Gloo Gateway.

# Open Source

Gloo Gateway Open Source Software (OSS) runs in `gateway` or `ingress` modes to enable different use cases.

{{% notice note %}}
Note: The installation modes are not mutually exclusive. To run both `gateway` in and `ingress` modes of Gloo Gateway OSS, install separate instances of Gloo Gateway in the same or different namespaces.
{{% /notice %}}

<div markdown=1>
<table>
  <tr height="100">
    <td width="10%">
      <a href="{{% versioned_link_path fromRoot="/installation/gateway/" %}}"><img src='{{% versioned_link_path fromRoot="/img/Gloo-01.png" %}}' width="60"/></a>
    </td>
    <td>
     Run Gloo Gateway in <code>gateway</code> mode to function as an API Gateway. This is the most fully-featured and customizable installation of Gloo Gateway, and is the <a href="{{% versioned_link_path fromRoot="/installation/gateway/" %}}"><b>recommended install for first-time users</b></a>. Gloo Gateway can be configured via Kubernetes Custom Resources, Consul Key-Value storage, or <code>.yaml</code> files on Gloo Gateway's local filesystem.
    </td>
  </tr>
  <tr height="100">
    <td width="10%">
      <a href="{{% versioned_link_path fromRoot="/installation/ingress/" %}}"><img src='{{% versioned_link_path fromRoot="/img/ingress.png" %}}' width="60"/></a>
    </td>
    <td>Run Gloo Gateway in <code>ingress</code> mode to act as a standard Kubernetes Ingress controller. In this mode, Gloo Gateway will import its configuration from the <code>networking.k8s.io/v1.Ingress</code> Kubernetes resource. This can be used to achieve compatibility with the standard Kubernetes ingress API. Note that Gloo Gateway's Ingress API does not support customization via annotations. If you wish to extend Gloo Gateway beyond the functions of basic ingress, it is recommended to run Gloo Gateway in <code>gateway</code> mode.
    </td>
  </tr>
</table>
</div>

## Enterprise

Gloo Gateway Enterprise Edition (EE) has a single installation workflow.

{{% notice note %}}
{{< readfile file="static/content/license-key" markdown="true">}}
{{% /notice %}}

<div markdown=1>
<table>
  <tr height="100">
    <td width="10%">
      <a href="{{% versioned_link_path fromRoot="/installation/enterprise/" %}}"><img src='{{% versioned_link_path fromRoot="/img/gloo-ee.png" %}}' width="60"/></a>
    </td>
    <td>
    Gloo Gateway Enterprise is based on open-source Gloo Gateway with additional, closed source features. For a comparison between open source and enterprise editions, see the <a href="{{% versioned_link_path fromRoot="/introduction/faq/#oss-enterprise" %}}">FAQs</a>. For installation instructions, see the <a href="{{% versioned_link_path fromRoot="/installation/enterprise/" %}}">Enterprise setup guide</a>.
    </td>
  </tr>
</table>
</div>
