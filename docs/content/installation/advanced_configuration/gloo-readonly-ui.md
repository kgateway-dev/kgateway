---
title: Gloo Edge read-only UI
weight: 65
description: Gloo Edge Read-only UI (open source & enterprise)
---


While the full Gloo Edge UI remains an enterprise feature, open-source Gloo Edge can be optionally installed with a read-only
version of the UI as a demo.

Additionally, Gloo Edge Enterprise customers can also tighten the permissions of their Gloo Edge UI, which can be helpful to
provide users insight into the state of Gloo Edge without giving them admin-level permissions typically held by the Gloo Edge UI
backend service.

Once installed, access the read-only UI with `glooctl ui` or `glooctl dashboard`.

## Install Gloo Edge OSS Read-Only UI (Deprecated in 1.7.0)

Along with each release of Gloo Edge Enterprise, we build and release a helm chart that deploys open-source Gloo Edge alongside
a read-only version of the UI. The helm chart is located at [https://storage.googleapis.com/gloo-os-ui-helm](https://storage.googleapis.com/gloo-os-ui-helm).

To install this version with `glooctl`, use the `--with-admin-console` flag, for example:

```shell script
glooctl install gateway --with-admin-console
```

With helm, add the following repo:
```shell script
helm repo add gloo-os-with-ui https://storage.googleapis.com/gloo-os-ui-helm
```

and install it:

{{< tabs >}}
{{% tab name="Helm 3" %}}
```shell script
helm install gloo gloo-os-with-ui/gloo-os-with-ui --namespace gloo-system
```
{{< /tab >}}
{{< tab name="Helm 2" codelang="shell">}}
helm install --name gloo gloo-os-with-ui/gloo-os-with-ui --namespace my-namespace --set crds.create=true
{{< /tab >}}
{{< /tabs >}}


## Install Gloo Edge Enterprise Read-Only UI

To install full Gloo Edge Enterprise (including extauth, ratelimiting, Envoy with enterprise-only Envoy filters) with the
read-only UI, install Gloo Edge with the following helm value override:

```yaml
# This makes Gloo Edge install the UI in read-only mode
apiServer:
  enterprise: false
```