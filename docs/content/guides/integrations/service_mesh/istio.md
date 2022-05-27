---
title: Set up your gateway with an Istio sidecar
menuTitle: Configure your Gloo Edge gateway to run an Istio sidecar 
weight: 20
---

## Before you begin

To inject an Istio sidecar into the Gloo Edge gateway, you must have a Kubernetes cluster where you installed [Istio](). 
- Installed Helm on your local machine

## Configure the Gloo Edge gateway with an Istio sidecar

1. Add the Gloo Edge Helm repo. 
   ```shell
   helm repo add glooe https://storage.googleapis.com/gloo-ee-helm
   ```
   
2. Update the repo. 
   ```
   helm update
   ```
   
3. Create a `value-overrides.yaml` file with the following content. To configure your gateway with an Istio sidecar, make sure to add the `istioIntegration` section and set the `enableIstioSidecarOnGateway` option to `true`. 
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
   
4. Install Gloo Edge with the settings in the `value-overrides.yaml` file.  
   ```shell
   helm install gloo glooe/gloo-ee --namespace gloo-system \
   -f value-overrides.yaml --create-namespace --set-string license_key=YOUR_LICENSE_KEY
   ```
   
   If you already installed Gloo Edge on your cluster
   

