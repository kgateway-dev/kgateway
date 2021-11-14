---
title: "Google Cloud Load Balancers"
description: Use Gloo Edge to complement Google Cloud load balancers
weight: 8
---

Standard load balancers still route the traffic to machine instances where iptables are used to route traffic to individual pods running on these machines. This introduces at least one additional network hop thereby introducing latency in the packet’s journey from load balancer to the pod.

Google introduced Cloud Native Load Balancing with a new data model called Network Endpoint Group (NEG). Instead of routing to the machine and then relying on iptables to route to the pod, with NEGs the traffic goes straight to the pod.

This leads to decreased latency and an increase in throughput when compared to traffic routed with vanilla load balancers.

## Create Network Endpoint Group automatically

To use container-native load balancing, you must create a cluster with alias IPs enabled. This cluster:

- Must run GKE version 1.16.4 or later.
- Must be a VPC-native cluster.
- Must have the HttpLoadBalancing add-on enabled.

This article assumes that the cluster satisfies the requirements and Gloo Edge is installed in the cluster.

As well, an upstream is up and running with a valid VirtualService to be reachable from the loadbalancer.

You can use following example:

```bash
kubectl apply -f - << 'EOF' 
apiVersion: apps/v1
kind: Deployment
metadata:
  labels:
    run: neg-demo-app
  name: neg-demo-app
spec:
  replicas: 3
  selector:
    matchLabels:
      run: neg-demo-app
  template:
    metadata:
      labels:
        run: neg-demo-app
    spec:
      containers:
      - image: k8s.gcr.io/serve_hostname:v1.4
        name: hostname
---
apiVersion: v1
kind: Service
metadata:
  name: neg-demo-svc
spec:
  type: ClusterIP
  selector:
    run: neg-demo-app
  ports:
  - port: 80
    protocol: TCP
    targetPort: 9376
---
apiVersion: gateway.solo.io/v1
kind: VirtualService
metadata:
  name: neg-demo
  namespace: gloo-system
spec:
  virtualHost:
    domains:
      - 'my-gloo-edge.com'
    routes:
      - matchers:
          - prefix: /
        routeAction:
            single:
              upstream:
                name: default-neg-demo-svc-80
                namespace: gloo-system
EOF
```

Test that the demo application is reachable. You should see the host name:

```bash
curl -s $(glooctl proxy url --port http)/
```

Upgrade your gloo installation with following attributes in your helm `values.yaml`. This will create 3 replicas for the default gateway proxy and will add specific GCP annotations:

```yaml
gloo:
  gatewayProxies:
    gatewayProxy:
      kind:
        deployment:
          replicas: 3 # You deploy three replicas of the proxy
      service:
        type: ClusterIP
        extraAnnotations:
          cloud.google.com/neg: '{ "exposed_ports":{ "80":{"name": "my-gloo-edge-http"}, "443":{"name": "my-gloo-edge-https"} } }'
```

While this example uses a ClusterIP service, all five types of Services support standalone NEGs. Google recommends the default type, ClusterIP.

With that configuration you can see two new resources automatically created:

```
kubectl get svcneg -A
```

And you will see:

```
NAMESPACE     NAME                                       AGE
gloo-system   my-gloo-edge-http                          1m
gloo-system   my-gloo-edge-https                         1m
```

You can `kubectl describe` the resources to see the status.

And in the google cloud console, you will see the new NEGs.

![New NEGs]({{% versioned_link_path fromRoot="/img/new-negs.png" %}})

Since you have deployed three replicas, you can see there are 3 network endpoints per each NEG.

## Attach the NEG to a LoadBalancer

Google offer different types of LoadBalancers. To find out which one fits more in your requirements, have a look at this [article](https://cloud.google.com/load-balancing/docs/choosing-load-balancer)

In this article we will show how to setup an external HTTPS LoadBalancer and an external TCP LoadBalancer.

To instantiate a LoadBalancer in Google Clooud (GCP), you need to create following set of resources:

![GCP Resources]({{% versioned_link_path fromRoot="/img/neg-resources.png" %}})

### External HTTPS LoadBalancer

You need to configure a firewall rule to allow to allow communication between the loadbalancer and the pods in the cluster:

```bash
gcloud compute firewall-rules create my-gloo-edge-fw-allow-health-check-and-proxy \
   --action=allow \
   --direction=ingress \
   --source-ranges=0.0.0.0/0 \
   --rules=tcp:8080 \ # This is the pods port
   --target-tags <my-target-tag>
```

{{% notice note %}}
Notice that you are allowing only port `8080` which is the port for gloo-edge http connection. For other scenarios, you might need to open `8443` and different protocols.
{{% /notice %}}

If you did not create custom network tags for your nodes, GKE automatically generates tags for you. You can look up these generated tags by running the following command:

```
gcloud compute firewall-rules list --filter="name~gke-$CLUSTER_NAME-[0-9a-z]*"  --format="value(targetTags[0])"
```

In the Gloogle Console, you can find the resource at **VPC Network -> Firewall**. You can filter by the name.

![LB Firewall]({{% versioned_link_path fromRoot="/img/firewall.png" %}})

You need an address for the LoadBalancer:

```
gcloud compute addresses create my-gloo-edge-loadbalancer-address-https \
    --global
```

In the Gloogle Console, you can find the resource at **VPC Network -> External IP Addresses**. You can filter by the name.

![LB Address]({{% versioned_link_path fromRoot="/img/address.png" %}})

A health check:

```
gcloud compute health-checks create http my-gloo-edge-loadbalancer-http-health-check \
    --global \
    --port 8080 # This is the port for the pod. In the official documentation it imght be wrong
```

{{% notice note %}}
Notice that you are checking port `8080`. It is important that you have configured the firewall rules accordingly to the configuration you have applied here.
{{% /notice %}}

In the Gloogle Console, you can find the resource at **Compute Engine -> Health checks**. You can filter by the name.

![LB HealthCheck]({{% versioned_link_path fromRoot="/img/healthcheck.png" %}})

A backend service:

```
gcloud compute backend-services create my-gloo-edge-backend-service-http \
    --protocol=HTTP \
    --health-checks my-gloo-edge-loadbalancer-http-health-check \
    --global
```

In the Gloogle Console, you can find the resource at **Network Services -> Load Balancing -> Backends tab**. You can filter by the name.

![LB BackEnd Services]({{% versioned_link_path fromRoot="/img/backend.png" %}})


A URL-map:

```
gcloud compute url-maps create my-gloo-edge-loadbalancer-http \
    --default-service my-gloo-edge-backend-service-http \
    --global
```

In the Gloogle Console, you can find the resource at **Network Services -> Load Balancing -> Load Balancers tab**. You can filter by the name.

![LB URL Map]({{% versioned_link_path fromRoot="/img/urlmap.png" %}})

Create a self-signed certificate and a `ssl-certificate`:

```
openssl genrsa -out ca.key 2048
openssl req -x509 -new -nodes -key ca.key -days 100000 -out ca.crt -subj "/CN=*"
gcloud compute ssl-certificates create my-gloo-edge-loadbalancer-https \
    --certificate=ca.crt \
    --private-key=ca.key \
    --global
```

A target proxy:

```
gcloud compute target-https-proxies create my-gloo-edge-loadbalancer-https-lb-target-proxy \
    --url-map=my-gloo-edge-loadbalancer-http \
    --ssl-certificates=my-gloo-edge-loadbalancer-https \
    --global
```

{{% notice note %}}
Notice that there is another object with name `target-http-proxies` which is used for HTTP.
{{% /notice %}}

{{% notice note %}}
This feature belongs to [Load Balancer advanced configuration](https://console.cloud.google.com/net-services/loadbalancing/advanced/targetProxies/list)
{{% /notice %}}

You can find the resource at **Network Services -> Load Balancing -> Target Proxies tab**. You can filter by the name.

![LB Forwarding Rules]({{% versioned_link_path fromRoot="/img/targetproxies.png" %}})

A forwarding-rule:

```
gcloud compute forwarding-rules create my-gloo-edge-loadbalancer-http-content-rule \
    --address=my-gloo-edge-loadbalancer-address-https \
    --global \
    --target-https-proxy my-gloo-edge-loadbalancer-https-lb-target-proxy \
    --ports=443
```

In the Gloogle Console, you can find the resource at **Network Services -> Load Balancing -> Frontends tab**. You can filter by the name.

![LB Forwarding Rules]({{% versioned_link_path fromRoot="/img/forwardingrules.png" %}})

And you need to attach the NEG to the backend service:

```
gcloud compute backend-services add-backend my-gloo-edge-backend-service-http \
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

curl -k "https://${APP_IP}"
```
