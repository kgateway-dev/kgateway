---
title: "Google Cloud Load Balancers"
description: Use Gloo Edge to complement Google Cloud load balancers
weight: 8
---

This document shows how to instantiate Google Cloud Load Balancers to complement Gloo Edge.

## Table of Contents

- [Introduction](#introduction)
- [Prerequisites](#prerequisites)
- [Network Load Balancer](#network-load-balancer)
- [HTTPS Load Balancer](#https-load-balancer)


### Introduction

You can use Gloo Edge with a Google Cloud Platform (GCP) Load Balancer, to get get benefits such as failover across availability zones.

GCP provides these different types of Load Balancers:

- Global or regional load balancing. Global load balancing means backend endpoints live in multiple regions. Regional load balancing means backend endpoints live in a single region.

- External or Internal.
  - External includes:
    - HTTP(S) Load Balancing for HTTP or HTTPS traffic, TCP Proxy for TCP traffic for ports other than 80 and 8080, without SSL offload.
    - SSL Proxy for SSL offload on ports other than 80 or 8080.
    - Network Load Balancing for TCP/UDP traffic.
  - Internal Load Balancing is a proxy-based (Envoy), regional Layer 7 load balancer that enables you to run and scale your services behind an internal IP address.

{{% notice note %}}
Google pushed load balancing out to the edge network on front-end servers, as opposed to using the traditional DNS-based approach. Thus, global load-balancing capacity can be behind a single Anycast virtual IPv4 or IPv6 address. This means you can deploy capacity in multiple regions without having to modify the DNS entries or add a new load balancer IP address for new regions.
{{% /notice %}}

#### Combining with Gloo Edge

Standard load balancers still route the traffic to machine instances where iptables are used to route traffic to individual pods running on these machines. This introduces at least one additional network hop thereby introducing latency in the packet’s journey from load balancer to the pod.

Google introduced Cloud Native Load Balancing with a new data model called Network Endpoint Group (NEG). Instead of routing to the machine and then relying on iptables to route to the pod, with NEGs the traffic goes straight to the pod.

This leads to decreased latency and an increase in throughput when compared to traffic routed with vanilla load balancers.

![NEG]({{% versioned_link_path fromRoot="/img/gcp-lb-neg.png" %}})

### Prerequisites

To use container-native load balancing, you must create a cluster with alias IPs enabled. This cluster:

- Must run GKE version 1.16.4 or later.
- Must be a VPC-native cluster.
- Must have the HttpLoadBalancing add-on enabled.

### Network Load Balancer

To connect a NLB, first, you will configure a service with SSL enabled.

Create a certificate:

```bash
openssl req -x509 -nodes -days 365 -newkey rsa:2048 \
   -keyout tls.key -out tls.crt -subj "/CN=*"
```

Then, the secret:

```bash
kubectl create secret tls upstream-tls --key tls.key \
   --cert tls.crt --namespace gloo-system
```

And the test application with its Virtual Service:

```bash
kubectl apply -f - << 'EOF' 
apiVersion: v1
kind: ServiceAccount
metadata:
  name: httpbin
---
apiVersion: v1
kind: Service
metadata:
  name: httpbin
  labels:
    app: httpbin
spec:
  type: LoadBalancer
  ports:
  - name: http
    port: 80
    targetPort: 80
  selector:
    app: httpbin
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: httpbin
spec:
  replicas: 1
  selector:
    matchLabels:
      app: httpbin
      version: v1
  template:
    metadata:
      labels:
        app: httpbin
        version: v1
    spec:
      serviceAccountName: httpbin
      containers:
      - image: docker.io/kennethreitz/httpbin
        imagePullPolicy: IfNotPresent
        name: httpbin
        ports:
        - containerPort: 80
---
apiVersion: gateway.solo.io/v1
kind: VirtualService
metadata:
  name: neg-demo
  namespace: gloo-system
spec:
# ---------------- SSL config ---------------------------
  sslConfig:
    secretRef:
      name: upstream-tls
      namespace: gloo-system
# -------------------------------------------------------
  virtualHost:
    domains:
      - 'my-gloo-edge.com'  # Notice the hostname!!
    routes:
      - matchers:
          - prefix: /
        routeAction:
            single:
              upstream:
                name: default-httpbin-80
                namespace: gloo-system
EOF
```

Test that the demo application is reachable. You should see the host name:

```bash
kubectl -n gloo-system port-forward svc/gateway-proxy 8443:443

curl -k -s https://localhost:8443/get -H "Host: my-gloo-edge.com"
```

Upgrade your Gloo Edge installation with the following Helm `values.yaml` file. This example creates 3 replicas for the default gateway proxy and adds specific GCP annotations.

```yaml
[...]
gloo:
  [...]
  gatewayProxies:
    [...]
    gatewayProxy:
      kind:
        deployment:
          replicas: 3 # You deploy three replicas of the proxy
      service:
        type: ClusterIP
        extraAnnotations:
          cloud.google.com/neg: '{ "exposed_ports":{ "443":{"name": "my-gloo-edge-nlb"} } }'
[...]
```

While this example uses a ClusterIP service, all five types of Services support standalone NEGs. Google recommends the default type, ClusterIP.

With that configuration you can see one resource automatically created:

```bash
kubectl get svcneg -A
```

And you will see:

```text
NAMESPACE     NAME                                       AGE
gloo-system   my-gloo-edge-nlb                          1m
```

You can `kubectl describe` the resources to see the status.

In the Google Cloud console, you will see the new NEG.

![New NEGs]({{% versioned_link_path fromRoot="/img/gcp-lb-nlb-negs.png" %}})

Since you have deployed three replicas, you can see there are 3 network endpoints.

#### Configuration in GCP for NLB

To instantiate a Load Balancer in Google Cloud (GCP), you need to create the following set of resources:

![GCP Resources]({{% versioned_link_path fromRoot="/img/gcp-lb-neg-resources.png" %}})

You need to configure a firewall rule to allow to allow communication between the Load Balancer and the pods in the cluster:

```bash
gcloud compute firewall-rules create my-gloo-edge-nlb-fw-allow-health-check-and-proxy \
   --action=allow \
   --direction=ingress \
   --source-ranges=0.0.0.0/0 \
   --rules=tcp:8443 \ # This is the pods port
   --target-tags <my-target-tag>
```

{{% notice note %}}
Notice that you are allowing only port `8443` which is the port for gloo-edge https connections.
{{% /notice %}}

If you did not create custom network tags for your nodes, GKE automatically generates tags for you. You can look up these generated tags by running the following command:

```bash
gcloud compute firewall-rules list --filter="name~gke-$CLUSTER_NAME-[0-9a-z]*"  --format="value(targetTags[0])"
```

In the Google Console, you can find the resource at **VPC Network -> Firewall**. You can filter by the name.

![LB Firewall]({{% versioned_link_path fromRoot="/img/gcp-lb-firewall.png" %}})

You need an address for the Load Balancer:

```bash
gcloud compute addresses create my-gloo-edge-loadbalancer-address-nlb \
    --global
```

In the Google Console, you can find the resource at **VPC Network -> External IP Addresses**. You can filter by the name.

![LB Address]({{% versioned_link_path fromRoot="/img/gcp-lb-address.png" %}})

A health check:

```bash
gcloud compute health-checks create tcp my-gloo-edge-nlb-health-check \
    --global \
    --port 8443 # This is the port for the pod. In the official documentation it might be wrong
```

{{% notice note %}}
Notice that you are checking port `8443`. It is important that you have configured the firewall rules according to the configuration you have applied here.
{{% /notice %}}

In the Google Console, you can find the resource at **Compute Engine -> Health checks**. You can filter by the name.

![LB HealthCheck]({{% versioned_link_path fromRoot="/img/gcp-lb-healthcheck.png" %}})

A backend service:

```bash
gcloud compute backend-services create my-gloo-edge-nlb-backend-service \
    --protocol=TCP \
    --health-checks my-gloo-edge-nlb-health-check \
    --global
```

In the Google Console, you can find the resource at **Network Services -> Load Balancing -> Backends tab**. You can filter by the name.

![LB BackEnd Services]({{% versioned_link_path fromRoot="/img/gcp-lb-backend.png" %}})

A target proxy:

```bash
gcloud compute target-tcp-proxies create my-gloo-edge-nlb-target-proxy \
    --backend-service=my-gloo-edge-nlb-backend-service \
    --proxy-header PROXY_V1
```

{{% notice note %}}
Notice that there are other objects for HTTP and HTTPS called `target-http-proxies` and `target-https-proxies`.
{{% /notice %}}

You can find the resource enabling the **Advanced menu** within the **Network Services -> Load Balancing**.

![LB Forwarding Rules]({{% versioned_link_path fromRoot="/img/gcp-lb-advanced-lb.png" %}})

And then the **Target Proxies tab**.

![LB Forwarding Rules]({{% versioned_link_path fromRoot="/img/gcp-lb-targetproxies.png" %}})

A forwarding-rule:

```bash
gcloud compute forwarding-rules create my-gloo-edge-nlb-content-rule \
    --address=my-gloo-edge-loadbalancer-address-nlb \
    --global \
    --target-tcp-proxy my-gloo-edge-nlb-target-proxy \
    --ports=443
```

In the Google Console, you can find the resource as part of the **Advanced Menu** at **Network Services -> Load Balancing -> Frontends tab**. You can filter by the name.

![LB Forwarding Rules]({{% versioned_link_path fromRoot="/img/gcp-lb-forwardingrules.png" %}})

And you need to attach the NEG to the backend service:

```bash
gcloud compute backend-services add-backend my-gloo-edge-nlb-backend-service \
    --network-endpoint-group=my-gloo-edge-nlb \
    --balancing-mode CONNECTION \
    --max-connections-per-endpoint 5 \
    --network-endpoint-group-zone <my-cluster-zone> \
    --global
```

Where `<my-cluster-zone>` is the zone where the cluster has been deployed.

After adding the Backend Service to the NEG, you will see the Health Check becoming green:

![NEG Health Checks]({{% versioned_link_path fromRoot="/img/gcp-lb-neg-hc.png" %}})

Finally, let's test the connectivity through the Load Balancer:

```bash
APP_IP=$(gcloud compute addresses describe my-gloo-edge-loadbalancer-address-nlb --global --format=json | jq -r '.address')

curl -k https://34.117.34.186/get -H "Host: my-gloo-edge.com"
```

The application should be accessible through the Load Balancer.


### HTTPS Load Balancer

To connect a HTTPS Load Balancer, first, you will configure a Gloo Edge Virtual Service.

```bash
kubectl apply -f - << 'EOF' 
apiVersion: v1
kind: ServiceAccount
metadata:
  name: httpbin
---
apiVersion: v1
kind: Service
metadata:
  name: httpbin
  labels:
    app: httpbin
spec:
  type: LoadBalancer
  ports:
  - name: http
    port: 80
    targetPort: 80
  selector:
    app: httpbin
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: httpbin
spec:
  replicas: 1
  selector:
    matchLabels:
      app: httpbin
      version: v1
  template:
    metadata:
      labels:
        app: httpbin
        version: v1
    spec:
      serviceAccountName: httpbin
      containers:
      - image: docker.io/kennethreitz/httpbin
        imagePullPolicy: IfNotPresent
        name: httpbin
        ports:
        - containerPort: 80
---
apiVersion: gateway.solo.io/v1
kind: VirtualService
metadata:
  name: neg-demo
  namespace: gloo-system
spec:
  virtualHost:
    domains:
      - 'my-gloo-edge.com'  # Notice the hostname!!
    routes:
      - matchers:
          - prefix: /
        routeAction:
            single:
              upstream:
                name: default-httpbin-80
                namespace: gloo-system
EOF
```

Test that the demo application is reachable. You should see the host name:

```bash
kubectl -n gloo-system port-forward svc/gateway-proxy 8080:80

curl -k -s http://localhost:8080/get -H "Host: my-gloo-edge.com"
```

Upgrade your Gloo Edge installation with the following Helm `values.yaml` file. This example creates 3 replicas for the default gateway proxy and adds specific GCP annotations.

```yaml
[...]
gloo:
  [...]
  gatewayProxies:
    [...]
    gatewayProxy:
      kind:
        deployment:
          replicas: 3 # You deploy three replicas of the proxy
      service:
        type: ClusterIP
        extraAnnotations:
          cloud.google.com/neg: '{ "exposed_ports":{ "80":{"name": "my-gloo-edge-https"} } }'
[...]
```

While this example uses a ClusterIP service, all five types of Services support standalone NEGs. Google recommends the default type, ClusterIP.

With that configuration you can see one resource automatically created:

```bash
kubectl get svcneg -A
```

And you will see:

```text
NAMESPACE     NAME                                       AGE
gloo-system   my-gloo-edge-https                          1m
```

You can `kubectl describe` the resources to see the status.

And, in the google cloud console, you will see the new NEG.

![New NEGs]({{% versioned_link_path fromRoot="/img/gcp-lb-nlb-negs.png" %}})

Since you have deployed three replicas, you can see there are 3 network endpoints.

#### Configuration in GCP for HTTPS Load Balancer

To instantiate a Load Balancer in Google Cloud (GCP), you need to create the following set of resources.

![GCP Resources]({{% versioned_link_path fromRoot="/img/gcp-lb-neg-resources.png" %}})

You need to configure a firewall rule to allow to allow communication between the Load Balancer and the pods in the cluster:

```bash
gcloud compute firewall-rules create my-gloo-edge-https-fw-allow-health-check-and-proxy \
   --action=allow \
   --direction=ingress \
   --source-ranges=0.0.0.0/0 \
   --rules=tcp:8080 \ # This is the pods port
   --target-tags <my-target-tag>
```

{{% notice note %}}
Notice that you are allowing only port `8080` which is the port for gloo-edge http connections.
{{% /notice %}}

If you did not create custom network tags for your nodes, GKE automatically generates tags for you. You can look up these generated tags by running the following command:

```bash
gcloud compute firewall-rules list --filter="name~gke-$CLUSTER_NAME-[0-9a-z]*"  --format="value(targetTags[0])"
```

In the Google Console, you can find the resource at **VPC Network -> Firewall**. You can filter by the name.

![LB Firewall]({{% versioned_link_path fromRoot="/img/gcp-lb-firewall.png" %}})

You need an address for the Load Balancer:

```bash
gcloud compute addresses create my-gloo-edge-loadbalancer-address-https \
    --global
```

In the Google Console, you can find the resource at **VPC Network -> External IP Addresses**. You can filter by the name.

![LB Address]({{% versioned_link_path fromRoot="/img/gcp-lb-address.png" %}})

A health check:

```bash
gcloud compute health-checks create tcp my-gloo-edge-https-health-check \
    --global \
    --port 8080 # This is the port for the pod. In the official documentation it might be wrong
```

{{% notice note %}}
Notice that you are checking port `8080`. It is important that you have configured the firewall rules according to the configuration you have applied here.
{{% /notice %}}

In the Google Console, you can find the resource at **Compute Engine -> Health checks**. You can filter by the name.

![LB HealthCheck]({{% versioned_link_path fromRoot="/img/gcp-lb-healthcheck.png" %}})

A backend service:

```bash
gcloud compute backend-services create my-gloo-edge-https-backend-service \
    --protocol=HTTP \
    --health-checks my-gloo-edge-https-health-check \
    --global
```

In the Google Console, you can find the resource at **Network Services -> Load Balancing -> Backends tab**. You can filter by the name.

![LB BackEnd Services]({{% versioned_link_path fromRoot="/img/gcp-lb-backend.png" %}})


A URL-map:

```bash
gcloud compute url-maps create my-gloo-edge-https-url-map \
    --default-service my-gloo-edge-https-backend-service \
    --global
```

In the Google Console, you can find the resource at **Network Services -> Load Balancing -> Load Balancers tab**. You can filter by the name.

![LB URL Map]({{% versioned_link_path fromRoot="/img/gcp-lb-urlmap.png" %}})

Create a self-signed certificate and a `ssl-certificate`:

```bash
openssl genrsa -out ca.key 2048
openssl req -x509 -new -nodes -key ca.key -days 100000 -out ca.crt -subj "/CN=*"
gcloud compute ssl-certificates create my-gloo-edge-https-certificate \
    --certificate=ca.crt \
    --private-key=ca.key \
    --global
```

A target proxy:

```bash
gcloud compute target-https-proxies create my-gloo-edge-https-target-proxy \
    --url-map=my-gloo-edge-https-url-map \
    --ssl-certificates=my-gloo-edge-https-certificate \
    --global
```

{{% notice note %}}
Notice that there are other objects for HTTP and TCP called `target-http-proxies` and `target-tcp-proxies`.
{{% /notice %}}

You can find the resource at **Network Services -> Load Balancing -> Target Proxies tab**. You can filter by the name.

![LB Forwarding Rules]({{% versioned_link_path fromRoot="/img/gcp-lb-targetproxies.png" %}})

A forwarding-rule:

```bash
gcloud compute forwarding-rules create my-gloo-edge-https-content-rule \
    --address=my-gloo-edge-loadbalancer-address-https \
    --global \
    --target-https-proxy my-gloo-edge-https-target-proxy \
    --ports=443
```

In the Google Console, you can find the resource at **Network Services -> Load Balancing -> Frontends tab**. You can filter by the name.

![LB Forwarding Rules]({{% versioned_link_path fromRoot="/img/gcp-lb-forwardingrules.png" %}})

And you need to attach the NEG to the backend service:

```bash
gcloud compute backend-services add-backend my-gloo-edge-https-backend-service \
    --network-endpoint-group=my-gloo-edge-https \
    --balancing-mode RATE \
    --max-rate-per-endpoint 5 \
    --network-endpoint-group-zone <my-cluster-zone> \
    --global
```

Where `<my-cluster-zone>` is the zone where the cluster has been deployed.

Finally, let's test the connectivity through the Load Balancer:

```bash
APP_IP=$(gcloud compute addresses describe my-gloo-edge-loadbalancer-address-https --global --format=json | jq -r '.address')

curl -k "https://${APP_IP2}/get" -H "Host: my-gloo-edge.com"
```

The application should be accessible through the Load Balancer.
