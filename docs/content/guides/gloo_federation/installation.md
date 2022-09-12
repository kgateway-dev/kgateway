---
title: Installation
description: How and where to deploy Gloo Edge Federation
weight: 15
---

## Deployment topology


Gloo Edge Federation is primarily an additional Kubernetes controller running alongside Gloo Edge controllers. It is composed of Kubernetes Custom Resource Definitions (CRDs) and a controller pod that watches the custom resources and executes actions. 

The controller deployment and CRDs are created in an "administrative" cluster. Note that the Gloo Edge Federation controller can also be deployed in an existing cluster that is already running Gloo Edge.

{{< tabs >}}
{{% tab name="Dedicated Admin cluster" %}}
![]({{% versioned_link_path fromRoot="/img/gloo-fed-arch-admin-cluster.png" %}})
{{% /tab %}}
{{% tab name="Shared cluster" %}}
![]({{% versioned_link_path fromRoot="/img/gloo-fed-arch-shared-cluster.png" %}})
{{% /tab %}}
{{< /tabs >}}


## Gloo Edge Federation deployment
**Option A**: by default, Gloo Edge Federation is installed along with Gloo Edge Enterprise.

Once deployed, the following deployments should be visible and ready:
```
kubectl -n gloo-system get deploy
NAME                                  READY   UP-TO-DATE   AVAILABLE   AGE
gloo-fed                              1/1     1            1           130m
gloo-fed-console                      1/1     1            1           130m
```

If you can't see these deployments, you have to install / upgrade the Gloo Edge Helm chart with the following Helm values:
```yaml
gloo-fed:
  enabled: true
```

**Option B**: deploy Gloo Edge Federation in a standalone mode.
In this case, you must install the Gloo Edge Federation Helm chart:
```shell
helm install gloo-fed gloo-fed/gloo-fed --version $GLOO_VERSION --set license_key=$LICENSE_KEY -n gloo-system --create-namespace
```

Next step is [registering]({{% versioned_link_path fromRoot="/guides/gloo_federation/cluster_registration/" %}}) the Gloo Edge instances.