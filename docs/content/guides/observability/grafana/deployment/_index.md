---
title: Deployment Configuration
weight: 2
description: How to configure your Grafana installation
---
This functionality is turned on by default, and can be turned off a couple of different ways: through [Helm chart install
options]({{< versioned_link_path fromRoot="/installation/gateway/kubernetes/#installing-the-gloo-gateway-on-kubernetes" >}}); and through environment variables.

 * [Default Installation](#default-installation)
    * [Credentials](#credentials)
 * [Custom Deployment](#custom-deployment)
 
### Default Installation

{{% notice warning %}}
Gloo is shipped by default with grafana 5.x charts, and provides a set of default values that are not suitable for production usage in most cases. Please provide your own instance of grafana or configure the helm chart options with production values
{{% /notice %}}

{{% notice note %}}
For a complete set of options, please refer to: https://github.com/grafana/helm-charts/tree/main/charts/grafana
{{% /notice %}}

No special configuration is needed to use the instance of Grafana that ships by default with Gloo Edge. Find the deployment and port-forward to it:

```bash
~ > kubectl -n gloo-system get deployment glooe-grafana
NAME            READY   UP-TO-DATE   AVAILABLE   AGE
glooe-grafana   1/1     1            1           34h

~ > kubectl -n gloo-system port-forward deployment/glooe-grafana 3000
Forwarding from 127.0.0.1:3000 -> 3000
Forwarding from [::1]:3000 -> 3000

```

Grafana can now be viewed at `localhost:3000`.

#### Credentials
The admin user/password combo that the default installation of Grafana starts up with is controlled by the helm values `grafana.adminUser` and `grafana.adminPassword`, which are set to `admin/admin` by default.

These are read into the `observability` pod's env from the secret `gloo-observability-secrets`.

```bash
~ > kubectl -n gloo-system get secret gloo-observability-secrets -o yaml
apiVersion: v1
data:
  # by default, these are both the base64 encoded string "admin"
  GRAFANA_PASSWORD: YWRtaW4=
  GRAFANA_USERNAME: YWRtaW4=
kind: Secret
...
```

If you create an API key in Grafana, you can make the pod use it instead of basic auth by setting the key `GRAFANA_API_KEY` in that same secret and then restarting the `observability` pod. If an API key is present, the pod will prefer to use that over any username/password combo that may be set.

### Custom Deployment
If you'd like Gloo Edge to talk to your pre-existing instance of Grafana, there are a few helm values that you'll need to set at install time. See the code snippet below for the bare minimum, but in general you'll need to set several values in the `observability.customGrafana` object; see a complete list of those fields [here]({{% versioned_link_path fromRoot="/installation/enterprise/#list-of-gloo-helm-chart-values" %}}).

```bash
helm install ... \
    --set grafana.defaultInstallationEnabled=false \
    --set observability.customGrafana.enabled=true

# the first --set ensures that the default deployment of Grafana is not created
# the second --set tells Gloo Edge to expect to find configuration related to your own Grafana instance
```
