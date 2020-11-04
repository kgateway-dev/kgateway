---
title: Datadog
weight: 60
description: Integrate the Datadog agent with your Gloo Edge and Envoy deployment
---

Datadog is a SaaS platform that allows you to easily collect metrics and events from your environment through integrations with solutions like Kubernetes, public cloud providers, Linux and more. In this guide, we will show you how Gloo Edge can work with the Datadog Kubernetes integration to deliver information from Envoy to Datadog for analysis.

Here are the high level steps for completing the integration:

1. Install Kubernetes and Envoy Integrations
1. Get the API key for Helm installation
1. Create and edit Datadog values file
1. Install Datadog using Helm
1. Update the gateway-proxy Deployment
1. Update the gateway-proxy ConfigMap
1. Validate using kubectl

---

## Prerequisites

You will need the following to complete this guide:

* **Datadog account**: If you don't already have an account, you can sign up for a free trial [on their website](https://app.datadoghq.com).
* **Kubernetes cluster**: This can be deployed in any environment, follow our [preparation guide]({{% versioned_link_path fromRoot="/installation/platform_configuration/cluster_setup/" %}}) for more information.
* **Gloo Edge installation**: You can install Gloo Edge on Kubernetes by following our [setup guide](/installation/gateway/kubernetes).
* **Helm**: You will be deploying the Datadog integration using [Helm v3](https://helm.sh/docs/intro/install/).
* **kubectl**: Kubectl should be installed and configured to access the cluster where you are adding Datadog.

Once you have the prerequisites complete, you will need to get Datadog deployed on your Kubernetes cluster.

---

## Prepare Datadog

Now that we've got all the prerequisites in place, there are a few things to do on the Datadog side to prepare for data collection. We need to enable the Kubernetes and Envoy integrations and get the API key for the Kubernetes installation.

### Install Kubernetes and Envoy Integrations

From the Datadog portal, select Integrations and install the Envoy and Kubernetes integrations.

![Envoy Integration]({{% versioned_link_path fromRoot="/img/datadog-envoy.png" %}})

![Kubernetes Integration]({{% versioned_link_path fromRoot="/img/datadog-k8s.png" %}})

If this is the first time you've logged into Datadog, you can also follow the wizard for enabling Kubernetes and Envoy.

### Retrieve the API key

From the Datadog portal, select Integrations and the Agents subcategory. Then select the Kubernetes agent. Under the directions, you can find the Helm command to install the Datadog agent on your cluster. It will look like this:

```bash
helm install datadog-gloo -f datadog-values.yaml --set datadog.apiKey=API_KEY stable/datadog 
```

Copy this command for later use.

---

## Install Datadog using Helm

We will be using Helm to install Datadog on your Kubernetes cluster. We will need to create a file to override the default values in the Helm chart.

### Prepare the datadog-values.yaml file

First things first, we are going to download the `values.yaml` file from the Datadog GitHub repository and making a couple edits.

```console
wget https://raw.githubusercontent.com/helm/charts/master/stable/datadog/values.yaml -O datadog-values.yaml
```

Once you have the file, we are going to update two settings. They are both under `datadog.logs`. Update the yaml as follows:

```yaml
  logs:
    ## @param enabled - boolean - optional - default: false
    ## Enables this to activate Datadog Agent log collection.
    ## ref: https://docs.datadoghq.com/agent/basic_agent_usage/kubernetes/#log-collection-setup
    #
    enabled: true

    ## @param containerCollectAll - boolean - optional - default: false
    ## Enable this to allow log collection for all containers.
    ## ref: https://docs.datadoghq.com/agent/basic_agent_usage/kubernetes/#log-collection-setup
    #
    containerCollectAll: true
```

The first setting will enable log collection by Datadog and the second enables log collection across all containers.

Now that the `datadog-values.yaml` file is ready, we will use Helm to deploy Datadog to our Kubernetes cluster.

### Install Datadog with Helm

You will need to log into your Datadog account to retrieve the API keys for your installation. We retrieved them in an earlier step. You can find the example Helm command with your API keys on the [Kubernetes integration page](https://app.datadoghq.com/account/settings#agent/kubernetes). 

Since we already prepared our `datadog-values.yaml` file in the previous step, we can simply run the following Helm command against the target Kubernetes cluster. Be sure to change the `API_KEY` to the key found in the example command in your Datadog account.

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

The installation creates a daemonset for the Datadog agent, so there will be a pod for each worker node in your cluster, and a pod for `kube-state-metrics`.

With Datadog installed, we now need to configure our Gloo Edge deployment to make Envoy metrics available to Datadog.

---

## Configure the Gloo Edge deployment

We will need to update two things to allow metrics collection from Datadog. In order to discover the Envoy pods, the Datadog agent is relying on an [Autodiscovery feature](https://docs.datadoghq.com/agent/kubernetes/integrations/?tab=kubernetes) that looks for specific annotations associated with Envoy pods. 

The Envoy pods are configured by default to listen on port 8081 and the path `/metrics`. When a request comes in matching that path, it is rewritten to `/stats/prometheus`. Since we want to use Datadog instead, we are going to update that rule to rewrite the path to just `/stats`. If you'd like to use both Prometheus and Datadog, you could set up a matcher for a different path and rewrite it to `/stats`.

### Updating the annotations

We are going to edit the deployment for the `gateway-proxy` in Gloo Edge, which includes the pods running Envoy. We are going to add a series of Datadog specific annotations that alert the agent that these pods are running Envoy. The annotations also let Datadog know what address to use for metrics collection and any log processing rules. The full list of potential annotations can be found in the [Datadog documentation](https://docs.datadoghq.com/agent/kubernetes/integrations/?tab=kubernetes).

Assuming that you have deployed Gloo Edge in the namespace `gloo-system`, run the following command:

```bash
kubectl edit deployments -n gloo-system gateway-proxy
```

Then update the `spec.template` section of the yaml with these additional annotations.

```yaml
  template:
    metadata:
      annotations:
        ad.datadoghq.com/gateway-proxy.check_names: '["envoy"]'
        ad.datadoghq.com/gateway-proxy.init_configs: '[{}]'
        ad.datadoghq.com/gateway-proxy.instances: '[{"stats_url": "http://%%host%%:8081/metrics"}]'
        ad.datadoghq.com/gateway-proxy.logs: '[{"source": "envoy", "service": "gloo","log_processing_rules":
         [{"type": "multi_line", "name": "log_start_with_date","pattern" : "^\\[[0-9]{4}-[0-9]{2}-[0-9]{2}|{"}]}]'
```

{{< notice note >}}
If you upgrade the cluster using Helm version 3, these annotations should stay in place. Helm 3 uses a three-way merge when performing an update. Helm version 2 will also attempt a merge, but may have issues with changes made using kubectl edit. You should update the values used by Helm to include these annotations. More information is available [here](https://helm.sh/docs/faq/#improved-upgrade-strategy-3-way-strategic-merge-patches).
{{< /notice >}}

You can verify that the annotations have been successfully updated by running the following command:

```bash
kubectl get pods -n gloo-system -l gloo=gateway-proxy -o json | jq '.items | .[].metadata.annotations'
```

### Updating the Envoy configuration

The configuration of the Envoy `gateway-proxy` is stored in a ConfigMap called `gateway-proxy-envoy-config`. We are going to update the ConfigMap to update the route on the Prometheus listeners. 

Assuming that you have deployed Gloo Edge in the namespace `gloo-system`, run the following command:

```bash
kubectl edit configmap gateway-proxy-envoy-config -n gloo-system
```

Find the following section in the ConfigMap:

```yaml
  route:
    prefix_rewrite: "/stats/prometheus"
    cluster: admin_port_cluster
```

And update it to the following:

```yaml
  route:
    prefix_rewrite: "/stats" # instead of "/stats/prometheus"
    cluster: admin_port_cluster
```

Then save the change. The Envoy pod will update within a few seconds with the new configuration settings. Datadog should now be able to collect metrics from any Envoy pods using the `/metrics` path.

---

## Validate the configuration

We can check that the Datadog agent has enabled Envoy metrics collection by running the `agent status` command against the daemonset pod that is on the same node as the `gateway-proxy` pod. First we can get the worker node in question by running:

```bash
NODE_NAME=$(kubectl get pods -n gloo-system -l gloo=gateway-proxy -o json | jq '.items | .[].spec.nodeName' -r)
```

Then we can get the daemonset pod for Datadog by running the following:

```bash
kubectl get pods -o wide --field-selector spec.nodeName=$NODE_NAME
```

You may see two pods returned. You want the one that doesn't have `kube-state-metrics` in the name. Now you can run the `agent status` command from inside the Datadog agent pod. Change the `POD_NAME` to reflect your pod name.

```bash
kubectl exec -it POD_NAME agent status | grep envoy
```

The output should be similar to this:

```console
envoy (1.14.0)
      Instance ID: envoy:f312c8247060dc62 [OK]
```

That means the Datadog agent has fired up the Envoy integration and should be collecting metrics. You can now verify this by going back to the portal and checking through the Metrics Explorer.

---

## Summary

In this guide we showed you how to integrate Datadog with Gloo Edge and Envoy proxies. It's important to remember that the changes made to the gateway-proxy ConfigMap and Deployment should also be updated in the Helm values file you use to deploy Gloo Edge. This is especially important if you are using Helm version 2, which does not gracefully handle out-of-band changes. It is also possible to configure these settings before Gloo Edge is installed by using a custom values file with the Helm installation. 

The full list of Helm values is available in the [docs]({{% versioned_link_path fromRoot="/reference/helm_chart_values/" %}}). In particular the value `gatewayProxies.gatewayProxy.podTemplate.extraAnnotations.NAME` can be updated to add the required annotations. You can use [Last Mile Patching for Helm]({{% versioned_link_path fromRoot="/installation/gateway/kubernetes/helm_advanced/" %}}) to patch the produced Helm chart to include the ConfigMap alterations prior to deployment, or simply make the edits post deployment.
