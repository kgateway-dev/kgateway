---
title: "Gloo Edge as an Ingress Controller"
description: How to install Gloo Edge to run in Ingress Mode on Kubernetes.
weight: 50
---

Gloo Edge can be used as a simple ingress controller on Kubernetes. This guide will take you through the process of deploying Gloo Edge as an ingress controller using either `glooctl` or Helm.

These directions assume you've prepared your Kubernetes cluster appropriately. Full details on setting up your Kubernetes cluster [here]({{< versioned_link_path fromRoot="/installation/platform_configuration/cluster_setup/" >}}).

## Installing on Kubernetes with `glooctl`

Before you begin, make sure that you [install `glooctl`]({{< versioned_link_path fromRoot="/installation/preparation/" >}}), the Gloo Edge command line tool (CLI).

Once your Kubernetes cluster is up and running, run the following command to deploy the Gloo Edge Ingress Controller to the `gloo-system` namespace:

```bash
glooctl install ingress
```

{{% notice note %}}
You can run the command with the flag `--dry-run` to output the Kubernetes manifests (as `yaml`) that `glooctl` will apply to the cluster instead of installing them.
{{% /notice %}}

---

## Installing on Kubernetes with Helm

This is the recommended method for installing Gloo Edge to your production environment as it offers rich customization to the Gloo Edge control plane and the proxies Gloo Edge manages. This guide assumes that you are using Helm version 3, and have already installed the Helm client on your local machine.

As a first step, you have to add the Gloo Edge repository to the list of known chart repositories and update the repository:

```shell
helm repo add gloo https://storage.googleapis.com/solo-public-helm
helm repo update
```

In the values for the Helm chart, you are going to set `gateway.enabled` to `false` and `ingress.enabled` to `true`. You can do this either by creating a `values.yaml` file with the proper settings or by defining the settings in line.

### Install using a values.yaml file

If you would like to define the settings with a file, create a `values.yaml` file with the following overrides:

```yaml
gateway:
  enabled: false
ingress:
  enabled: true
```

Then install Gloo Edge using the following command:

```shell
helm install gloo gloo/gloo --namespace gloo-system --create-namespace -f values.yaml
```

Gloo Edge can be installed to a namespace of your choosing with the `--namespace` flag.

#### Install using in-line settings

Instead of creating a `values.yaml` file, you can simply define the settings in-line. This is useful for a small number of values, but quickly becomes impractical if you want to override several values.

Run the following commands to install the Gloo Edge ingress controller.

```shell
helm install gloo gloo/gloo --namespace gloo-system --create-namespace \
  --set gateway.enabled=false,ingress.enabled=true
```

---

## Verify your Installation

Check that the Gloo Edge pods and services have been created. Depending on your install option, you may see some differences from the following example. And if you chose to install Gloo Edge into a different namespace than the default `gloo-system`, you will need to query your chosen namespace instead.

```shell
kubectl get all -n gloo-system
```

```noop
NAME                                       READY   STATUS    RESTARTS   AGE
pod/ingress-proxy-6d786fd9f-4k5r4          1/1     Running   0          64s
pod/discovery-55b8645d77-72mbt             1/1     Running   0          63s
pod/gloo-9f9f77c8d-6sk7z                   1/1     Running   0          64s
pod/ingress-85ffc7b77b-z6lsm               1/1     Running   0          64s

NAME                           TYPE           CLUSTER-IP     EXTERNAL-IP       PORT(S)                      AGE
service/ingress-proxy          LoadBalancer   10.7.250.225   35.226.24.166     80:32436/TCP,443:32667/TCP   64s
service/gloo                   ClusterIP      10.7.251.47    <none>            9977/TCP                     4d10h

NAME                                   DESIRED   CURRENT   UP-TO-DATE   AVAILABLE   AGE
deployment.apps/ingress-proxy          1         1         1            1           64s
deployment.apps/discovery              1         1         1            1           63s
deployment.apps/gloo                   1         1         1            1           64s
deployment.apps/ingress                1         1         1            1           64s

NAME                                             DESIRED   CURRENT   READY   AGE
replicaset.apps/ingress-proxy-6d786fd9f          1         1         1       64s
replicaset.apps/discovery-55b8645d77             1         1         1       63s
replicaset.apps/gloo-9f9f77c8d                   1         1         1       64s
replicaset.apps/ingress-85ffc7b77b               1         1         1       64s
```

---

## Uninstall {#uninstall}

To uninstall Gloo Edge and all related components, simply run the following.

```shell
glooctl uninstall
```

If you installed Gloo Edge to a different namespace, you will have to specify that namespace using the `-n` option:

```shell
glooctl uninstall -n my-namespace
```

## Next Steps

To begin using Gloo Edge with the Kubernetes Ingress API, check out the [Ingress Controller guide]({{< versioned_link_path fromRoot="/guides/integrations/ingress/" >}}).

{{< readfile file="static/content/upgrade-note.md" markdown="true">}}
