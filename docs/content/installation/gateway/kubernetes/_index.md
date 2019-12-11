---
title: "Installing Gloo Gateway on Kubernetes"
description: How to install Gloo to run in Gateway Mode on Kubernetes (Default).
weight: 2
---

Gloo Gateway can be installed on a Kubernetes cluster by using either the [`glooctl` command line tool](#installing-on-kubernetes-with-glooctl) or a [Helm chart](#installing-on-kubernetes-with-helm). The following document will take you through the process of either installation, [verifying the installation](#verify-your-installation), and [how to remove Gloo Gateway](#uninstall) if necessary.

---

## Installing the Gloo Gateway on Kubernetes

These directions assume you've prepared your Kubernetes cluster appropriately. Full details on setting up your
Kubernetes cluster [here](./cluster_setup).

{{% notice note %}}
For certain providers with more strict multi-tenant security, like OpenShift, be sure to follow the cluster set up accordingly. 
{{% /notice %}}

{{< readfile file="installation/glooctl_setup.md" markdown="true" >}}

### Installing on Kubernetes with `glooctl`

Once your Kubernetes cluster is up and running, run the following command to deploy the Gloo Gateway to the `gloo-system` namespace:

```bash
glooctl install gateway
```

{{% notice note %}}
You can run the command with the flag `--dry-run` to output the Kubernetes manifests (as `yaml`) that `glooctl` will apply to the cluster instead of installing them.
{{% /notice %}}

### Installing on Kubernetes with Helm

This is the recommended method for installing Gloo to your production environment as it offers rich customization to the Gloo control plane and the proxies Gloo manages.

As a first step, you have to add the Gloo repository to the list of known chart repositories:

```shell
helm repo add gloo https://storage.googleapis.com/solo-public-helm
helm repo update
```

For an installation with all the default values, install Gloo using the following command:

```shell
kubectl create namespace my-namespace
helm install gloo-gateway gloo/gloo --namespace my-namespace
```

#### Customizing your installation with Helm

You can customize the Gloo installation by providing your own value file.

For example, you can create a file named `value-overrides.yaml` with the following content:

```yaml
global:
  glooRbac:
    # do not create kubernetes rbac resources
    create: false
settings:
  # configure gloo to write generated custom resources to a custom namespace
  writeNamespace: my-custom-namespace
```

and use it to override default values in the Gloo Helm chart:

```shell
helm install gloo/gloo --name gloo-custom-0-7-6 --namespace my-namespace -f value-overrides.yaml
```

#### List of Gloo Helm chart values

The [Helm Chart Values page](./helm_chart_values) describes all the values that you can override in your custom values file.

---

## Verify your Installation

To verify that your installation was successful, check that the Gloo pods and services have been created. Depending on your install options, you may see some differences from the following example. If you choose to install Gloo into a namespace other than the default `gloo-system`, you will need to query your chosen namespace instead.

```shell
kubectl get all -n gloo-system
```

```noop
NAME                                READY     STATUS    RESTARTS   AGE
pod/discovery-f7548d984-slddk       1/1       Running   0          5m
pod/gateway-5689fd59d7-wsg7f        1/1       Running   0          5m
pod/gateway-proxy-9d79d48cd-wg8b8   1/1       Running   0          5m
pod/gloo-5b7b748dbf-jdsvg           1/1       Running   0          5m

NAME                    TYPE           CLUSTER-IP      EXTERNAL-IP   PORT(S)                     AGE
service/gateway         ClusterIP      10.0.180.15     <none>        443/TCP                     5m
service/gateway-proxy   LoadBalancer   10.97.232.107   <pending>     80:30221/TCP,443:32340/TCP  5m
service/gloo            ClusterIP      10.100.64.166   <none>        9977/TCP,9988/TCP,9966/TCP  5m

NAME                            READY   UP-TO-DATE   AVAILABLE   AGE
deployment.apps/discovery       1/1     1            1           5m
deployment.apps/gateway         1/1     1            1           5m
deployment.apps/gateway-proxy   1/1     1            1           5m
deployment.apps/gloo            1/1     1            1           5m

NAME                                      DESIRED   CURRENT   READY     AGE
replicaset.apps/discovery-f7548d984       1         1         1         5m
replicaset.apps/gateway-5689fd59d7        1         1         1         5m
replicaset.apps/gateway-proxy-9d79d48cd   1         1         1         5m
replicaset.apps/gloo-5b7b748dbf           1         1         1         5m

NAME                        COMPLETIONS   DURATION   AGE
job.batch/gateway-certgen   1/1           14s        5m
```

---

## Uninstall {#uninstall}

It's so hard to say goodbye. Actually, in this case it's not. 

### Uninstall with `glooctl`

To uninstall Gloo and all related components, simply run the following.

```shell
glooctl uninstall
```

If you installed Gloo to a different namespace, you will have to specify that namespace using the `-n` option:

```shell
glooctl uninstall -n my-namespace
```

The gloo-system namespace and Custom Resource Definitions created by the `glooctl install` command will not be removed. Those can also be deleted by running the following commands. Proceed with caution and only remove the CRDs if there are no more instances of Gloo Gateway in the cluster.

```shell
kubectl delete namespace gloo-system
crds=$(kubectl get crds -o json | jq .items[].metadata.name -r | grep solo.io)
for n in $crds
do
  echo "Removing $n from CRDs"
  kubectl delete customresourcedefinition $n
done
```

---

## Next Steps

After you've installed Gloo, please check out our user guides on [Gloo Routing]({{< versioned_link_path fromRoot="/gloo_routing" >}}).
