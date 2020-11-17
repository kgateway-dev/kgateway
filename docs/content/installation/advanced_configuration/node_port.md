---
title: Exposing Gloo Edge with NodePort
weight: 30
description: Exposing Gloo Edge's listeners on a Kubernetes Service Node Port
---

By default, microservices deployed in Kubernetes have an internal flat network that is not accessible from the outside of the cluster. This is true even if you use Kubernetes on a public cloud (like Amazon AWS or Google Cloud).

A NodePort service is a way to make Kubernetes services available from outside the cluster (and potentially allow access from the internet) by opening ports on all of the nodes in the cluster and allowing traffic to go directly to the pods running within the cluster.

In this document, we will review how to expose Gloo Edge via a NodePort service. It's important to note that NodePort is *not* a recommended production setting. Node port has the following drawbacks not limited to:

* You must coordinate what services use what ports so there is no conflict
* A single service can be served on a port
* You can only use ports 30000-32767
* You can run into issues if your host/node/VMs change IP addresses.

See [this article](https://medium.com/google-cloud/kubernetes-nodeport-vs-loadbalancer-vs-ingress-when-should-i-use-what-922f010849e0) for more.

---

## What is NodePort Service?

A Kubernetes cluster is composed of one or more nodes. A node VM is a (most likely) Linux machine (can be a virtual machine or bare-metal) that actually runs the Kubernetes pods. 

When a Kubernetes service is created with NodePort type, Kubernetes chooses a port number and assigns it to the service. In addition, every node in the cluster is configured to forward traffic from this port to the pods belonging service.

This allows you to access the service simply by connecting to a `node-ip:node-port` where `node-ip` is the ip of any node in the cluster, and `node-port` is the NodePort assigned by Kubernetes.

One advantage of using a NodePort is that it allows relatively easy deployment on bare metal, as it does not depend on any load-balancing component outside the cluster.

---

## How to use Gloo Edge with NodePort?

In Gloo Edge, the service that's responsible for ingress traffic is called `gateway-proxy`. To use Gloo Edge with NodePort we simply need to configure the `gateway-proxy` Kubernetes service to use NodePort. For example, when installing with Helm, use the following command:

```
helm install gloo/gloo --namespace gloo-system --set gatewayProxies.gatewayProxy.service.type=NodePort
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
    port: 80
    protocol: TCP
    targetPort: 8080
  selector:
    gloo: gateway-proxy
  sessionAffinity: None
  type: NodePort
status:
  loadBalancer: {}
```

In our example, port 30348 was allocated. You can now use `http://NODE-IP:30348` to make requests to your Gloo Edge virtual services.

If you are not using helm, or you already have Gloo Edge installed, you can delete the existing Service, and expose a node port like this:

```
kubectl -n gloo-system delete svc gateway-proxy

kubectl -n gloo-system expose deploy/gateway-proxy \
  --name gateway-proxy \
  --type NodePort  \
  --port 80 \
  --target-port 8080
```

Note, if we manually expose it like this, we will only get a single port exposed. By default Gloo Edge will use two ports, one for HTTP and one for HTTPS traffic. If you need to expose two ports, then you can do this directly in a `yaml` file.
