---
title: Setup
weight: 20
---

# Gloo Edge Open-Source

Gloo Edge Open-Source runs in 3 different modes to enable different use cases:

<div markdown=1>
<table>
  <tr height="100">
    <td width="10%">
      <a href="{{% versioned_link_path fromRoot="/installation/gateway/" %}}"><img src='{{% versioned_link_path fromRoot="/img/Gloo-01.png" %}}' width="60"/></a>
    </td>
    <td>
     Run Gloo Edge in <code>gateway</code> mode to function as an API Gateway. This is the most fully-featured and customizable installation of Gloo Edge, and is our <a href="{{% versioned_link_path fromRoot="/installation/gateway/" %}}"><b>recommended install for first-time users</b></a>. Gloo Edge can be configured via Kubernetes Custom Resources, Consul Key-Value storage, or <code>.yaml</code> files on Gloo Edge's local filesystem.
    </td>
  </tr>
  <tr height="100">
    <td width="10%">
      <a href="{{% versioned_link_path fromRoot="/installation/ingress/" %}}"><img src='{{% versioned_link_path fromRoot="/img/ingress.png" %}}' width="60"/></a>
    </td>
    <td>Run Gloo Edge in <code>ingress</code> mode to act as a standard Kubernetes Ingress controller. In this mode, Gloo Edge will import its configuration from the <code>networking.k8s.io/v1.Ingress</code> Kubernetes resource. This can be used to achieve compatibility with the standard Kubernetes ingress API. Note that Gloo Edge's Ingress API does not support customization via annotations. If you wish to extend Gloo Edge beyond the functions of basic ingress, it is recommended to run Gloo Edge in <code>gateway</code> mode.
    </td>
  </tr>
</table>
</div>

{{% notice note %}}
Note: The installation modes are not mutually exclusive, e.g. if you wish to run `gateway` in conjunction with `ingress`, it can be done by installing both options to the same (or different) namespaces.
{{% /notice %}}

# Gloo Edge Enterprise

Gloo Edge Enterprise has a single installation workflow:

<div markdown=1>
<table>
  <tr height="100">
    <td width="10%">
      <a href="{{% versioned_link_path fromRoot="/installation/enterprise/" %}}"><img src='{{% versioned_link_path fromRoot="/img/gloo-ee.png" %}}' width="60"/></a>
    </td>
    <td>
    Gloo Edge Enterprise is based on open-source Gloo Edge with additional (closed source) UI and plugins. See <a href="{{% versioned_link_path fromRoot="/installation/enterprise/" %}}">the Gloo Edge Enterprise documentation</a> for more details on the additional features of the Enterprise version of Gloo Edge.
    </td>
  </tr>
</table>
</div>
