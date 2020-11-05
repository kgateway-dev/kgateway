---
title: gRPC
weight: 125
description: Routing to gRPC services with a gRPC client
---

gRPC has become a popular, high-performance framework used by many applications. In this guide, we will show you how to expose a gRPC `Upstream` through a Gloo Edge `Virtual Service` and connect to it with a gRPC client. Once we have basic connectivity, we will add in TLS connectivity between the gRPC client and the Gloo Edge proxy (Envoy).

In this guide we are going to:

1. Deploy a gRPC demo service
1. Verify that the gRPC descriptors were indeed discovered
1. Add a Virtual Service that maps to the gRPC API
1. Verify that everything is working as expected
1. Add TLS and a domain to the Virtual Service and verify again

Let's get started!

---

## Prerequisites

To follow along with this guide, you will need to have a [Kubernetes cluster deployed]({{% versioned_link_path fromRoot="/installation/platform_configuration/cluster_setup/" %}}) with [Gloo Edge installed]({{% versioned_link_path fromRoot="/installation/gateway/kubernetes/" %}}). You will also [need the tool](https://github.com/fullstorydev/grpcurl) `grpcurl`, aka curl for gRPC, to act as the gRPC client for testing communications. Finally, we will be using openssl to generate a self-signed certificate for TLS.

---

## Deploy the demo gRPC store

We have a container image on Docker Hub which has a simple Store service for gRPC. We are going to deploy that image and expose it using port 80.

Create a deployment and a service:

```shell
kubectl create deployment grpcstore-demo --image=docker.io/soloio/grpcstore-demo
kubectl expose deployment grpcstore-demo --port 80 --target-port=8080
```

### Verify that gRPC functions were discovered

After a few seconds Gloo Edge should have discovered the service:

```shell
kubectl get upstream -n gloo-system default-grpcstore-demo-80
```

We should also enable Gloo Edge FDS, if it is not already (whitelist mode by default), so the proto descriptor is found:

```shell script
kubectl label upstream -n gloo-system default-grpcstore-demo-80 discovery.solo.io/function_discovery=enabled
```

FDS should update the discovered upstream:

```shell
kubectl get upstream -n gloo-system default-grpcstore-demo-80 -o yaml
```

You should see output similar to this:

```yaml
apiVersion: gloo.solo.io/v1
kind: Upstream
metadata:
  labels:
    app: grpcstore-demo
    discovered_by: kubernetesplugin
  name: default-grpcstore-demo-80
  namespace: gloo-system
spec:
  discoveryMetadata: {}
  kube:
    selector:
      app: grpcstore-demo
    serviceName: grpcstore-demo
    serviceNamespace: default
    servicePort: 80
    serviceSpec:
      grpc:
        descriptors: Q3F3RkNoVm5iMjluYkdVdllYQnBMMmgwZE … bTkwYnpNPQ== # snipped for brevity
        grpcServices:
        - functionNames:
          - CreateItem
          - ListItems
          - DeleteItem
          - GetItem
          packageName: solo.examples.v1
          serviceName: StoreService
status:
  reported_by: gloo
  state: 1

```

{{% notice note %}}
The descriptors field above was truncated for brevity.
{{% /notice %}}

As you can see Gloo Edge's function discovery detected the gRPC functions on that service.

---

## Adding a Virtual Service

Now let's add a Virtual Service to Gloo Edge that will map to the gRPC service listening on port 80. The following yaml describes the Virtual Service:

```yaml
apiVersion: gateway.solo.io/v1
kind: VirtualService
metadata:
  name: grpc
  namespace: gloo-system
spec:
  virtualHost:
    routes:
      - matchers:
          - prefix: /
        routeAction:
          single:
            upstream:
              name: default-grpcstore-demo-80
              namespace: gloo-system
```

The Virtual Service assumes that you are using the namespace `gloo-system` for your Gloo Edge installation. In this initial configuration, we are matching the prefix `/` for all domains. Save the yaml as the file `grpc-vs.yaml` and run the following:

```bash
kubectl apply -f grpc-vs.yaml
```

### Validate with gRPC client

The next step is to test connectivity to the service using the tool `grpcurl`. We are going to get the IP address Gloo Edge is using for a proxy, and then issue a request using `grpcurl`. Since we are not using TLS, we will have to use the flag `-plaintext` to allow for unencrypted communications. Later in this guide, we'll show how to add TLS to the configuration.

`grpcurl` expects a port number as part of the request. The Store service has Server Reflection enabled, which means that we do not have to specify a proto source file for `grpcurl` to use with our request. We are going to use the `list` argument to enumerate the services available.

```bash
grpcurl -plaintext $(glooctl proxy address --port http) list
```

```console
grpc.reflection.v1alpha.ServerReflection
solo.examples.v1.StoreService
```

Excellent! We were able to communicate with our server and get a list of services. Now let's see what methods are available in `solo.examples.v1.StoreService` using the `describe` argument.

```bash
grpcurl -plaintext $(glooctl proxy address --port http) describe solo.examples.v1.StoreService
```

```console
solo.examples.v1.StoreService is a service:
service StoreService {
  rpc CreateItem ( .solo.examples.v1.CreateItemRequest ) returns ( .solo.examples.v1.CreateItemResponse );
  rpc DeleteItem ( .solo.examples.v1.DeleteItemRequest ) returns ( .solo.examples.v1.DeleteItemResponse );
  rpc GetItem ( .solo.examples.v1.GetItemRequest ) returns ( .solo.examples.v1.GetItemResponse );
  rpc ListItems ( .solo.examples.v1.ListItemsRequest ) returns ( .solo.examples.v1.ListItemsResponse );
```

You can continue to describe the individual methods and messages using the same syntax. Let's try using the `CreateItem` method to add an item to the store.

```bash
grpcurl -plaintext -d '{"item":{"name":"item1"}}' $(glooctl proxy address --port http) solo.examples.v1.StoreService/CreateItem
```

```json
{
  "item": {
    "name": "item1"
  }
}
```

We can retrieve all items by using the `ListItems` method:

```bash
grpcurl -plaintext $(glooctl proxy address --port http) solo.examples.v1.StoreService/ListItems
```

```json
{
  "items": [
    {
      "name": "item1"
    }
  ]
}
```

Looks like things are working pretty well. Now let's make them a bit more complicated.

---

## Adding TLS and a specific domain

In this section we are going to add a specific domain to expose the service on, and enable encryption on the communication between the client and Envoy.

### Using a specific domain

You may want to narrow the availability of your service to a specific domain. This is done by editing the Virtual Service and adding a `domain` entry to the `virtualHost` configuration. Run the following command:

```bash
kubectl edit vs grpc -n gloo-system
```

And update the yaml by adding the highlighted lines:

{{< highlight yaml "hl_lines=3-4" >}}
spec:
  virtualHost:
    domains:
    - store.example.com
    routes:
    - matchers:
      - prefix: /
      routeAction:
        single:
          upstream:
            name: default-grpcstore-demo-80
            namespace: gloo-system
{{< /highlight >}}

Now if we try to list the items again:

```bash
grpcurl -plaintext $(glooctl proxy address --port http) solo.examples.v1.StoreService/ListItems
```

We get an error:

```console
Error invoking method "solo.examples.v1.StoreService/ListItems": failed to query for service descriptor "solo.examples.v1.StoreService": server does not support the reflection API
```

The error message is not strictly true, but it's the best that Envoy can figure out. gRPC is using HTTP/2 and we did not specify a `authority` header, which is the equivalent of a HOST header in curl. Envoy instead used whatever value was in `$IP` as the HOST name.  Let's update our command to use the `-authority` flag.

```bash
grpcurl -plaintext -authority store.example.com $(glooctl proxy address --port http) solo.examples.v1.StoreService/ListItems
```

We once again get the expect response.

```json
{
  "items": [
    {
      "name": "item1"
    }
  ]
}
```

{{< notice note >}}
In our example we are using a public IP address in the form X.X.X.X:80 and specifying the authority header. If we were using a domain in our request instead, e.g. <code>store.example.com:80</code>, we would still need to specify the authority header. Otherwise Envoy will interpret the domain name including the <code>:80</code> on end as the HOST header. Since <code>store.example.com:80</code> does not match <code>store.example.com</code>, you will receive an error. By specifying the authority header explicitly, you will avoid this issue. If you cannot specify the authority header, you can update the domain match on the Virtual Service to use <code>store.example.com*</code>, which will match anything that begins with that domain.
{{< /notice >}}

### Adding TLS

Now that we have things associated with a specific domain, let's add a certificate. In our example, we are going to create a self-signed certificate, but in a production scenario you should use a certificate from public or private CA.

First, let's generate the certificate using openssl.

```bash
openssl req -x509 -nodes -days 365 -newkey rsa:2048 \
   -keyout tls.key -out tls.crt -subj "/CN=store.example.com"
```

Now we will create the Kubernetes secret to hold this cert:

```bash
kubectl create secret tls grpc-tls --key tls.key \
   --cert tls.crt --namespace gloo-system
```

Lastly, let's configure the Virtual Service to use this cert via the Kubernetes secrets:

```bash
glooctl edit virtualservice --name grpc --namespace gloo-system \
   --ssl-secret-name grpc-tls --ssl-secret-namespace gloo-system
```

Now if we get the `grpc` Virtual Service, we should see the new SSL configuration:

```bash
glooctl get virtualservice grpc -o kube-yaml
```

{{< highlight yaml "hl_lines=7-10" >}}
apiVersion: gateway.solo.io/v1
kind: VirtualService
metadata:
  name: grpc
  namespace: gloo-system
spec:
  sslConfig:
    secretRef:
      name: grpc-tls
      namespace: gloo-system
  virtualHost:
    domains:
    - store.example.com
    routes:
    - matchers:
      - prefix: /
      routeAction:
        single:
          upstream:
            name: default-grpcstore-demo-80
            namespace: gloo-system
status:
  reported_by: gateway
  state: 1
  subresource_statuses:
    '*v1.Proxy.gloo-system.gateway-proxy':
      reported_by: gloo
      state: 1
{{< /highlight >}}

We'll need to update the `grpcurl` command to use the `-insecure` flag instead of the `-plaintext` flag. We also need to update the address to use port 443 instead of 80.

Alright, let's try to connect to our service on port 443 (note the `--port https` flag) and invoke the `ListItem` method.

```bash
grpcurl -insecure -authority store.example.com $(glooctl proxy address --port https) solo.examples.v1.StoreService/ListItems
```

```console
{
  "items": [
    {
      "name": "item1"
    }
  ]
}
```

Nice! If you happen to be using a certificate that has the correct domain listed and is trusted by the client, you can skip the `-insecure` flag.

---

## Summary

In this guide we saw how to present a gRPC Upstream through Gloo Edge and connect to it using a gRPC client. We also saw how to add a domain filter and enable TLS. For more information on gRPC, check out the guide for presenting a [gRPC service as a REST API]({{% versioned_link_path fromRoot="/installation/gateway/kubernetes/" %}}) through Gloo Edge. You can find out more about using TLS with Gloo Edge in the [Network Encryption]({{% versioned_link_path fromRoot="/guides/security/tls/" %}}) section of our guides. 
