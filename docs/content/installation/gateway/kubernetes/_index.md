---
title: "Installing Gloo Edge on Kubernetes"
menuTitle: "Kubernetes"
description: How to install Gloo Edge to run in Gateway Mode on Kubernetes (Default).
weight: 10
---

Gloo Edge can be installed on a Kubernetes cluster by using either the [`glooctl` command line tool](#installing-on-kubernetes-with-glooctl) or a [Helm chart](#installing-on-kubernetes-with-helm). The following document will take you through the process of either installation, [verifying the installation](#verify-your-installation), and [how to remove Gloo Edge](#uninstall) if necessary.

{{% notice note %}}
Minimum required Kubernetes is 1.11.x. For older versions see our [release support guide]({{% versioned_link_path fromRoot="/reference/support/#kubernetes" %}})
{{% /notice %}}

---

## Before you begin

1. Make sure that you prepared your Kubernetes cluster according to the [instructions for platform configuration]({{% versioned_link_path fromRoot="/installation/platform_configuration/cluster_setup/" %}}).
   {{% notice note %}}
   Pay attention to provider-specific information in the setup guide. For example, [OpenShift]({{< versioned_link_path fromRoot="/installation/platform_configuration/cluster_setup/#openshift" >}}) requires stricter multi-tenant support, so the setup guide includes an example Helm chart `values.yaml` file that you must supply while installing Gloo Edge Enterprise.
   {{% /notice %}}
2. Get your Gloo Edge Enterprise license key. If you don't have one already, you may request a trial license key [here](https://www.solo.io/products/gloo/#enterprise-trial).
   {{% notice info %}}
   You must provide the license key during the installation process. When you install Gloo Edge, a Kubernetes secret is created to store the license key. Note that each trial license key is typically valid for **30 days**. When the license key expires, you can request a new license key by contacting your Account Representative or filling out [this form](https://lp.solo.io/request-trial). For more information, see [Updating Enterprise Licenses]({{< versioned_link_path fromRoot="/operations/updating_license/" >}}).
   {{% /notice %}}
3. Install or upgrade `glooctl` with the following instructions.

{{< readfile file="installation/glooctl_setup.md" markdown="true" >}}

## Installing Gloo Edge on Kubernetes

Review the following steps to install Gloo Edge with `glooctl` or with Helm.

### Installing on Kubernetes with `glooctl`

Once your Kubernetes cluster is up and running, run the following command to deploy Gloo Edge to the `gloo-system` namespace:

```bash
glooctl install gateway
```

<details>
<summary>Special Instructions to Install Gloo Edge on Kind</summary>
If you followed the cluster setup instructions for Kind <a href="{{< versioned_link_path fromRoot="/installation/platform_configuration/cluster_setup/#kind" >}}">here</a>, then you should have exposed custom ports 31500 (for http) and 32500 (https) from your cluster's Docker container to its host machine. The purpose of this is to make it easier to access your service endpoints from your host workstation.  Use the following custom installation for Gloo Edge to publish those same ports from the proxy as well.

```bash
cat <<EOF | glooctl install gateway --values -
gatewayProxies:
  gatewayProxy:
    service:
      type: NodePort
      httpPort: 31500
      httpsPort: 32500
      httpNodePort: 31500
      httpsNodePort: 32500
EOF
```

```
Creating namespace gloo-system... Done.
Starting Gloo Edge installation...

Gloo Edge was successfully installed!
```

Note also that the url to invoke services published via Gloo Edge will be slightly different with Kind-hosted clusters.  Much of the Gloo Edge documentation instructs you to use `$(glooctl proxy url)` as the header for your service url.  This will not work with kind.  For example, instead of using curl commands like this:

```bash
curl $(glooctl proxy url)/all-pets
```

You will instead route your request to the custom port that you configured above for your docker container to publish. For example:

```bash
curl http://localhost:31500/all-pets
```
</details>

**Video with example output**

<video controls loop>
  <source src="https://solo-docs.s3.us-east-2.amazonaws.com/gloo/videos/glooctl-gateway-install.mp4" type="video/mp4">
</video>

Once you've installed Gloo Edge, please be sure [to verify your installation]({{% versioned_link_path fromRoot="/installation/gateway/kubernetes/#verify-your-installation" %}}).


{{% notice note %}}
You can run the command with the flag `--dry-run` to output the Kubernetes manifests (as `yaml`) that `glooctl` will apply to the cluster instead of installing them.
Note that a proper Gloo Edge installation depends on Helm Chart Hooks (https://helm.sh/docs/topics/charts_hooks/), so the behavior of your installation
may not be correct if you install by directly applying the dry run manifests, e.g. `glooctl install gateway --dry-run | kubectl apply -f -`.
{{% /notice %}}

### Installing on Kubernetes with Helm

{{% notice warning %}}
Using Helm 2 is not supported in Gloo Edge v1.8.0.
{{% /notice %}}

As a first step, you have to add the Gloo Edge repository to the list of known chart repositories, as well as prepare the installation namespace:

```shell
helm repo add gloo https://storage.googleapis.com/solo-public-helm
helm repo update
kubectl create namespace my-namespace
```

For an installation with all the default values, use one of the following commands:

```shell
helm install gloo gloo/gloo --namespace my-namespace
```

Once you've installed Gloo Edge, please be sure [to verify your installation]({{% versioned_link_path fromRoot="/installation/gateway/kubernetes/#verify-your-installation" %}}).

<br>

#### Customizing your installation with Helm

You can customize the Gloo Edge installation by providing your own Helm chart values file.

For example, you can create a file named `value-overrides.yaml` with the following content.

```yaml
global:
  glooRbac:
    # do not create kubernetes rbac resources
    create: false
settings:
  # configure gloo to write generated custom resources to a custom namespace
  writeNamespace: my-custom-namespace
```

Then, refer to the file during installation to override default values in the Gloo Edge Helm chart.

```shell
helm install gloo-custom-0-7-6 gloo/gloo --namespace my-namespace -f value-overrides.yaml
```

#### List of Gloo Edge Helm chart values

The [Helm Chart Values page]({{< versioned_link_path fromRoot="/reference/helm_chart_values/" >}}) describes all the values that you can override in your custom values file.

---

## Verify your Installation

To verify that your installation was successful, check that the Gloo Edge pods and services have been created. Depending on your install options, you may see some differences from the following example. If you choose to install Gloo Edge into a namespace other than the default `gloo-system`, you will need to query your chosen namespace instead.

```shell
kubectl get all -n gloo-system
```

<video controls loop>
  <source src="https://solo-docs.s3.us-east-2.amazonaws.com/gloo/videos/kubectl-get-all.mp4" type="video/mp4">
</video>

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
#### Looking for opened ports?
You will NOT have any open ports listening on a default install. For Envoy to open the ports and actually listen, you need to have a Route defined in one of the VirtualServices that will be associated with that particular Gateway/Listener. Please see the [Hello World tutorial to get started]({{% versioned_link_path fromRoot="/guides/traffic_management/hello_world/" %}}). 

{{% notice note %}}
NOT opening the listener ports when there are no listeners (routes) is by design with the intention of not over-exposing your cluster by accident (for security). If you feel this behavior is not justified, please let us know.
{{% /notice %}}

---

## Uninstall {#uninstall}

To uninstall Gloo Edge, you can use the `glooctl` CLI. If you installed Gloo Edge to a different namespace, include the `-n` option.

```shell
glooctl uninstall -n my-namespace
```

By default, the `gloo-system` namespace and Custom Resource Definitions created by the `glooctl install` command are not removed. To remove the namespace and CRDs, include the `--all` option. 

{{% notice warning %}}
Make sure that your cluster has no other instances of Gloo Edge running, such as by running `kubectl get pods --all-namespaces`. If you remove the CRDs while Gloo Edge is still installed, you will experience errors.
{{% /notice %}}

```shell
glooctl uninstall --all
```

### Uninstall with `glooctl` video

<video controls loop>
  <source src="https://solo-docs.s3.us-east-2.amazonaws.com/gloo/videos/glooctl-uninstall.mp4" type="video/mp4">
</video>

---

## Next Steps

After you've installed Gloo Edge, please check out our user guides on [Traffic Management]({{< versioned_link_path fromRoot="/guides/traffic_management/" >}}).

{{< readfile file="static/content/upgrade-crd.md" markdown="true">}}