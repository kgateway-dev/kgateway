---
title: Kubernetes Cluster Setup
weight: 10
description: How to prepare a Kubernetes cluster for Gloo installation.
---

Installing Gloo will require an environment for installation. Kubernetes and OpenShift are common targets for the installation of the Gloo Gateway. In this document we will review how to prepare different Kubernetes and OpenShift environments for the installation of Gloo. 

Click on the links below for details specific to your Kubernetes distribution:

- [Minikube](#minikube)
- [Minishift](#minishift)
- [Google Kubernetes Engine (GKE)](#google-kubernetes-engine-gke)
- [Azure Kubernetes Service (AKS)](#azure-kubernetes-service-aks)
- [Amazon Elastic Container Service for Kubernetes (EKS)](#amazon-elastic-container-service-for-kubernetes-eks)
- [Additional Notes](#additional-notes)
- [Next Steps](#next-steps)

{{% notice note %}}
Minimum required Kubernetes is 1.11.x. For older versions see our [release support guide]({{% versioned_link_path fromRoot="/introduction/support/#kubernetes" %}})
{{% /notice %}}

{{% notice note %}}
This document assumes you have `kubectl` installed. Details on how to install [here](https://kubernetes.io/docs/tasks/tools/install-kubectl/).
{{% /notice %}}

---

## Minikube

Minikube is a single-node Kubernetes cluster running inside a VM on your local machine. You can use Minikube to try out Kubernetes features or perform local development. You can find more details on running Minikube [here](https://kubernetes.io/docs/setup/minikube/).

Ensure you're running a standard Minikube cluster, e.g. `minikube start`, and verify that your `kubectl` context is
correctly pointing to it.

```bash
kubectl config current-context
```

This command should return `minikube` as the context.

If it does not, you can switch to the `minikube` context by running the following:

```bash
kubectl config use-context minikube
```

Now you're all set to install Gloo, simply follow the Gloo installation guide [here]({{< versioned_link_path fromRoot="/installation" >}}).

{{% notice note %}}
To avoid resource limitations, make sure to give your Minikube VM extra RAM and CPU. Minimally, 
we recommend provide the following arguments to Minikube: `minikube start --memory=4096 --cpus=2`. 
{{% /notice %}}

---

## Minishift

Minishift runs a single-node OpenShift cluster inside a VM running on your local machine. You can use Minishift to try out OpenShift features or perform local development. You can find more details on running Minishift [here](https://github.com/minishift/minishift).

Ensure you're running a standard Minishift cluster, e.g. `minishift start`, and verify that your `kubectl` context is
correctly pointing to it. 

```bash
kubectl config current-context
```

This command should return `minishift` as the context.

If it does not, you can switch to the `minishift` context by running the following:

```bash
kubectl config use-context minishift
```

For installation, you need to be an admin-user, so use the following commands:

```bash
minishift addons install --defaults
minishift addons apply admin-user

# Login as administrator
oc login -u system:admin
```
If you plan to install Gloo Enterprise, you will need to enable certain permissions for storage and userid:

```bash
oc adm policy add-scc-to-user anyuid  -z glooe-prometheus-server -n gloo-system 
oc adm policy add-scc-to-user anyuid  -z glooe-prometheus-kube-state-metrics  -n gloo-system 
oc adm policy add-scc-to-user anyuid  -z default -n gloo-system 
oc adm policy add-scc-to-user anyuid  -z glooe-grafana -n gloo-system
```

Now you're all set to install Gloo, simply follow the Gloo installation guide [here]({{< versioned_link_path fromRoot="/installation" >}}).

---

## Google Kubernetes Engine (GKE)

Google Kubernetes Engine is Google Cloud's managed Kubernetes service. GKE can run both development and production workloads depending on its size and configuration. You can find more details on GKE [here](https://cloud.google.com/kubernetes-engine/docs/quickstart).

You will need to deploy a GKE cluster. The default settings in the `clusters create` command should be sufficient for installing Gloo Gateway and going through the [Gloo Routing]({{< versioned_link_path fromRoot="/gloo_routing" >}}) examples. The commands below can be run as-is, although you may want to change the zone (*us-central1-a*) and cluster name (*myGKECluster*).

These commands can be run locally if you have the [Google Cloud SDK](https://cloud.google.com/sdk/) installed or using the Cloud Shell from the [GCP Console](https://console.cloud.google.com). The Cloud Shell already has `kubectl` installed along with the Google Cloud SDK.

Example GKE cluster creation:

```bash
gcloud container clusters create myGKECluster \
  --zone=us-central1-a
```

```console
kubeconfig entry generated for YOUR-CLUSTER-NAME.
NAME          LOCATION       MASTER_VERSION  MASTER_IP        MACHINE_TYPE   NODE_VERSION   NUM_NODES  STATUS
myGKECluster  us-central1-a  1.13.11-gke.9   XXX.XXX.XXX.XXX  n1-standard-1  1.13.11-gke.9  3          RUNNING
```

Next you will need to make sure that your `kubectl` context is correctly set to the newly created cluster. 

```bash
gcloud container clusters get-credentials myGKECluster \
  --zone=us-central1-a
```

```console
Fetching cluster endpoint and auth data.
kubeconfig entry generated for myGKECluster.
```

You can retrieve the current context by running the command below.

```bash
kubectl config current-context
```

The command should return `gke_YOUR-PROJECT-ID_us-central1-a_myGKECluster` as the context.

For installation, you need to be an admin-user, so use the following commands:

```bash
kubectl create clusterrolebinding cluster-admin-binding \
    --clusterrole cluster-admin \
    --user $(gcloud config get-value account)
```

Now you're all set to install Gloo, simply follow the Gloo installation guide [here]({{< versioned_link_path fromRoot="/installation" >}}).

---

## Azure Kubernetes Service (AKS)

Azure Kubernetes Service is Microsoft Azure's managed Kubernetes service. AKS can run both development and production workloads depending on its size and configuration. You can find more details on AKS [here](https://docs.microsoft.com/en-us/azure/aks/).

You will need to deploy an AKS cluster. The default settings in the `aks create` command should be sufficient for installing Gloo Gateway and going through the [Gloo Routing]({{< versioned_link_path fromRoot="/gloo_routing" >}}) examples. The commands below can be run as-is, although you may want to change the resource group location (*eastus*), resource group name (*myResourceGroup*), and cluster name (*myAKSCluster*).

These commands can be run locally if you have the [Azure CLI](https://docs.microsoft.com/en-us/cli/azure/install-azure-cli?view=azure-cli-latest) installed or by using the [Azure Cloud Shell](https://shell.azure.com). The Azure Cloud Shell already has `kubectl` installed along with the Azure CLI.

Example AKS cluster creation:

```bash
az group create \
    --name myResourceGroup \
    --location eastus

az aks create \
    --resource-group myResourceGroup \
    --name myAKSCluster \
    --node-count 1 \
    --enable-addons monitoring \
    --generate-ssh-keys
```

```console
[...]

"provisioningState": "Succeeded",
"resourceGroup": "myResourceGroup"

[...]
```

Next you will need to make sure that your `kubectl` context is correctly set to the newly created cluster. 

{{% notice note %}}
The `--admin` option logs you into the cluster as the cluster admin, which is needed to install Gloo. It
assumes that you have granted "Azure Kubernetes Service Cluster Admin Role" to your current logged in user. More details
on AKS role access [here](https://docs.microsoft.com/en-us/azure/role-based-access-control/role-assignments-cli).
{{% /notice %}}

```bash
az aks get-credentials --resource-group myResourceGroup --name myAKSCluster --admin
```

```console
Merged "myAKSCluster-admin" as current context in /home/USER/.kube/config
```

You can retrieve the current context by running the command below.

```bash
kubectl config current-context
```

The command should return `myAKSCluster-admin` as the context.

Now you're all set to install Gloo, simply follow the Gloo installation guide [here]({{< versioned_link_path fromRoot="/installation" >}}).

---

## Amazon Elastic Container Service for Kubernetes (EKS)

Amazon Elastic Kubernetes Service is Amazon's managed Kubernetes service. EKS can run both development and production workloads depending on its size and configuration. You can find more details on EKS [here](https://docs.aws.amazon.com/eks/latest/userguide/getting-started.html).

You will need to deploy an EKS cluster. We suggest using the `eksctl` tool from <https://eksctl.io/> as it complements the `aws` command line tool, and makes it super simple to create and manage an EKS cluster from the command line. To run the following commands, you will need both the AWS CLI and the `eksctl` tool installed on your local machine.

The default settings in the `eks create cluster` command should be sufficient for installing Gloo Gateway and going through the [Gloo Routing]({{< versioned_link_path fromRoot="/gloo_routing" >}}) examples. The commands below can be run as-is, although you may want to change the region (*us-east-1*) and cluster name (*myEKSCluster*).

Example AKS cluster creation:

```bash
eksctl create cluster --name myEKSCluster --region=us-east-1
```

```console
[...]
kubectl command should work with "/home/USER/.kube/config", try 'kubectl get nodes'
EKS cluster "myEKSCluster" in "us-east-1" region is ready
[...]
```

Next you will need to make sure that your `kubectl` context is correctly set to the newly created cluster. 

```bash
aws eks --region us-east-1 update-kubeconfig --name myEKSCluster
```

```console
Added new context arn:aws:eks:us-east-1:ACCOUNT-ID:cluster/myEKSCluster to /home/USER/.kube/config
```

You can retrieve the current context by running the command below.

```bash
kubectl config current-context
```

The command should `arn:aws:eks:us-east-1:ACCOUNT-ID:cluster/myEKSCluster` as the context.

Now you're all set to install Gloo, simply follow the Gloo installation guide [here]({{< versioned_link_path fromRoot="/installation" >}}).

---

## Additional Notes

While these additional sections are not required to set up your Kubernetes cluster or install Gloo, you may want to consider your approach for managing things like DNS and SSL certificates.

### DNS Records

Kubernetes DNS will take care of the internal DNS for the cluster, but it does not publish public DNS records for services running inside the cluster including Gloo Gateway. Using a tool like [external-dns](https://github.com/kubernetes-incubator/external-dns) will enable you to set up DNS records for Gloo and make your services publicly accessible.

### Certificate Management

Gloo has the ability to provide TLS off-load for services running inside the Kubernetes cluster through Gloo's VirtualService Custom Resource Definition (CRD). Gloo does not handle the actual provisioning and management of certificates for use with TLS communication. You can use a tool like [cert-manager](https://github.com/jetstack/cert-manager/) to provision those SSL certificates and store them in Kubernetes Secrets for Gloo to consume.

## Next Steps

Woo-hoo! You've made it through the gauntlet of getting your Kubernetes cluster ready. Now let's get to the fun stuff, [installing Gloo]({{< versioned_link_path fromRoot="/installation" >}})!
