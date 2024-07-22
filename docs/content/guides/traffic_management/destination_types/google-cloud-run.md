---
title: Google Cloud Platform
weight: 100
description: Routing to Google Cloud Run as an Upstream.
---

Route traffic requests directly to a Google Cloud Platform (GCP) service, such as [Google Cloud Run](https://cloud.google.com/run).

{{% notice note %}}
{{< readfile file="static/content/enterprise_only_feature_disclaimer" markdown="true">}}
{{% /notice %}}

## Before you begin

Prepare your Google Cloud account for use with the Cloud Run and Kubernetes Engine (GKE) services.

1. Identify a Google Cloud project with billing enabled that you plan to use for both the Cloud Run and Kubernetes Engine instances.
2. Verify that you have the appropriate permissions to create Cloud Run and Kubernetes Engine instances.

## Step 1: Set up your Google Cloud environment {#google-cloud}

Create a sample Cloud Run workload. Then, use Google Workload Identity to authorize Gloo Gateway to send requests to the Cloud Run workload.

1. In your Google Cloud project, identify or create a Google Kubernetes Engine (GKE) cluster. For an example, follow the [Kubernetes Engine quickstart in the Google Cloud docs](https://cloud.google.com/kubernetes-engine/docs/quickstarts/create-cluster).

2. [Install Gloo Gateway Enterprise **version 1.17 or later** in your GKE cluster]({{% versioned_link_path fromRoot="/installation/platform_configuration/cluster_setup/#google-kubernetes-engine-gke" %}}).

3. In the same Google Cloud project as your cluster, deploy a `hello-world` Cloud Run application by following the [Deploy to Cloud Run quickstart in the Google Cloud docs](https://cloud.google.com/run/docs/quickstarts/deploy-container).

4. In your cluster, link the `gateway-proxy` Kubernetes ServiceAccount in the `gloo-system` namespace to your Google IAM service account. This way, the `gateway-proxy` can authenticate to your Google Cloud APIs by using Workload Identity Federation for GKE. 
   * At a minimum, the IAM service account must include the `run.invoker` and `iam.serviceAccountUser` roles.
   * For steps, see the [Kubernetes ServiceAccounts to IAM guide in the Google Cloud docs](https://cloud.google.com/kubernetes-engine/docs/how-to/workload-identity#kubernetes-sa-to-iam).

## Step 2: Create the Upstream for the Google Cloud Run service {#upstream}

The following examples create a basic VirtualService that routes traffic to an Upstream that represents your Cloud Run workload.

1. Create an Upstream that represents your Cloud Run workload. Replace the `host` with the Cloud Run endpoint that your GKE cluster can access. In the Google Cloud console, the host is the **URL** on the services detail page for your Cloud Run workload. For more options, see the [API docs]({{% versioned_link_path fromRoot="/reference/api/github.com/solo-io/gloo/projects/gloo/api/v1/enterprise/options/gcp/gcp.proto.sk/#upstreamspec" %}}).
   
   ```yaml
   kubectl apply -f - <<EOF
   apiVersion: gloo.solo.io/v1
   kind: Upstream
   metadata:
     name: cloud-run-upstream
     namespace: gloo-system
   spec:
     gcp:
       host: <hello-world>.a.run.app
   EOF
   ```

2. Create a VirtualService with a `/gcp` route that sends traffic to the Cloud Run Upstream. For more routing options, see the [API docs]({{% versioned_link_path fromRoot="/reference/api/github.com/solo-io/gloo/projects/gateway/api/v1/virtual_service.proto.sk/" %}}).

   ```yaml
   kubectl apply -f - <<EOF
   apiVersion: gateway.solo.io/v1
   kind: VirtualService
   metadata:
     name: cloud-run-vs
     namespace: gloo-system
   spec:
     virtualHost:
       domains:
       - '*'
       routes:
       - matchers:
         - prefix: /gcp
         routeAction:
           single:
             upstream:
               name: cloud-run-upstream
               namespace: gloo-system
   EOF
   ```

## Step 3: Verify traffic to the Upstream {#verify}

1. Send a request through Gloo Gateway to your Cloud Run workload.

   {{< tabs >}} 
{{% tab name="glooctl proxy url" %}}
```shell
curl $(glooctl proxy url -n gloo-system --name gateway-proxy)/gcp
```
{{% /tab %}} 
{{% tab name="Port-forwarding for local testing" %}}
1. Enable port-forwarding on the `gateway-proxy` service to your localhost.
   
   ```shell
   kubectl port-forward -n gloo-system svc/gateway-proxy 8080:80
   ```

2. Send a `curl` request to the localhost.

   ```shell
   curl -vik http://localhost:8080/gcp
   ```
{{% /tab %}} 
   {{< /tabs >}}

2. Verify that you get back the hello world response from your Cloud Run workload.

   Example response:

   ```
   <!doctype html>
   <html lang=en>
   <head>
   <meta charset=utf-8>
   <meta name="viewport" content="width=device-width, initial-scale=1">
   <meta name="robots" content="noindex,nofollow">
   <title>Congratulations | Cloud Run</title>
   ...
   ```

## Cleanup

You can optionally remove the resources that you set up as part of this guide.

1. Delete the routing resources in your cluster.
   
   ```shell
   kubectl delete upstream -n gloo-system cloud-run-upstream
   kubectl delete vs -n gloo-system cloud-run-vs
   ```

2. Delete the Google IAM service accounts and policy bindings.

3. Delete the sample Cloud Run and Kubernetes Engine instances.