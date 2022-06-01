---
title: Set up your gateway with an Istio sidecar
menuTitle: Configure your Gloo Edge gateway to run an Istio sidecar 
weight: 20
---

You can configure your Gloo Edge gateway with an Istio sidecar to secure the connection between your gateway and the services in your Istio service mesh. The sidecar in your Gloo Edge gateway uses mutual TLS (mTLS) to proves its identity to the services in the mesh and vice versa.

## Before you begin

Complete the following tasks before configuring an Istio sidecar for your Gloo Edge gateway: 

1. Create or use an existing cluster that runs Kubernetes version 1.20 or later. 
2. [Install Istio in your cluster](https://istio.io/latest/docs/setup/getting-started/). Currently, Istio version 1.11 and 1.12 are supported in Gloo Edge.
3. Set up a service mesh for your cluster. For example, you can use [Gloo Mesh Enterprise](https://docs.solo.io/gloo-mesh-enterprise/latest/getting_started/managed_kubernetes/) to configure a service mesh that is based on Envoy and Istio, and that you can span across multiple service meshes and clusters. 
4. Install an application in your mesh, such as Bookinfo. 
   ```shell
   kubectl label namespace default istio-injection=enabled
   kubectl apply -f samples/bookinfo/platform/kube/bookinfo.yaml
   ```
   
5. Install [Helm](https://helm.sh/docs/intro/install/) on your local machine.

## Configure the Gloo Edge gateway with an Istio sidecar

Install the Gloo Edge gateway and inject it with an Istio sidecar. 

1. Add the Gloo Edge Helm repo. 
   ```shell
   helm repo add gloo https://storage.googleapis.com/solo-public-helm
   ```
   
2. Update the repo. 
   ```shell
   helm repo update
   ```
   
3. Create the namespace where you want to install Gloo Edge. The following command creates the `gloo` namespace.
   ```shell
   kubectl create namespace gloo-system
   ```
   
4. Create a `value-overrides.yaml` file with the following content. To configure your gateway with an Istio sidecar, make sure to add the `istioIntegration` section and set the `enableIstioSidecarOnGateway` option to `true`. 
   ```yaml
   global:
     istioIntegration:
       labelInstallNamespace: true
       whitelistDiscovery: true
       enableIstioSidecarOnGateway: true
   gatewayProxies:
     gatewayProxy:
       podTemplate: 
         httpPort: 8080
         httpsPort: 8443
   ```
   
5. Install Gloo Edge with the settings in the `value-overrides.yaml` file.  
   ```shell
   helm install gloo gloo/gloo --namespace gloo-system -f value-overrides.yaml
   ```
   
6. [Verify your installation]({{< versioned_link_path fromRoot="/installation/gateway/kubernetes/#verify-your-installation" >}}). 
8. Label the `gloo` namespace to automatically inject an Istio sidecar to all pods that run in that namespace. 
   ```shell
   kubectl label namespaces gloo-system istio-injection=enabled
   ```
   
9. Restart the proxy gateway deployment to pick up the Envoy configuration for the Istio sidecar. 
   ```shell
   kubectl rollout restart -n gloo-system deployment gateway-proxy
   ```
   
10. Get the pods for your gateway proxy deployment. You now see a second container in each pod. 
    ```shell
    kubectl get pods -n gloo-system
    ```
    
    Example output: 
    ```
    NAME                             READY   STATUS    RESTARTS   AGE
    discovery-5c66ccfccb-tvr5v       1/1     Running   0          3h58m
    gateway-6f88cff479-7mx6k         1/1     Running   0          3h58m
    gateway-proxy-584974c887-km4mk   2/2     Running   0          158m
    gloo-6c8f68bd4b-rv52f            1/1     Running   0          3h58m
    ```

Congratuliations! You successfully configured an Istio sidecar for your Gloo Edge gateway. 

## Verify the connection 
