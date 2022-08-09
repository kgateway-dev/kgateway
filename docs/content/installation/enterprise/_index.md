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
3. Check whether `glooctl`, the Gloo Edge command line tool (CLI), is installed.
   ```bash
   glooctl version
   ```
   * If `glooctl` is not installed, [install it](#install-glooctl).
   * If `glooctl` is installed, [update it to the latest version](#update-glooctl).

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
Using Helm 2 is not supported in Gloo Edge.
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
   quay.io/solo-io/gloo-fed-apiserver:1.12.1
   quay.io/solo-io/gloo-federation-console:1.12.1
   quay.io/solo-io/gloo-fed-apiserver-envoy:1.12.1
   quay.io/solo-io/gloo-fed:1.12.1
   quay.io/solo-io/gloo-ee:1.12.1
   quay.io/solo-io/discovery-ee:1.12.1
   quay.io/solo-io/gloo-ee-envoy-wrapper:1.12.1
   "grafana/grafana:8.2.1"
   "quay.io/coreos/kube-state-metrics:v1.9.7"
   "jimmidyson/configmap-reload:v0.5.0"
   "quay.io/prometheus/prometheus:v2.24.0"
   docker.io/busybox:1.28
   docker.io/redis:6.2.4
   quay.io/solo-io/rate-limit-ee:1.12.1
   quay.io/solo-io/extauth-ee:1.12.1
   quay.io/solo-io/observability-ee:1.12.1
   bitnami/kubectl:1.22.9
   bitnami/kubectl:1.22.9
   bitnami/kubectl:1.22.9
   quay.io/solo-io/certgen:1.12.0-rc6
   bitnami/kubectl:1.22.9
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
Using Helm 2 is not supported in Gloo Edge.
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

| Option                                                    | Type     | Description                                                                                                                                                                                                                                                    |
| --------------------------------------------------------- | -------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| grafana.defaultInstallationEnabled                        | bool     | Deploy Grafana in the `gloo-system` namespace. Default is `true`. |
| prometheus.enabled                                        | bool     | Deploy Prometheus in the `gloo-system` namespace. Default is `true`. |
| rateLimit.enabled                                         | bool     | Deploy the rate-limiting server in the `gloo-system` namespace. Default is `true`. |
| global.extensions.caching.enabled                         | bool     | Deploy the caching server in the `gloo-system` namespace. Default is `false`. |
| global.extensions.extAuth.enabled                         | bool     | Deploy the ext-auth server in the `gloo-system` namespace. Default is `true`. |
| global.extensions.extAuth.envoySidecar                    | bool     | Deploy ext-auth in the `gateway-proxy` pod as a sidecar to Envoy. Communicates over a Unix domain socket instead of TCP. Default is `false`. |
| observability.enabled                                     | bool     | Deploy Grafana in the `gloo-system` namespace. Default is `true`. |
| observability.customGrafana.enabled                       | bool     | Use your own Grafana instance instead of the default Gloo Edge Grafana instance. Default is `false`. |
| observability.customGrafana.username                      | string   | Authenticate to your custom Grafana instance using this username for basic auth. |
| observability.customGrafana.password                      | string   | Authenticate to your custom Grafana instance using this password basic auth. |
| observability.customGrafana.apiKey                        | string   | Authenticate to your custom Grafana instance using this API key. |
| observability.customGrafana.url                           | string   | The URL for your custom Grafana instance. |
---

## Enterprise UI

Gloo Edge Enterprise includes the user interface (UI) by default. Note that when you enable Gloo Federation, the UI does not show any data until you [register one or more clusters]({{< versioned_link_path fromRoot="/guides/gloo_federation/cluster_registration/" >}}). If you do not use Gloo Federation, the UI shows the installed Gloo Edge instance automatically without cluster registration.

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
NAME                                                       READY   STATUS    RESTARTS        AGE
pod/caching-service-5d7f867cdc-jgrbb                       1/1     Running   0               5m22s
pod/discovery-755c4f9f45-n5qxp                             1/1     Running   0               5m22s
pod/extauth-7c4c7f5999-m5zmc                               1/1     Running   0               5m22s
pod/gateway-proxy-59bf9f7d5f-7xm2f                         1/1     Running   2 (6d15h ago)   5m22s
pod/gloo-7bb4dcf577-g6mcm                                  1/1     Running   0               5m22s
pod/gloo-fed-7b55f67c5d-m6mdk                              1/1     Running   0               5m22s
pod/gloo-fed-console-7c446775c-mpzq8                       3/3     Running   0               5m22s
pod/glooe-grafana-865bb9cd45-v6f4v                         1/1     Running   0               5m22s
pod/glooe-prometheus-kube-state-metrics-55ffc89cbb-66x2h   1/1     Running   0               5m22s
pod/glooe-prometheus-server-7d5b85764c-2zb4d               2/2     Running   0               5m22s
pod/observability-7f8f5c549c-794pm                         1/1     Running   0               5m22s
pod/rate-limit-74d459bfb6-2pzc6                            1/1     Running   0               5m22s
pod/redis-57fd559c5c-jjrrc                                 1/1     Running   0               5m22s

NAME                                          TYPE           CLUSTER-IP     EXTERNAL-IP      PORT(S)                                                AGE
service/caching-service                       ClusterIP      10.76.6.4      <none>           8085/TCP                                               5m22s
service/extauth                               ClusterIP      10.76.9.130    <none>           8083/TCP                                               5m22s
service/gateway-proxy                         LoadBalancer   10.76.5.209    34.150.172.173   80:30451/TCP,443:32528/TCP                             5m22s
service/gloo                                  ClusterIP      10.76.6.206    <none>           9977/TCP,9976/TCP,9988/TCP,9966/TCP,9979/TCP,443/TCP   5m22s
service/gloo-fed-console                      ClusterIP      10.76.8.207    <none>           10101/TCP,8090/TCP,8081/TCP                            5m22s
service/glooe-grafana                         ClusterIP      10.76.3.207    <none>           80/TCP                                                 5m22s
service/glooe-prometheus-kube-state-metrics   ClusterIP      10.76.8.74     <none>           8080/TCP                                               5m22s
service/glooe-prometheus-server               ClusterIP      10.76.3.75     <none>           80/TCP                                                 5m22s
service/rate-limit                            ClusterIP      10.76.8.248    <none>           18081/TCP                                              5m22s
service/redis                                 ClusterIP      10.76.14.247   <none>           6379/TCP                                               5m22s

NAME                                                  READY   UP-TO-DATE   AVAILABLE   AGE
deployment.apps/caching-service                       1/1     1            1           5m22s
deployment.apps/discovery                             1/1     1            1           5m22s
deployment.apps/extauth                               1/1     1            1           5m22s
deployment.apps/gateway-proxy                         1/1     1            1           5m22s
deployment.apps/gloo                                  1/1     1            1           5m22s
deployment.apps/gloo-fed                              1/1     1            1           5m22s
deployment.apps/gloo-fed-console                      1/1     1            1           5m22s
deployment.apps/glooe-grafana                         1/1     1            1           5m22s
deployment.apps/glooe-prometheus-kube-state-metrics   1/1     1            1           5m22s
deployment.apps/glooe-prometheus-server               1/1     1            1           5m22s
deployment.apps/observability                         1/1     1            1           5m22s
deployment.apps/rate-limit                            1/1     1            1           5m22s
deployment.apps/redis                                 1/1     1            1           5m22s

NAME                                                             DESIRED   CURRENT   READY   AGE
replicaset.apps/caching-service-5d7f867cdc                       1         1         1       5m22s
replicaset.apps/discovery-755c4f9f45                             1         1         1       5m22s
replicaset.apps/extauth-7c4c7f5999                               1         1         1       5m22s
replicaset.apps/gateway-proxy-59bf9f7d5f                         1         1         1       5m22s
replicaset.apps/gloo-7bb4dcf577                                  1         1         1       5m22s
replicaset.apps/gloo-fed-7b55f67c5d                              1         1         1       5m22s
replicaset.apps/gloo-fed-console-7c446775c                       1         1         1       5m22s
replicaset.apps/glooe-grafana-865bb9cd45                         1         1         1       5m22s
replicaset.apps/glooe-prometheus-kube-state-metrics-55ffc89cbb   1         1         1       5m22s
replicaset.apps/glooe-prometheus-server-7d5b85764c               1         1         1       5m22s
replicaset.apps/observability-7f8f5c549c                         1         1         1       5m22s
replicaset.apps/rate-limit-74d459bfb6                            1         1         1       5m22s
replicaset.apps/redis-57fd559c5c                                 1         1         1       5m22s
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

{{< readfile file="static/content/upgrade-note.md" markdown="true">}}