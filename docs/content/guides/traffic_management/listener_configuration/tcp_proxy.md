---
title: TCP Proxy
weight: 30
description: Learn how to use Gloo Edge as a simple TCP proxy
---

In this tutorial, we'll take a look at using Gloo Edge as a TCP proxy. Envoy is an L4 proxy by default and is therefore
more than up to the task. Gloo Edge's TCP routing features are slightly different than the rest of Gloo Edge's routing as a result
of the relative simplicity of TCP level routing. Current features include standard routing, SSL, and Server Name Indication (SNI) domain matching.

---

## Resources 

Gloo Edge uses the gateway Custom Resource (CR) to configure the TCP proxy settings. The gateway CRs are combined to form a Proxy CR, which is used to generate the configuration for the Envoy proxy. You can read more about the gateway and proxy API at the links below.

- {{< protobuf name="gateway.solo.io.Gateway" display="Gateway">}}
- {{< protobuf name="gloo.solo.io.Proxy" display="Proxy">}}

---

## What you'll need

To complete this guide you will need the following items:

* Gloo Edge installed on Kubernetes and access to that Kubernetes cluster. Please refer to the [Gloo Edge installation]({{< versioned_link_path fromRoot="/installation" >}}) for guidance on installing Gloo Edge into Kubernetes.

* Access from the Kubernetes cluster to an external API. 

* A TCP service running in cluster. For the purposes of this tutorial we will use a basic tcp-echo pod.

---

## Configuring the TCP proxy

### Deploy the tcp-echo pod

Firstly apply the following yaml into the namespace of your choice. For the purposes of this tutorial we will be using `gloo-system`

```bash
kubectl apply -n gloo-system -f - <<EOF
apiVersion: v1
kind: Pod
metadata:
  labels:
    gloo: tcp-echo
  name: tcp-echo
spec:
  containers:
  - image: soloio/tcp-echo:latest
    imagePullPolicy: IfNotPresent
    name: tcp-echo
  restartPolicy: Always
---
apiVersion: v1
kind: Service
metadata:
  labels:
    app: gloo
  name: tcp-echo
spec:
  ports:
  - name: http
    port: 1025
    protocol: TCP
    targetPort: 1025
  selector:
    gloo: tcp-echo
EOF
```

Once the `tcp-echo` pod is up and running we are ready to create our gateway resource and begin routing to it.

### Provision the gateway CR

The gateway will contain the following: 
```bash
kubectl apply -n gloo-system -f - <<EOF
apiVersion: gateway.solo.io/v1
kind: Gateway
metadata:
  name: tcp
  namespace: gloo-system
spec:
  bindAddress: '::'
  bindPort: 8000
  tcpGateway:
    tcpHosts:
    - name: one
      destination:
        single:
          upstream:
            name: gloo-system-tcp-echo-1025
            namespace: gloo-system
  useProxyProto: false
EOF
```

To check that the gateway has been created properly run:
```bash
kubectl get gateways.gateway.solo.io -A
```

```
NAMESPACE     NAME          AGE
gloo-system   gateway       26h
gloo-system   gateway-ssl   26h
gloo-system   tcp           5s
```

Gloo Edge reads and combines the gateway configuration with the other gateways into an Envoy proxy configuration. To make sure that the configuration has been translated properly, check the proxy.

```shell script
glooctl get proxy <proxy> -o yaml
```

{{< highlight yaml "hl_lines=6-13" >}}
...
  spec:
    listeners:
    - bindAddress: '::'
      bindPort: 8000
      name: listener-::-8000
      tcpListener:
        tcpHosts:
        - destination:
            single:
              upstream:
                name: gloo-system-tcp-echo-1025
                namespace: gloo-system
          name: one
      useProxyProto: false
  status:
    reported_by: gloo
    state: 1
kind: List
metadata:
  resourceVersion: ""
  selfLink: ""
{{< /highlight >}}


If the translation worked, the listeners array in the resource spec will contain an entry for the TCP service we will be routing to. Once the state on the resource is recorded as `1` the service is ready to be routed to.

The next step is adding a port to the gateway-proxy service so we can route to the Envoy listener which is handling our TCP traffic.

### Add a port to the gateway-proxy

The service should look like the following:

{{< highlight yaml "hl_lines=23-27" >}}
apiVersion: v1
kind: Service
metadata:
  labels:
    app: gloo
    gloo: gateway-proxy
  name: gateway-proxy
  namespace: gloo-systemservices/gateway-proxy
  uid: 1b624541-9f3e-11e9-8766-3e49e0c5bb1c
spec:
  externalTrafficPolicy: Cluster
  ports:
  - name: http
    nodePort: 32302
    port: 80
    protocol: TCP
    targetPort: 8080
  - name: https
    nodePort: 30440
    port: 443
    protocol: TCP
    targetPort: 8443
  - name: tcp
    nodePort: 30197
    port: 8000
    protocol: TCP
    targetPort: 8000
  selector:
    gloo: gateway-proxy
  sessionAffinity: None
  type: LoadBalancer
status:
  loadBalancer: {}
{{< /highlight >}}

The important part here is the entry on port `8000` for our TCP service. Once the service has been saved to Kubernetes, get the NodePort from the service port entry and save it for later.

The next and final step is routing to the service.

### Routing to the TCP service

This step assumes you are running on a local minikube instance.

```bash
curl -v telnet://$(minikube ip):30197
```

```
* Rebuilt URL to: telnet://192.168.64.13:30197/
*   Trying 192.168.64.13...
* TCP_NODELAY set
* Connected to 192.168.64.13 (192.168.64.13) port 30197 (#0)

```

{{% notice note %}}
note: The node port was inserted in the above command following the ip
{{% /notice %}}


Now the terminal is waiting for input. All input entered will be echo'd back. An inspection of the logs from the pod will reveal that the data is in fact being proxied through.

---

## Next Steps

In this guide you saw how Gloo Edge can be configured as a TCP proxy for services that do not use HTTP/S. Gloo Edge can also handle [gRPC-Web clients]({{< versioned_link_path fromRoot="/guides/traffic_management/listener_configuration/grpc_web/" >}}) and [Websockets]({{< versioned_link_path fromRoot="/guides/traffic_management/listener_configuration/websockets/" >}}). Check out those guides for more information.
