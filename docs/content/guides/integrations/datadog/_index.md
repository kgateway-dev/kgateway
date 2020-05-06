---
title: Datadog
weight: 60
description: Integrate the Datadog agent with your Gloo and Envoy deployment
---

Datadog is a SaaS platform that allows you to easily collect metrics and events from your environment through integrations with with software like Kubernetes, cloud providers, Linux and more. In this guide, we will show you how Gloo can work with the Datadog Kubernetes integration to deliver information to Datadog for analysis.

---

## Prerequisites

You will need the following to complete this guide:

* **Datadog account**: If you don't already have an account, you can sign up for a free trial on their website.
* **Kubernetes cluster**: This can be deployed in any environment, follow our preparation guide for more information.
* **Gloo installation**: You can install Gloo on Kubernetes by following our setup guide.
* **Helm**: You will be deploying the Datadog integration using Helm. You can find the installation guide here.
* **kubectl**: Kubectl should be installed and configured to access the cluster where you are adding Datadog.

Once you have the prerequisites complete, you will need to get Datadog deployed on your Kubernetes cluster.

---

## Deploying Datadog for Gloo

Now that we've got all the prerequisites in place, let's get Datadog deployed and collecting data from Envoy.

### Prepare the datadog-values.yaml file

We will be using Helm to install Datadog on your Kubernetes cluster. First things first, we are going to download the `values.yaml` file from the Datadog GitHub repository and making a couple edits.

```console
wget https://raw.githubusercontent.com/helm/charts/master/stable/datadog/values.yaml -O datadog-values.yaml
```

Once you have the file, we are going to update two settings. The first is under `datadog.logs.enabled`. Update the yaml as follows:

```yaml
  logs:
    ## @param enabled - boolean - optional - default: false
    ## Enables this to activate Datadog Agent log collection.
    ## ref: https://docs.datadoghq.com/agent/basic_agent_usage/kubernetes/#log-collection-setup
    #
    enabled: true
```

The second is under `datadog.confd`. Update the yaml as follows:

```yaml
  confd:
    envoy.yaml: |-
      init_config:
      instances:
        - stats_url: "http://gateway-proxy-stats.gloo-system:8082/stats"
```

The first setting will enable log collection by Datadog. The second will let Datadog know that it can collect metrics from the path `gateway-proxy-stats.gloo-system:8082/stats`. 

Now that the `datadog-values.yaml` file is ready, we will use Helm to deploy Datadog to our Kubernetes cluster.

### Install Datadog with Helm

You will need to log into your Datadog account to retrieve the API keys for your installation. An example Helm command with your API keys can be found on the [Kubernetes integration page]https://app.datadoghq.com/account/settings#agent/kubernetes. Since we already prepared our `datadog-values.yaml` file in the previous step, we can simply run the following Helm command against the target Kubernetes cluster. Be sure to change the API_KEY to the key found in the example command in your Datadog account.

```bash
helm install datadog-gloo -f datadog-values.yaml --set datadog.apiKey=API_KEY stable/datadog 
```

You can validate that Datadog has installed by checking for the deployed pods.

```bash
kubectl get pods | grep datadog
```

```console
datadog-gloo-6d7wk                                 1/1     Running   0          3m1s
datadog-gloo-j227x                                 1/1     Running   0          3m1s
datadog-gloo-kube-state-metrics-678b97d74f-w69jz   1/1     Running   0          3m1s
datadog-gloo-prn8j                                 1/1     Running   0          3m1s
```

There will be a pod for each worker node in your cluster, and a pod for `kube-state-metrics`.

With Datadog installed, we now need to create the service that will publish the stats from Envoy.

### Create the stats service

Datadog is now looking at the path `gateway-proxy-stats.gloo-system:8082/stats` to collect stats. Let's create a service so it has something to collect!

Create a yaml file called `gateway-proxy-stats.yaml` with the following content:

```yaml
apiVersion: v1
kind: Service
metadata:
  labels:
    app: gloo
    gateway-proxy-id: gateway-proxy
    gloo: gateway-proxy
  name: gateway-proxy-stats
  namespace: gloo-system
spec:
  ports:
  - name: http
    port: 8082
    protocol: TCP
    targetPort: 8082
  selector:
    gateway-proxy: live
    gateway-proxy-id: gateway-proxy
```

The service assumes you have deployed Gloo in the namespace `gloo-system`. If you have deployed in a different namespace, adjust the configuration accordingly.

When you file is ready, go ahead and create the service:

```bash

```