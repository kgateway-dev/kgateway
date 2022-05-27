---
title: Set up your gateway with an Istio sidecar
menuTitle: Configure your Gloo Edge gateway to run an Istio sidecar 
weight: 20
---

## Before you begin

Complete the following tasks before configuring an Istio sidecar for your Gloo Edge gateway: 
- Create or use an existing Kubernetes cluster. 
- Install Istio in that cluster and set up your service mesh.  
- Install Helm on your local machine.

## Configure the Gloo Edge gateway with an Istio sidecar

1. Add the Gloo Edge Helm repo. 
   ```shell
   helm repo add gloo https://storage.googleapis.com/solo-public-helm
   ```
   
2. Update the repo. 
   ```
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
    ```

Congratuliations! You successfully configured an Istio sidecar for your Gloo Edge gateway. 
