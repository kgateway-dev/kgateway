---
title: Production Deployment Using NodePort
weight: 7
---

## Motivation
By default, micro-services deployed in kubernetes have an internal flat network that is not accessible from the outside
of the cluster. This is true even if you use kuberentes on a public cloud (like Amazon AWs or Google Cloud).

A NodePort is a way to make kubernetes services available from outside the cluster (and potentially allow access from the internet).

In this document we will review how to expose Gloo via a NodePort.


## What is NodePort Service?
A kuberenetes cluster is composed of one or more nodes. A node VM is a (most likely) linux machine (can be a virtual machine or bare-metal) that actually runs the kuberenetes pods. 

When a kubernetes service is created with NodePort type, kubernetes chooses a port number and assigns it to the service. In addition, every node in the cluster is configured to forward traffic from this port to the pods belonging service.

This allows you to access the service simply by connecting to a 'node-ip:node-port' where node-ip is the ip of any node in the cluster, and node-port is the NodePort assigned by kuberentes.

One advantage of using a NodePort is that it allows relatively easy deployment on bare metal, as it does not depend on any load balancing component outside the cluster.

## How to use Gloo with NodePort?

In Gloo, the service that's responsible for ingress traffic is called "gateway-proxy". To use Gloo with NodePort
we simply need to configure this service to NodePort. For example, when installing with helm,
use the following command:

```
helm install gloo/gloo --namespace gloo-system --set gatewayProxy.service.type=NodePort
```

Once installed, check what port was allocated:
```
kubectl get svc -n gloo-system gateway-proxy -o yaml

apiVersion: v1
kind: Service
metadata:
  labels:
    app: gloo
    gloo: gateway-proxy
  name: gateway-proxy
  namespace: gloo-system
spec:
  clusterIP: 10.106.198.61
  externalTrafficPolicy: Cluster
  ports:
  - name: http
    nodePort: 30348
    port: 8080
    protocol: TCP
    targetPort: 8080
  selector:
    gloo: gateway-proxy
  sessionAffinity: None
  type: NodePort
status:
  loadBalancer: {}
```

In our example, port 30348 was allocated. You can now use `http://NODE-IP:30348` to make requests to your
Gloo virtual services.
