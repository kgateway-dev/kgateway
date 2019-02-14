---
title: Ingress Routing
weight: 4
---

Kubernetes Ingress Controllers are used for simple traffic routing into a kubernetes cluster. When Gloo is installed with the 
`glooctl install ingress` command, Gloo will configure Envoy as a Kubernetes Ingress Controller, supporting Ingress objects 
written with the annotation `kubernetes.io/ingress.class: gloo`.

### What you'll need
- [`kubectl`](https://kubernetes.io/docs/tasks/tools/install-kubectl/)
- Kubernetes v1.11.3+ deployed somewhere. [Minikube](https://kubernetes.io/docs/tasks/tools/install-minikube/) is a great way to get a cluster up quickly.

### Steps

1. The Gloo Ingress [installed](../../installation) and running on Kubernetes. 
 
1. Next, deploy the Pet Store app to kubernetes:

        kubectl apply \
         -f https://raw.githubusercontent.com/solo-io/gloo/master/example/petstore/petstore.yaml

1. Let's create a Kubernetes Ingress object to route requests to the petstore

        cat <<EOF | kubectl apply -f -
        apiVersion: extensions/v1beta1
        kind: Ingress
        metadata:
         name: petstore-ingress
         namespace: default
         annotations:
            kubernetes.io/ingress.class: gloo
        spec:
         rules:
         - http:
             paths:
             - path: /.*
               backend:
                 serviceName: petstore
                 servicePort: 8080
        EOF
        
       ingress.extensions "petstore-ingress" created

1. Let's test the route `/api/pets` using `curl`:

        export INGRESS_URL=$(glooctl proxy url --name ingress-proxy)
        curl ${INGRESS_URL}/api/pets
        
        [{"id":1,"name":"Dog","status":"available"},{"id":2,"name":"Cat","status":"pending"}]
        

Great! our ingress is up and running. See [https://kubernetes.io/docs/concepts/services-networking/ingress/](https://kubernetes.io/docs/concepts/services-networking/ingress/) for more information 
on using kubernetes ingress controllers.
