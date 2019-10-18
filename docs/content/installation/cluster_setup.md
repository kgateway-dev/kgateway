---
title: Kubernetes Cluster Setup
weight: 1
description: How to prepare a Kubernetes cluster for Gloo installation.
---

In this document we will review how to prepare different Kubernetes environments before installing Gloo. 

Details for specific Kubernetes distributions:

- [Minikube](#minikube)
- [Minishift](#minishift-openshift)
- [Google Kubernetes Engine (GKE)](#google-kubernetes-engine-gke)
- [Azure Kubernetes Service (AKS)](#azure-kubernetes-service-aks)
- [Amazon Elastic Container Service for Kubernetes (EKS)](#amazon-elastic-container-service-for-kubernetes-eks)
- [Additional Notes](#additional-notes)
- [Next Steps](#next-steps)

{{% notice note %}}
This document assumes you have `kubectl` installed. Details on how to install [here](https://kubernetes.io/docs/tasks/tools/install-kubectl/).
{{% /notice %}}

---

## Minikube

Ensure you're running a standard Minikube cluster, e.g. `minikube start`, and verify that your `kubectl` context is
correctly pointing to it. More details on Minikube [here](https://kubernetes.io/docs/setup/minikube/).

```bash
kubectl config current-context
```

Should return `minikube` as the context.

You're all set. Gloo install guide [here](../).

---

## Minishift

Ensure you're running a standard Minishift cluster, e.g. `minishift start`, and verify that your `kubectl` context is
correctly pointing to it. More details on Minishift [here](https://github.com/minishift/minishift).

```bash
kubectl config current-context
```

Should return `minishift` as the context.

For installation, you need to be an admin-user, so use the following commands:

```bash
minishift addons install --defaults
minishift addons apply admin-user

# Login as administrator
oc login -u system:admin
```
For Gloo Enterprise, you need to enable certain permissions for storage and userid:

```bash
oc adm policy add-scc-to-user anyuid  -z glooe-prometheus-server -n gloo-system 
oc adm policy add-scc-to-user anyuid  -z glooe-prometheus-kube-state-metrics  -n gloo-system 
oc adm policy add-scc-to-user anyuid  -z default -n gloo-system 
oc adm policy add-scc-to-user anyuid  -z glooe-grafana -n gloo-system
```

You're all set. Gloo install guide [here](../)

---

## Google Kubernetes Engine (GKE)

Ensure you're running a standard GKE cluster, e.g. `gcloud container clusters create YOUR-CLUSTER-NAME`, and verify
that your `kubectl` context is correctly pointing to it. More details on GKE [here](https://cloud.google.com/kubernetes-engine/docs/quickstart).

```bash
gcloud container clusters get-credentials YOUR-CLUSTER-NAME
```

```bash
kubectl config current-context
```

Should return `gke_YOUR-PROJECT-ID_YOUR-REGION_YOUR-CLUSTER-NAME` as the context.

For installation, you need to be an admin-user, so use the following commands:

```bash
kubectl create clusterrolebinding cluster-admin-binding \
    --clusterrole cluster-admin \
    --user $(gcloud config get-value account)
```

You're all set. Gloo install guide [here](../).

---

## Azure Kubernetes Service (AKS)

Ensure you're running a standard AKS cluster. More details on
AKS [here](https://docs.microsoft.com/en-us/azure/aks/).

Example AKS cluster create.

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

Verify that your `kubectl` context is correctly pointing to it.

**NOTE**: the `--admin` option logs you into the cluster as the cluster admin, which is needed to install Gloo. It
assumes that you have granted "Azure Kubernetes Service Cluster Admin Role" to your current logged in user. More details
on AKS role access [here](https://docs.microsoft.com/en-us/azure/role-based-access-control/role-assignments-cli).

```bash
az aks get-credentials --resource-group myResourceGroup --name myAKSCluster --admin
```

```bash
kubectl config current-context
```

Should return `myAKSCluster-admin` as the context.

You're all set. Gloo install guide [here](../).

---

## Amazon Elastic Container Service for Kubernetes (EKS)

Ensure you're running a standard EKS cluster. More details on
AKS [here](https://docs.aws.amazon.com/eks/latest/userguide/getting-started.html).

We suggest using the `eksctl` tool from <https://eksctl.io/> as it complements the `aws` command line tool, and makes
it super simple to create and manage an EKS cluster from the command line. For example, to create an EKS cluster is as
simple as `eksctl create cluster --name YOUR-CLUSTER-NAME --region=YOUR-REGION`.

Verify that your `kubectl` context is correctly pointing to your EKS cluster.

```bash
aws eks --region YOUR-REGION update-kubeconfig --name YOUR-CLUSTER-NAME
```

```bash
kubectl config current-context
```

Should return `arn:aws:eks:YOUR-REGION:ACCOUNT_ID:cluster/YOUR-CLUSTER-NAME` as the context.

You're all set. Gloo install guide [here](../)

---

## Additional Notes

In addition to Gloo, usually you will also want to:

- Use a tool like [external-dns](https://github.com/kubernetes-incubator/external-dns) to setup DNS Record for Gloo.
- Use a tool like [cert-manager](https://github.com/jetstack/cert-manager/) to provision SSL certificates to use
with Gloo's VirtualService CRD.

## Next Steps

[Install Gloo](../)!
