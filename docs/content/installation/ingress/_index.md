---
title: "Installing Gloo as an Ingress Controller"
description: How to install Gloo to run in Ingress Mode on Kubernetes.
weight: 4
---

## Installing the Gloo Ingress Controller on Kubernetes

These directions assume you've prepared your Kubernetes cluster appropriately. Full details on setting up your
Kubernetes cluster [here](../gateway/kubernetes/cluster_setup).

{{< readfile file="installation/glooctl_setup.md" markdown="true" >}}

### Installing on Kubernetes with `glooctl`

Once your Kubernetes cluster is up and running, run the following command to deploy the Gloo Ingress Controller to the `gloo-system` namespace:

```bash
glooctl install ingress
```

> Note: You can run the command with the flag `--dry-run` to output
the Kubernetes manifests (as `yaml`) that `glooctl` will
apply to the cluster instead of installing them.

### Installing on Kubernetes with Helm


This is the recommended method for installing Gloo to your production environment as it offers rich customization to
the Gloo control plane and the proxies Gloo manages.

As a first step, you have to add the Gloo repository to the list of known chart repositories:

```shell
helm repo add gloo https://storage.googleapis.com/solo-public-helm
```

Create a `values-ingress.yaml` file with the following overrides:
```yaml
gateway:
  enabled: false
ingress:
  enabled: true
```

Finally, install Gloo using the following command:

```shell
kubectl create namespace gloo-system
helm install gloo gloo/gloo --namespace gloo-system -f values-ingress.yaml
```

Gloo can be installed to a namespace of your choosing with the `--namespace` flag.

## Verify your Installation

Check that the Gloo pods and services have been created. Depending on your install option, you may see some differences
from the following example. And if you choose to install Gloo into a different namespace than the default `gloo-system`,
then you will need to query your chosen namespace instead.

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

To uninstall Gloo and all related components, simply run the following.

```shell
glooctl uninstall
```

If you installed Gloo to a different namespace, you will have to specify that namespace using the `-n` option:

```shell
glooctl uninstall -n my-namespace
```

## Next Steps

To begin using Gloo with the Kubernetes Ingress API, check out the [Ingress Controller guide]({{< versioned_link_path fromRoot="/gloo_integrations/ingress" >}}).
