---
title: Installing Gloo Edge to Multiple Namespaces
weight: 20
description: Multi-tenant Gloo Edge installations by installing to multiple namespaces
---

In the default deployment scenario, a single deployment of the Gloo Edge control plane and Envoy proxy are installed for the entire cluster. However, in some cases, it may be desirable to deploy multiple instances of the Gloo Edge control plane and proxies in a single cluster.

This is useful when multiple tenants or applications want control over their own instance of Gloo Edge. Some deployment scenarios may involve a Gloo Edge per-application architecture. Additionally, different Gloo Edge instances living in their own namespace may be given different levels of RBAC permissions.

In this document, we will review how to deploy multiple instances of Gloo Edge to their own namespaces within a single Kubernetes cluster. 

---

## Scoping Gloo Edge to specific namespaces

When using the default installation, Gloo Edge will watch all namespaces for Kubernetes services and Gloo Edge CRDs. This means that any Kubernetes service can be a destination for any VirtualService in the cluster.

Gloo Edge can be configured to only watch specific namespaces, meaning Gloo Edge will not see services and CRDs in any namespaces other than those provided in the {{< protobuf name="gloo.solo.io.Settings" display="watchNamespaces setting">}}.

By leveraging this option, we can install Gloo Edge to as many namespaces we need, ensuring that the `watchNamespaces` do not overlap.

{{% notice note %}}
`watchNamespaces` can be shared between Gloo Edge instances, so long as any Virtual Services are not written to a shared namespace. When this happens, both Gloo Edge instances will attempt to apply the same routing config, which can cause domain conflicts.
{{% /notice %}}

Currently, installing Gloo Edge with specific `watchNamespaces` requires installation via the Helm chart.

---

## Installing Namespace-Scoped Gloo Edge with Helm

In this section we'll deploy Gloo Edge twice, each instance to a different namespace, with two different Helm value files.

Create a file named `gloo1-overrides.yaml` and paste the following inside:

{{< tabs >}}
{{< tab name="Helm 2" codelang="yaml" >}}
crds:
  create: true
settings:
  create: true
  writeNamespace: gloo1
  watchNamespaces:
  - default
  - gloo1
{{< /tab >}}
{{< tab name="Helm 3" codelang="yaml">}}
settings:
  create: true
  writeNamespace: gloo1
  watchNamespaces:
  - default
  - gloo1
{{< /tab >}}
{{< /tabs >}}

Now, let's install Gloo Edge. Review our [Kubernetes installation guide]({{% versioned_link_path fromRoot="/installation/gateway/kubernetes/" %}}) if you need a refresher.

First create the namespace for our first Gloo Edge deployment:

```shell script
kubectl create ns gloo1
```

Then install Gloo Edge using one of the following methods:

{{< tabs >}}
{{< tab name="glooctl" codelang="shell" >}}
glooctl install gateway -n gloo1 --values gloo1-overrides.yaml
{{< /tab >}}
{{% tab name="Helm 2" %}}
Either:

```shell script
helm install gloo/gloo --name gloo1 --namespace gloo1 -f gloo1-overrides.yaml
```

or:

```shell script
helm template gloo --namespace gloo1 --values gloo1-overrides.yaml  | k apply -f - -n gloo1
```
{{% /tab %}}
{{< tab name="Helm 3" codelang="shell">}}
helm install gloo gloo/gloo --namespace gloo1 -f gloo1-overrides.yaml
{{< /tab >}}
{{< /tabs >}}

Check that gloo pods are running: 

```bash
kubectl get pod -n gloo1
```

```bash
NAME                             READY   STATUS    RESTARTS   AGE
discovery-798cdd5499-z7rrt       1/1     Running   0          37s
gateway-5fc999b847-jf4xp         1/1     Running   0          32s
gateway-proxy-67f4c7dfb6-hc5kg   1/1     Running   0          27s
gloo-dd5bcdc8f-bvtjh             1/1     Running   0          39s
```

And we should see that Gloo Edge is only creating Upstreams from services in `default` and `gloo1`:

```bash
kubectl get us -n gloo1                                              
```

```bash
NAME                      AGE
default-kubernetes-443    1h
gloo1-gateway-proxy-443   1h
gloo1-gateway-proxy-80    1h
gloo1-gloo-9977           1h
```

Let's repeat the above process, substituting `gloo2` for `gloo1`:

Create a file named `gloo2-overrides.yaml` and paste the following inside:

{{< tabs >}}
{{< tab name="Helm 2" codelang="yaml" >}}
crds:
  create: true
settings:
  create: true
  writeNamespace: gloo2
  watchNamespaces:
  - default
  - gloo2
{{< /tab >}}
{{< tab name="Helm 3" codelang="yaml">}}
settings:
  create: true
  writeNamespace: gloo2
  watchNamespaces:
  - default
  - gloo2
{{< /tab >}}
{{< /tabs >}}

Now, let's install Gloo Edge for the second time. First create the second namespace:

```shell script
# create the namespace for our second gloo deployment
kubectl create ns gloo2
```

Then perform the second installation using one of the following methods:

{{< tabs >}}
{{< tab name="glooctl" codelang="shell" >}}
glooctl install gateway -n gloo2 --values gloo2-overrides.yaml
{{< /tab >}}
{{% tab name="Helm 2" %}}
Either:

```shell script
helm install gloo/gloo --name gloo2 --namespace gloo2 -f gloo2-overrides.yaml
```

or:

```shell script
helm template gloo --namespace gloo2 --values gloo2-overrides.yaml  | k apply -f - -n gloo2
```
{{% /tab %}}
{{< tab name="Helm 3" codelang="shell">}}
helm install gloo gloo/gloo --namespace gloo2 -f gloo2-overrides.yaml
{{< /tab >}}
{{< /tabs >}}

Check that gloo pods are running: 

```bash
kubectl get pod -n gloo2
```

```bash
NAME                             READY   STATUS    RESTARTS   AGE
discovery-798cdd5499-kzmkc       1/1     Running   0          8s
gateway-5fc999b847-pn2tk         1/1     Running   0          8s
gateway-proxy-67f4c7dfb6-284wv   1/1     Running   0          8s
gloo-dd5bcdc8f-krp5p             1/1     Running   0          9s
```

And we should see that the second installation of Gloo Edge is only creating Upstreams from services in `default` and `gloo2`:

```bash
kubectl get us -n gloo2
```

```bash
NAME                      AGE
default-kubernetes-443    53s
gloo2-gateway-proxy-443   53s
gloo2-gateway-proxy-80    53s
gloo2-gloo-9977           53s
```

And that's it! We can now create routes for Gloo Edge #1 by creating our Virtual Services in the `gloo1` namespace, and routes for Gloo Edge #2 by creating Virtual Services in the `gloo2` namespace. We can add `watchNamespaces` to our liking; the only catch is that a Virtual Service which lives in a shared namespace will be applied to both gateways (which can lead to undesired behavior if this was not the intended effect).

{{% notice warning %}}
When uninstalling a single instance of Gloo Edge when multiple instances are installed, you should only delete the namespace into which that instance is installed. Running `glooctl uninstall` can cause cluster-wide resources to be deleted, which will break any remaining Gloo Edge installation in your cluster
{{% /notice %}}