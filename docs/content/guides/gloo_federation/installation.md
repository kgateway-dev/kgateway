---
title: Installation
description: How and where to deploy Gloo Gateway Federation
weight: 15
---

Learn how and where to deploy Gloo Gateway Federation.

## Deployment topology

Gloo Gateway Federation is primarily an additional Kubernetes controller that runs alongside Gloo Gateway controllers. It is composed of Kubernetes Custom Resource Definitions (CRDs) and a controller pod that watches the custom resources and executes actions. 

The controller deployment and CRDs are created in an "administrative" cluster. Note that the Gloo Gateway Federation controller can also be deployed in an existing cluster that is already running Gloo Gateway.

{{< tabs >}}
{{% tab name="Dedicated Admin cluster" %}}
![Figure of Gloo Fed architecture in a dedicated admin cluster]({{% versioned_link_path fromRoot="/img/gloo-fed-arch-admin-cluster.png" %}})
{{% /tab %}}
{{% tab name="Shared cluster" %}}
![Figure of Gloo Fed architecture in a shared admin cluster]({{% versioned_link_path fromRoot="/img/gloo-fed-arch-shared-cluster.png" %}})
{{% /tab %}}
{{< /tabs >}}


## Gloo Gateway Federation deployment

You can choose between two standard deployment models for Gloo Gateway Federation:
1. Alongside Gloo Gateway Enterprise
2. Standalone mode

### Option 1: Alongside Gloo Gateway Enterprise

By default, Gloo Gateway Federation is installed alongside Gloo Gateway Enterprise.

After installation, check that the following deployments are ready.

```
kubectl -n gloo-system get deploy
NAME                                  READY   UP-TO-DATE   AVAILABLE   AGE
gloo-fed                              1/1     1            1           130m
gloo-fed-console                      1/1     1            1           130m
```

If you can't see these deployments, install or upgrade the Gloo Gateway Helm chart with the following Helm values.

```yaml
gloo-fed:
  enabled: true
```

### Option 2: Standalone mode

To deploy Gloo Gateway Federation in a standalone mode, you must install the Gloo Gateway Federation Helm chart.
```shell
helm install gloo-fed gloo-fed/gloo-fed --version $GLOO_VERSION --set license_key=$LICENSE_KEY -n gloo-system --create-namespace
```

Next, [register]({{% versioned_link_path fromRoot="/guides/gloo_federation/cluster_registration/" %}}) the Gloo Gateway instances.
