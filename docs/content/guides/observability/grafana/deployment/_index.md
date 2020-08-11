---
title: Deployment Configuration
weight: 2
description: How to configure your Grafana installation
---

> **Note**: This page details configuration for the Grafana deployment packaged with Enterprise Gloo

 * [Default Installation](#default-installation)
    * [Credentials](#credentials)
 * [Custom Deployment](#custom-deployment)
 
### Default Installation
No special configuration is needed to use the instance of Grafana that ships by default with Gloo. Find the deployment and port-forward to it:

```bash
~ > kubectl -n gloo-system get deployment glooe-grafana
NAME            READY   UP-TO-DATE   AVAILABLE   AGE
glooe-grafana   1/1     1            1           34h

~ > kubectl -n gloo-system port-forward deployment/glooe-grafana 3000
Forwarding from 127.0.0.1:3000 -> 3000
Forwarding from [::1]:3000 -> 3000

```

Grafana can now be viewed at `http://localhost:3000`.

#### Credentials
The admin user/password combo that the default installation of Grafana starts up with is `admin/admin`.

These are read into the `glooe-grafana` pod's env from the secret `glooe-grafana`.

```bash
~ > kubectl -n gloo-system get secret glooe-grafana -o yaml
apiVersion: v1
data:
  # by default, these are both the base64 encoded string "admin"
  admin-password: YWRtaW4=
  admin-user: YWRtaW4=
kind: Secret
...
```

### Custom Deployment
If you'd like Gloo to talk to your pre-existing instance of Grafana, there are a few helm values that you'll need to set at install time. See the code snippet below for the bare minimum, but in general you'll need to set several values in the `observability.customGrafana` object; see a complete list of those fields [here]({{% versioned_link_path fromRoot="/installation/enterprise/#list-of-gloo-helm-chart-values" %}}).

```bash
helm install ... \
    --set grafana.defaultInstallationEnabled=false \
    --set observability.customGrafana.enabled=true

# the first --set ensures that the default deployment of Grafana is not created
# the second --set tells Gloo to expect to find configuration related to your own Grafana instance
```
