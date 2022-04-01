---
title: "Installing Gloo Edge Enterprise"
menuTitle: Gloo Edge Enterprise
description: How to install Gloo Edge to run in Gateway Mode on Kubernetes (Default).
weight: 60
---

Review how to install Gloo Edge Enterprise.
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

## Installing Gloo Edge Enterprise on Kubernetes {#install-steps}

Review the following steps to install Gloo Edge Enterprise with `glooctl` or with Helm.

### Installing on Kubernetes with `glooctl`

Once your Kubernetes cluster is up and running, run the following command to deploy the Gloo Edge to the `gloo-system` namespace:

```bash
glooctl install gateway enterprise --license-key YOUR_LICENSE_KEY
```

{{% notice note %}}
For OpenShift clusters, make sure to include the `--values values.yaml` option to point to the [Helm chart custom values file]({{< versioned_link_path fromRoot="/installation/platform_configuration/cluster_setup/#openshift" >}}) that you created.
{{% /notice %}}

<details>
<summary>Special Instructions to Install Gloo Edge Enterprise on Kind</summary>
If you followed the cluster setup instructions for Kind <a href="{{< versioned_link_path fromRoot="/installation/platform_configuration/cluster_setup/#kind" >}}">here</a>, then you should have exposed custom ports 31500 (for http) and 32500 (https) from your cluster's Docker container to its host machine. The purpose of this is to make it easier to access your service endpoints from your host workstation.  Use the following custom installation for Gloo Edge to publish those same ports from the proxy as well.

```bash
cat <<EOF | glooctl install gateway enterprise --license-key YOUR_LICENSE_KEY --values -
gloo:
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
Starting Gloo Edge Enterprise installation...

Gloo Edge Enterprise was successfully installed!
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

Once you've installed Gloo Edge, please be sure [to verify your installation](#verify-your-installation).


{{% notice note %}}
You can run the command with the flag `--dry-run` to output 
the Kubernetes manifests (as `yaml`) that `glooctl` will 
apply to the cluster instead of installing them.
{{% /notice %}}

### Installing on Kubernetes with Helm

This is the recommended method for installing Gloo Edge Enterprise to your production environment as it offers rich customization to
the Gloo Edge control plane and the proxies Gloo Edge manages.

As a first step, you have to add the Gloo Edge repository to the list of known chart repositories:

```shell
helm repo add glooe https://storage.googleapis.com/gloo-ee-helm
```

Finally, install Gloo Edge using the following command:

```shell
helm install gloo glooe/gloo-ee --namespace gloo-system \
  --create-namespace --set-string license_key=YOUR_LICENSE_KEY
```

{{% notice note %}}
For OpenShift clusters, make sure to include the `--values values.yaml` option to point to the [Helm chart custom values file]({{< versioned_link_path fromRoot="/installation/platform_configuration/cluster_setup/#openshift" >}}) that you created.
{{% /notice %}}

{{% notice warning %}}
Using Helm 2 is not supported in Gloo Edge v1.8.0 or later.
{{% /notice %}}

Once you've installed Gloo Edge, please be sure [to verify your installation](#verify-your-installation).

### Airgap installation

You can install Gloo Edge Enterprise in an air-gapped environment, such as an on-premises datacenter, clusters that run on an intranet or private network only, or other disconnected environments.

Before you begin, make sure that you have the following setup:
* A connected device that can pull the required images from the internet.
* An air-gapped or disconnected device that you want to install Gloo Edge Enterprise in.
* A private image registry such as Sonatype Nexus Repository or JFrog Artifactory that both the connected and disconnected devices can connect to.

To install Gloo Edge Enterprise in an air-gapped environment:

1. Set the Gloo Edge Enterprise version that you want to use as an environment variable, such as the latest version in the following example.
   ```shell
   export GLOO_EE_VERSION={{< readfile file="static/content/version_gee_latest.md" markdown="true">}}
   ```
2. On the connected device, download the Gloo Edge Enterprise images.
   ```shell
   helm template glooe/gloo-ee --version $GLOO_EE_VERSION | yq e '. | .. | select(has("image"))' - | grep image: | sed 's/image: //'
   ```
   
   The example output includes the list of images.
   ```
   quay.io/solo-io/gloo-fed-apiserver:{{< readfile file="static/content/version_gee_latest.md" markdown="true">}}
   quay.io/solo-io/gloo-federation-console:{{< readfile file="static/content/version_gee_latest.md" markdown="true">}}
   quay.io/solo-io/gloo-fed-apiserver-envoy:{{< readfile file="static/content/version_gee_latest.md" markdown="true">}}
   quay.io/solo-io/gloo-fed:{{< readfile file="static/content/version_gee_latest.md" markdown="true">}}
   quay.io/solo-io/gloo-ee:{{< readfile file="static/content/version_gee_latest.md" markdown="true">}}
   quay.io/solo-io/discovery-ee:{{< readfile file="static/content/version_gee_latest.md" markdown="true">}}
   quay.io/solo-io/gateway:{{< readfile file="static/content/version_geoss_latest.md" markdown="true">}}
   quay.io/solo-io/gloo-ee-envoy-wrapper:{{< readfile file="static/content/version_gee_latest.md" markdown="true">}}
   "grafana/grafana:8.2.1"
   "quay.io/coreos/kube-state-metrics:v1.9.7"
   "jimmidyson/configmap-reload:v0.5.0"
   "quay.io/prometheus/prometheus:v2.24.0"
   docker.io/busybox:1.28
   docker.io/redis:6.2.4
   quay.io/solo-io/rate-limit-ee:{{< readfile file="static/content/version_gee_latest.md" markdown="true">}}
   quay.io/solo-io/extauth-ee:{{< readfile file="static/content/version_gee_latest.md" markdown="true">}}
   quay.io/solo-io/observability-ee:{{< readfile file="static/content/version_gee_latest.md" markdown="true">}}
   quay.io/solo-io/certgen:{{< readfile file="static/content/version_geoss_latest.md" markdown="true">}}
    ```

3. Push the images from the connected device to a private registry that the disconnected device can pull from. For instructions and any credentials you must set up to complete this step, consult your registry provider, such as [Nexus Repository Manager](https://help.sonatype.com/repomanager3/formats/docker-registry/pushing-images) or [JFrog Artifactory](https://www.jfrog.com/confluence/display/JFROG/Getting+Started+with+Artifactory+as+a+Docker+Registry).
4. Optional: You might want to set up your private registry so that you can also pull the Helm charts. For instructions, consult your registry provider, such as [Nexus Repository Manager](https://help.sonatype.com/repomanager3/formats/helm-repositories) or [JFrog Artifactory](https://www.jfrog.com/confluence/display/JFROG/Kubernetes+Helm+Chart+Repositories).
5. When you [install Gloo Edge Enterprise with a custom Helm chart values file](#customizing-your-installation-with-helm), make sure to use the specific images that you downloaded and stored in your private registry in the previous steps.

## Customizing your installation with Helm

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
helm install gloo glooe/gloo-ee --namespace gloo-system \
  -f value-overrides.yaml --create-namespace --set-string license_key=YOUR_LICENSE_KEY
```

{{% notice warning %}}
Using Helm 2 is not supported in Gloo Edge v1.8.0 or later.
{{% /notice %}}

### List of Gloo Edge Helm chart values

The following table describes the most important enterprise-only values that you can override in your custom values file.

For more information, see the following resources:
* [Gloo Edge Open Source overrides]({{< versioned_link_path fromRoot="/reference/helm_chart_values/" >}}) (also available in Enterprise). 
* [Advanced customization guide]({{% versioned_link_path fromRoot="/installation/gateway/kubernetes/helm_advanced/" %}}).
* [Enterprise Helm chart reference document]({{% versioned_link_path fromRoot="/reference/helm_chart_values/enterprise_helm_chart_values/" %}}).

{{% notice note %}}
Gloo Edge Open Source Helm values in Enterprise must be prefixed with `gloo`, unless they are the Gloo Edge settings, such as `settings.<rest of helm value>`.
{{% /notice %}}

| option                                                    | type     | description                                                                                                                                                                                                                                                    |
| --------------------------------------------------------- | -------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| grafana.defaultInstallationEnabled                        | bool     | deploy grafana in your gloo system namespace. default is `true` |
| prometheus.enabled                                        | bool     | deploy prometheus in your gloo system namespace. default is `true` |
| rateLimit.enabled                                         | bool     | deploy rate-limiting in your gloo system namespace. default is `true` |
| global.extensions.extAuth.enabled                         | bool     | deploy ext-auth in your gloo system namespace. default is `true` |
| global.extensions.extAuth.envoySidecar                    | bool     | deploy ext-auth in the gateway-proxy pod, as a sidecar to envoy. communicates over unix domain socket instead of TCP. default is `false` |
| observability.enabled                                     | bool     | deploy observability in your gloo system namespace. default is `true` |
| observability.customGrafana.enabled                       | bool     | indicate you'll be using your own instance of grafana rather than the one shipped with Gloo Edge. default is `false`
| observability.customGrafana.username                      | string   | set this and the `password` field to authenticate to the custom grafana instance using basic auth
| observability.customGrafana.password                      | string   | set this and the `username` field to authenticate to the custom grafana instance using basic auth
| observability.customGrafana.apiKey                        | string   | authenticate to the custom grafana instance using this api key
| observability.customGrafana.url                           | string   | the URL for the custom grafana instance
---

## Enterprise UI

For version 1.8.9 and later, Gloo Edge Enterprise includes the user interface (UI) by default. Prior versions have the UI only when you enable Gloo Federation.

Note that when you enable Gloo Federation in version 1.8.9 or later, the UI does not show any data until you [register one or more clusters]({{< versioned_link_path fromRoot="/guides/gloo_federation/cluster_registration/" >}}). If you do not use Gloo Federation, the UI shows the installed Gloo Edge instance automatically without cluster registration.

To disable Gloo Federation, you can set `gloo-fed.enabled=false` during installation as shown in the following examples.

{{< tabs >}}
{{% tab name="glooctl install" %}}
```shell script
echo "gloo-fed:
  enabled: false" > values.yaml
glooctl install gateway enterprise --values values.yaml --license-key=<LICENSE_KEY>
```
{{% /tab %}}
{{% tab name="helm install" %}}
```shell script
helm install gloo glooe/gloo-ee --namespace gloo-system --set gloo-fed.enabled=false --set license_key=<LICENSE_KEY>
```
{{% /tab %}}
{{< /tabs >}}




## Verify your Installation

Check that the Gloo Edge pods and services have been created. Depending on your install option, you may see some differences
from the following example. And if you choose to install Gloo Edge into a different namespace than the default `gloo-system`,
then you will need to query your chosen namespace instead.

```shell
kubectl --namespace gloo-system get all
```

```noop
NAME                                                       READY   STATUS    RESTARTS   AGE
pod/api-server-56fcb78878-d9mxt                            2/2     Running   0          5m21s
pod/discovery-759bd6cf85-sphjb                             1/1     Running   0          5m22s
pod/extauth-679d587db8-l9k56                               1/1     Running   0          5m21s
pod/gateway-568bfd477c-487zw                               1/1     Running   0          5m22s
pod/gateway-proxy-c84cbd647-n9kz2                          1/1     Running   0          5m22s
pod/gloo-6979c5bd8-2dfrj                                   1/1     Running   0          5m22s
pod/glooe-grafana-86445b465b-mnn8t                         1/1     Running   0          5m22s
pod/glooe-prometheus-kube-state-metrics-8587f58df6-954pw   1/1     Running   0          5m22s
pod/glooe-prometheus-server-6bd6f4667d-zqffp               2/2     Running   0          5m21s
pod/observability-6db6c659dd-v4bkp                         1/1     Running   0          5m21s
pod/rate-limit-6b847b95c8-kwcbd                            1/1     Running   1          5m21s
pod/redis-7f6954b84d-ff4ck                                 1/1     Running   0          5m21s

NAME                                          TYPE           CLUSTER-IP       EXTERNAL-IP   PORT(S)                      AGE
service/extauth                               ClusterIP      10.109.93.97     <none>        8080/TCP                     5m22s
service/gateway-proxy                         LoadBalancer   10.106.26.131    <pending>     80:31627/TCP,443:30931/TCP   5m22s
service/gloo                                  ClusterIP      10.103.56.88     <none>        9977/TCP                     5m22s
service/glooe-grafana                         ClusterIP      10.103.252.250   <none>        80/TCP                       5m22s
service/glooe-prometheus-kube-state-metrics   ClusterIP      None             <none>        80/TCP                       5m22s
service/glooe-prometheus-server               ClusterIP      10.100.244.136   <none>        80/TCP                       5m22s
service/rate-limit                            ClusterIP      10.100.54.112    <none>        18081/TCP                    5m22s
service/redis                                 ClusterIP      10.97.72.199     <none>        6379/TCP                     5m22s

NAME                                                  READY   UP-TO-DATE   AVAILABLE   AGE
deployment.apps/api-server                            1/1     1            1           5m21s
deployment.apps/discovery                             1/1     1            1           5m22s
deployment.apps/extauth                               1/1     1            1           5m21s
deployment.apps/gateway                               1/1     1            1           5m22s
deployment.apps/gateway-proxy                         1/1     1            1           5m22s
deployment.apps/gloo                                  1/1     1            1           5m22s
deployment.apps/glooe-grafana                         1/1     1            1           5m22s
deployment.apps/glooe-prometheus-kube-state-metrics   1/1     1            1           5m22s
deployment.apps/glooe-prometheus-server               1/1     1            1           5m22s
deployment.apps/observability                         1/1     1            1           5m21s
deployment.apps/rate-limit                            1/1     1            1           5m21s
deployment.apps/redis                                 1/1     1            1           5m21s

NAME                                                             DESIRED   CURRENT   READY   AGE
replicaset.apps/api-server-56fcb78878                            1         1         1       5m21s
replicaset.apps/discovery-759bd6cf85                             1         1         1       5m22s
replicaset.apps/extauth-679d587db8                               1         1         1       5m21s
replicaset.apps/gateway-568bfd477c                               1         1         1       5m22s
replicaset.apps/gateway-proxy-c84cbd647                          1         1         1       5m22s
replicaset.apps/gloo-6979c5bd8                                   1         1         1       5m22s
replicaset.apps/glooe-grafana-86445b465b                         1         1         1       5m22s
replicaset.apps/glooe-prometheus-kube-state-metrics-8587f58df6   1         1         1       5m22s
replicaset.apps/glooe-prometheus-server-6bd6f4667d               1         1         1       5m21s
replicaset.apps/observability-6db6c659dd                         1         1         1       5m21s
replicaset.apps/rate-limit-6b847b95c8                            1         1         1       5m21s
replicaset.apps/redis-7f6954b84d                                 1         1         1       5m21s
```

```shell script
kubectl --namespace gloo-fed get all
```

```noop
NAME                                    READY   STATUS    RESTARTS   AGE
pod/gloo-fed-695d6dd44c-v2l64           1/1     Running   0          57m
pod/gloo-fed-console-774f958867-j7bwc   3/3     Running   0          57m

NAME                       TYPE        CLUSTER-IP     EXTERNAL-IP   PORT(S)                       AGE
service/gloo-fed-console   ClusterIP   10.96.107.54   <none>        10101/TCP,8090/TCP,8081/TCP   72m

NAME                               READY   UP-TO-DATE   AVAILABLE   AGE
deployment.apps/gloo-fed           1/1     1            1           72m
deployment.apps/gloo-fed-console   1/1     1            1           72m

NAME                                          DESIRED   CURRENT   READY   AGE
replicaset.apps/gloo-fed-695d6dd44c           1         1         1       72m
replicaset.apps/gloo-fed-console-774f958867   1         1         1       72m
```

#### Looking for opened ports?
You will NOT have any open ports listening on a default install. For Envoy to open the ports and actually listen, you need to have a Route defined in one of the VirtualServices that will be associated with that particular Gateway/Listener. Please see the [Hello World tutorial to get started]({{% versioned_link_path fromRoot="/guides/traffic_management/hello_world/" %}}). 

{{% notice note %}}
NOT opening the listener ports when there are no listeners (routes) is by design with the intention of not over-exposing your cluster by accident (for security). If you feel this behavior is not justified, please let us know.
{{% /notice %}}

## Uninstall {#uninstall}

To uninstall Gloo Edge, you can use the `glooctl` CLI. If you installed Gloo Edge to a different namespace, include the `-n` option.

```shell
glooctl uninstall -n my-namespace
```

{{% notice warning %}}
Make sure that your cluster has no other instances of Gloo Edge running, such as by running `kubectl get pods --all-namespaces`. If you remove the CRDs while Gloo Edge is still installed, you will experience errors.
{{% /notice %}}

```shell
glooctl uninstall --all
```

## Next Steps

After you've installed Gloo Edge, please check out our [User Guides]({{< versioned_link_path fromRoot="/guides/" >}}).

{{< readfile file="static/content/upgrade-crd.md" markdown="true">}}