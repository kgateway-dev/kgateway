---
title: Getting Started with Knative
description: Deploy and invoke a Hello-World container with Knative and Gloo Edge.
weight: 10
---

For the purpose of running Knative, Gloo Edge can function as a complete replacement for Istio (supporting all documented Knative features), requiring less resource usage and operational overhead. 

This guide walks you through running a serverless app with Knative, using Gloo Edge as your ingress.
 
It assumes you've already followed the [installation guide for Gloo Edge and Knative]({{% versioned_link_path fromRoot="/installation/knative" %}}). 

### Before you start

Gloo Edge supports the features and tutorials that can be found in the [Knative documentation](https://knative.dev). 

When following tutorials in the Knative documentation, please note that you'll need to use the address of the `knative-external-proxy` service in `gloo-system` for requests to knative services rather than the `knative-ingressgateway` in `istio-system`, which is the default assumed by those tutorials.

{{% notice note %}}
To get the URL of the Gloo Edge Knative gateway, 
run `glooctl proxy url --name knative-external-proxy`
{{% /notice %}}

### What you'll need
- [`kubectl`](https://kubernetes.io/docs/tasks/tools/install-kubectl/)
- Kubernetes v1.14+ deployed somewhere. [Minikube](https://kubernetes.io/docs/tasks/tools/install-minikube/) is a great way to get a cluster up quickly.
- [Docker](https://www.docker.com) installed and running on your local machine, and a Docker Hub account configured (we'll use it for a container registry).

### Steps

1. First, [ensure Knative is installed with Gloo Edge]({{% versioned_link_path fromRoot="/installation/knative" %}}). 
 
1. Next, create a `Knative Service`

     For this demo, a simple helloworld application written in go will be used.
     Copy the YAML below to a file called `helloworld-go.yaml` and apply it with
     `kubectl`
  
    ```yaml
    apiVersion: serving.knative.dev/v1alpha1
    kind: Service
    metadata:
      name: helloworld-go
      namespace: default
    spec:
      template:
        spec:
          containers:
            - image: gcr.io/knative-samples/helloworld-go
              env:
                - name: TARGET
                  value: Go Sample v1
    ```
  
    ```
    kubectl apply -f helloworld-go.yaml
    ```

2. Send a request

     **Knative Services** are exposed via the *Host* header assigned by Knative. By
     default, Knative will use the header `Host`:
     `{service-name}.{namespace}.example.com`. You can discover the appropriate *Host* header by checking the URL Knative has assigned to the `helloworld-go` service created above.
  
     ```
     kubectl get ksvc helloworld-go -n default  --output=custom-columns=NAME:.metadata.name,URL:.status.url
     ```

     returns

     ```
     NAME            URL
     helloworld-go   http://helloworld-go.default.example.com
     ```
  
     Gloo Edge will use the `Host` header to route requests to the correct
     service. You can send a request to the `helloworld-go` service with curl
     using the `Host` and URL of the Gloo Edge gateway from above:
  
     ```
     curl -H "Host: helloworld-go.default.example.com" $(glooctl proxy url --name knative-external-proxy)
     ```

     returns

     ```
     Hello Go Sample v1!
     ```

Congratulations! You have successfully installed Knative with Gloo Edge to manage and route to serverless applications! Try out some of the more advanced tutorials for Knative in [the Knative documentation](https://knative.dev/docs/).
