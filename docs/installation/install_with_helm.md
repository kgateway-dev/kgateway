# Installing Gloo with Helm

This document outlines instructions for the setup and configuration of Gloo using Helm. This is the recommended install method for installing Gloo to your production environment as it offers rich customization to the Gloo control plane and the proxies Gloo manages.

## Installation

To install with Helm:

1. Add the Gloo Helm repository: 

        helm repo add gloo https://storage.googleapis.com/solo-public-helm
        
1. (Optional) Customize the Gloo installation with a `value-overrides.yaml` file: 

        # examle value-overerides.yaml
        namespace:
          create: false
        settings:
          integrations:
            knative:
              enabled: true
          watchNamespaces: []
          writeNamespace: mycustomnamespace   

     See [customizing Helm options](#Customizing-Helm-Options) for the full list of values and their purpose.

1. Install Gloo from the Helm Repository: 
    
        helm install gloo/gloo 
    
     If you're using custom overrides:

        helm install gloo/gloo --values value-overerides.yaml


<a name="Customizing-Helm-Options"></a>
## Customizing Helm Options

option | type | description
--- | --- | ---
$.namespace.create | bool | create the installation namespace
$.rbac.create | bool | create rbac rules for the gloo-system service account
$.settings.watchNamespaces | []string | whitelist of namespaces for gloo to watch for services and CRDs. leave empty to use all namespaces
$.settings.writeNamespace | string | namespace where intermediary CRDs will be written to, e.g. Upstreams written by Gloo Discovery.
$.settings.integrations.knative.enabled | bool | enable Gloo to serve as a cluster ingress controller for Knative Serving
$.settings.integrations.knative.proxy.image.repository | string | image name (registry/repository) for the knative proxy container. This proxy is configured automatically by Knative as the Knative Cluster Ingress.
$.settings.integrations.knative.proxy.image.tag | string | tag for the knative proxy container 
$.settings.integrations.knative.proxy.image.pullPolicy | string | image pull policy for the knative proxy container 
$.settings.integrations.knative.proxy.httpPort | string | HTTP port for the proxy
$.settings.integrations.knative.proxy.httpsPort | string | HTTPS port for the proxy
$.settings.integrations.knative.proxy.replicas | int | number of proxy instances to deploy
$.settings.create | bool | create a Settings CRD which configures Gloo controllers at boot time
$.gloo.deployment.image.repository | string | image name (registry/repository) for the gloo container. this container is the core controller of the system which watches CRDs and serves Envoy configuration over xDS
$.gloo.deployment.image.tag | string | tag for the gloo container
$.gloo.deployment.image.pullPolicy | string | image pull policy for gloo container
$.gloo.deployment.xdsPort | string | port where gloo serves xDS API to Envoy
$.gloo.deployment.replicas | int | number of gloo xds server instances to deploy
$.discovery.deployment.image.repository | string | image name (registry/repository) for the discovery container. this container adds service discovery and function discovery to Gloo 
$.discovery.deployment.image.tag | string | tag for the discovery container
$.discovery.deployment.image.pullPolicy | string | image pull policy for discovery container
$.gateway.enabled | bool | enable Gloo API Gateway features
$.gateway.deployment.image.repository | string | image name (registry/repository) for the gateway controller container. this container translates Gloo's VirtualService CRDs to the intermediary representation used by the gloo controller
$.gateway.deployment.image.tag | string | tag for the gateway controller container
$.gateway.deployment.image.pullPolicy | string | image pull policy for the gateway controller container
$.gatewayProxy.deployment.image.repository | string | image name (registry/repository) for the gateway proxy container. this proxy receives configuration created via VirtualService CRDs
$.gatewayProxy.deployment.image.tag | string | tag for the gateway proxy container
$.gatewayProxy.deployment.image.pullPolicy | string | image pull policy for the gateway proxy container
$.gatewayProxy.deployment.httpPort | string | HTTP port for the proxy
$.gatewayProxy.deployment.replicas | int | number of gateway proxy instances to deploy
$.ingress.enabled | bool | enable Gloo to function as a standard Kubernetes Ingress Controller (i.e. configure via [Kubernetes Ingress objects](https://kubernetes.io/docs/concepts/services-networking/ingress/)) 
$.ingress.deployment.image.repository | string | image name (registry/repository) for the ingress controller container. this container translates [Kubernetes Ingress objects](https://kubernetes.io/docs/concepts/services-networking/ingress/) to the intermediary representation used by the gloo controller
$.ingress.deployment.image.tag | string | tag for the ingress controller container
$.ingress.deployment.image.pullPolicy | string | image pull policy for the ingress controller container
$.ingressProxy.deployment.image.tag | string | tag for the ingress proxy container
$.ingressProxy.deployment.image.repository | string | image name (registry/repository) for the ingress proxy container. this proxy receives configuration created via Kubernetes Ingress objects
$.ingressProxy.deployment.image.pullPolicy | string | image pull policy for the ingress proxy container
$.ingressProxy.deployment.httpPort | string | HTTP port for the proxy
$.ingressProxy.deployment.httpsPort | string | HTTPS port for the proxy
$.ingressProxy.deployment.replicas | int | number of ingress proxy instances to deploy
