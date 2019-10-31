---
title: "Installing Gloo Gateway on Kubernetes"
description: How to install Gloo to run in Gateway Mode on Kubernetes (Default).
weight: 2
---

## Installing the Gloo Gateway on Kubernetes

These directions assume you've prepared your Kubernetes cluster appropriately. Full details on setting up your
Kubernetes cluster [here](../../cluster_setup).

Note: For certain providers with more strict multi-tenant security, like OpenShift, be sure to follow the cluster set up accordingly. 

{{< readfile file="installation/glooctl_setup.md" markdown="true" >}}

### Installing on Kubernetes with `glooctl`

Once your Kubernetes cluster is up and running, run the following command to deploy the Gloo Gateway to the `gloo-system` namespace:

```bash
glooctl install gateway
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

Finally, install Gloo using the following command:

```shell
helm install gloo/gloo --namespace my-namespace
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

The table below describes all the values that you can override in your custom values file.
{{< readfile file="installation/gateway/kubernetes/values.txt" markdown="true" >}}

---
## Verify your Installation

Check that the Gloo pods and services have been created. Depending on your install option, you may see some differences
from the following example. And if you choose to install Gloo into a different namespace than the default `gloo-system`,
then you will need to query your chosen namespace instead.

```shell
kubectl get all -n gloo-system
```

```noop
NAME                                READY     STATUS    RESTARTS   AGE
pod/discovery-f7548d984-slddk       1/1       Running   0          5m
pod/gateway-5689fd59d7-wsg7f        1/1       Running   0          5m
pod/gateway-proxy-9d79d48cd-wg8b8   1/1       Running   0          5m
pod/gloo-5b7b748dbf-jdsvg           1/1       Running   0          5m

NAME                    TYPE           CLUSTER-IP      EXTERNAL-IP   PORT(S)          AGE
service/gateway-proxy   LoadBalancer   10.97.232.107   <pending>     8080:31800/TCP   5m
service/gloo            ClusterIP      10.100.64.166   <none>        9977/TCP         5m

NAME                            DESIRED   CURRENT   UP-TO-DATE   AVAILABLE   AGE
deployment.apps/discovery       1         1         1            1           5m
deployment.apps/gateway         1         1         1            1           5m
deployment.apps/gateway-proxy   1         1         1            1           5m
deployment.apps/gloo            1         1         1            1           5m

NAME                                      DESIRED   CURRENT   READY     AGE
replicaset.apps/discovery-f7548d984       1         1         1         5m
replicaset.apps/gateway-5689fd59d7        1         1         1         5m
replicaset.apps/gateway-proxy-9d79d48cd   1         1         1         5m
replicaset.apps/gloo-5b7b748dbf           1         1         1         5m
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

After you've installed Gloo, please check out our user guides on [Gloo Routing]({{< versioned_link_path fromRoot="/gloo_routing" >}}).
