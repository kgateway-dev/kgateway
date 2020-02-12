---
title: Gloo read-only UI
weight: 65
description: Gloo Read-only UI (open source & enterprise)
---


While the full Gloo UI remains an enterprise feature, open-source Gloo can be optionally installed with a read-only
version of the UI as a demo.

Additionally, Gloo Enterprise customers can also tighten the permissions of their Gloo UI, which can be helpful to
provide users insight into the state of Gloo without giving them admin-level permissions typically held by the Gloo UI
backend service.

## Install Gloo OSS Read-Only UI

Along with each release of Gloo Enterprise, we build and release a helm chart that deploys open-source Gloo alongside
a read-only version of the UI. The helm chart is located at [https://storage.googleapis.com/gloo-os-ui-helm](https://storage.googleapis.com/gloo-os-ui-helm).

To install this version with `glooctl`, use the `--with-admin-console` flag, for example:

```shell script
glooctl install gateway --with-admin-console
```

With helm, add the following repo:
```shell script
helm repo add glooe http://storage.googleapis.com/gloo-ee-helm
```

and install it:

```shell script
helm install gloo gloo/gloo-os-with-ui --namespace my-namespace
```

## Install Gloo Enterprise Read-Only UI

To install full Gloo Enterprise (including extauth, ratelimiting, enterprise-only Envoy filters) with the read-only UI,
install Gloo with the following helm value override:

```yaml
apiServer:
  enterprise: false
```