---
title: "Installing Gloo for Knative"
description: How to install Gloo to run in Knative Mode on Kubernetes.
weight: 3
---

## Motivation

For the purpose of running Knative, Gloo can function as a complete replacement for Istio (supporting all documented Knative features), requiring less resource usage and operational overhead. 

This guide walks you through installing Gloo and Knative using `glooctl`, the Gloo command line. 

{{% notice note %}}
`glooctl` generates a manifest which can be piped to stdout or a file using the `--dry-run` flag. Alternatively,
Gloo can be installed via its [Helm Chart]({{< versioned_link_path fromRoot="/installation/gateway/kubernetes#installing-on-kubernetes-with-helm" >}}), which will permit fine-grained configuration of installation parameters.
{{% /notice %}}



## Installing the Gloo Knative Ingress on Kubernetes

These directions assume you've prepared your Kubernetes cluster appropriately. Full details on setting up your
Kubernetes cluster [here](../gateway/kubernetes/cluster_setup).

{{< readfile file="installation/glooctl_setup.md" markdown="true" >}}

### Installing on Kubernetes with `glooctl`

`glooctl`, addition to installing Gloo's Knative Ingress, will install Knative Serving components to the `knative-serving` namespace if it does not alreay exist in your cluster. This is a modified version of the Knative Serving manifest with the dependencies on Istio removed.

Once your Kubernetes cluster is up and running, run the following command to deploy the Gloo Ingress to the `gloo-system` namespace and Knative-Serving components to the `knative-serving` namespace:

```bash
glooctl install knative
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

The Gloo chart archive contains the necessary value files for the Knative deployment option. Run the
following command to download and extract the archive to the current directory:

```shell
helm fetch --untar=true --untardir=. gloo/gloo
```

Finally, install Gloo using the following command:

```shell
helm install gloo --namespace gloo-system -f gloo/values-knative.yaml
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
NAME                                          READY   STATUS    RESTARTS   AGE
pod/discovery-7b6684f57d-ldcvx                1/1     Running   0          73m
pod/gloo-6658f49f64-9zlgh                     1/1     Running   5          73m
pod/ingress-5476d956c7-rrr6m                  1/1     Running   0          73m
pod/knative-external-proxy-fdfc894fb-6w4m9    1/1     Running   0          73m
pod/knative-internal-proxy-745c9f6f86-t7h5j   1/1     Running   0          73m

NAME                             TYPE           CLUSTER-IP     EXTERNAL-IP     PORT(S)                      AGE
service/gloo                     ClusterIP      10.7.246.232   <none>          9977/TCP                     73m
service/knative-external-proxy   LoadBalancer   10.7.247.23    35.188.41.169   80:30388/TCP,443:32060/TCP   73m
service/knative-internal-proxy   ClusterIP      10.7.243.248   <none>          80/TCP,443/TCP               73m

NAME                                     READY   UP-TO-DATE   AVAILABLE   AGE
deployment.apps/discovery                1/1     1            1           73m
deployment.apps/gloo                     1/1     1            1           73m
deployment.apps/ingress                  1/1     1            1           73m
deployment.apps/knative-external-proxy   1/1     1            1           73m
deployment.apps/knative-internal-proxy   1/1     1            1           73m

NAME                                                DESIRED   CURRENT   READY   AGE
replicaset.apps/discovery-7b6684f57d                1         1         1       73m
replicaset.apps/gloo-6658f49f64                     1         1         1       73m
replicaset.apps/ingress-5476d956c7                  1         1         1       73m
replicaset.apps/knative-external-proxy-fdfc894fb    1         1         1       73m
replicaset.apps/knative-internal-proxy-745c9f6f86   1         1         1       73m
```

---

## Uninstall {#uninstall}

To uninstall Gloo and all related components, simply run the following.

{{% notice note %}}
This will also remove Knative-Serving, if it was installed by `glooctl`.
{{% /notice %}}

```shell
glooctl uninstall
```

If you installed Gloo to a different namespace, you will have to specify that namespace using the `-n` option:

```shell
glooctl uninstall -n my-namespace
```

## Next Steps

To begin using Gloo with Knative, check out the [Knative User Guide]({{< versioned_link_path fromRoot="/gloo_integrations/knative" >}}).
