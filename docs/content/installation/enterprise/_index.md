---
title: "Installing Gloo Edge Enterprise"
menuTitle: Gloo Edge Enterprise
description: How to install Gloo Edge to run in Gateway Mode on Kubernetes (Default).
weight: 60
---

## Installing the Gloo Edge on Kubernetes

These directions assume you've prepared your Kubernetes cluster appropriately. Full details on setting up your Kubernetes cluster [here]({{% versioned_link_path fromRoot="/installation/platform_configuration/cluster_setup/" %}}).

Note: For certain providers with more strict multi-tenant security, like OpenShift, be sure to follow the cluster set up accordingly. 

{{< readfile file="installation/glooctl_setup.md" markdown="true" >}}

{{% notice note %}}
To install Gloo Edge Enterprise you need a License Key. If you don't have one, go to [**https://solo.io/gloo**](https://www.solo.io/products/gloo/#enterprise-trial) and
request a trial now. Once you request a trial, an e-mail will be sent to you with your unique License Key that you will
need as part of installing Gloo Edge.
{{% /notice %}}

{{% notice info %}}
Each Key is valid for **31 days**. You can request a new key if your current key has expired.
The License Key is required only during the installation process. Once you install, a `secret` will be created to hold
your unique key.
{{% /notice %}}

Before starting installation, please ensure that you've prepared your Kubernetes cluster per the community
[Prep Kubernetes]({{< versioned_link_path fromRoot="/installation/platform_configuration/cluster_setup" >}}) instructions.


### Installing on Kubernetes with `glooctl`

Once your Kubernetes cluster is up and running, run the following command to deploy the Gloo Edge to the `gloo-system` namespace:

```bash
glooctl install gateway enterprise --license-key YOUR_LICENSE_KEY
```

<details>
<summary>Special Instructions to Install Gloo Edge Enterprise on Kind</summary>
If you followed the cluster setup instructions for Kind [here]({{< versioned_link_path fromRoot="/installation/platform_configuration/cluster_setup/#kind" >}}), then you should have exposed custom ports 31500 (for http) and 32500 (https) from your cluster's Docker container to its host machine. The purpose of this is to make it easier to access your service endpoints from your host workstation.  Use the following custom installation for Gloo Edge to publish those same ports from the proxy as well.

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


This is the recommended method for installing Gloo Edge to your production environment as it offers rich customization to
the Gloo Edge control plane and the proxies Gloo Edge manages.

As a first step, you have to add the Gloo Edge repository to the list of known chart repositories:

```shell
helm repo add glooe http://storage.googleapis.com/gloo-ee-helm
```

Finally, install Gloo Edge using the following command:

{{< tabs >}}
{{< tab name="Helm 2" codelang="shell">}}
helm install glooe/gloo-ee --name gloo --namespace gloo-system \
  --set gloo.crds.create=true --set-string license_key=YOUR_LICENSE_KEY
{{< /tab >}}
{{< tab name="Helm 3" codelang="shell">}}
helm install gloo glooe/gloo-ee --namespace gloo-system \
  --create-namespace --set-string license_key=YOUR_LICENSE_KEY
{{< /tab >}}
{{< /tabs >}}

Once you've installed Gloo Edge, please be sure [to verify your installation](#verify-your-installation).

#### Customizing your installation with Helm

You can customize the Gloo Edge installation by providing your own value file.

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

and use it to override default values in the Gloo Edge Helm chart:

{{< tabs >}}
{{< tab name="Helm 2" codelang="shell">}}
helm install glooe/gloo-ee --name gloo --namespace gloo-system \
  -f value-overrides.yaml --set gloo.crds.create=true --set-string license_key=YOUR_LICENSE_KEY
{{< /tab >}}
{{< tab name="Helm 3" codelang="shell">}}
helm install gloo glooe/gloo-ee --namespace gloo-system \
  -f value-overrides.yaml --create-namespace --set-string license_key=YOUR_LICENSE_KEY
{{< /tab >}}
{{< /tabs >}}

#### List of Gloo Edge Helm chart values

The table below describes the most important enterprise-only values that you can override in your custom values file.

The table for gloo open-source overrides (also available in enterprise) is [here]({{< versioned_link_path fromRoot="/reference/helm_chart_values/" >}}). To make customizations that are not part of the helm chart, please see our [advanced customization guide]({{% versioned_link_path fromRoot="/installation/gateway/kubernetes/helm_advanced/" %}})

{{% notice note %}}
Open source helm values in Gloo Edge enterprise must be prefixed with `gloo`, unless they are the Gloo Edge settings (i.e., `settings.<rest of helm value>`).
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

To uninstall Gloo Edge and all related components, simply run the following.

```shell
glooctl uninstall
```

If you installed Gloo Edge to a different namespace, you will have to specify that namespace using the `-n` option:

```shell
glooctl uninstall -n my-namespace
```

## Next Steps

After you've installed Gloo Edge, please check out our [User Guides]({{< versioned_link_path fromRoot="/guides/" >}}).
