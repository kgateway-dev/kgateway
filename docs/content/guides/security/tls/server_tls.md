---
title: Setting up Server TLS
weight: 10
description: Set up Server-side TLS for Gloo Edge
---

Gloo Edge can encrypt traffic coming from external clients over TLS/HTTPS. [We can also configure Gloo Edge to do mTLS with external clients as well]({{% versioned_link_path fromRoot="/guides/security/tls/client_tls//" %}}). In this document, we'll explore configuring Gloo Edge for server TLS.

---

## Server TLS

Gloo Edge supports server-side TLS where the server presents a certificate to the client based on the domain specified in the client request. This means we can support multiple virtual hosts on a single port and use Server Name Identification (SNI) to determine what certificate to serve depending what domain the client is requesting. In Gloo Edge, we associate our TLS configuration with a specific Virtual Service which can then describe which SNI hosts would be candidates for both the TLS certificates as well as the routing rules that are defined in the Virtual Service. Let's look at setting up TLS.

### Prepare sample environment

Before we walk through setting up TLS for our virtual hosts, let's deploy our sample applications with a default Virtual Service and routes.

To start, let's make sure the `petstore` application is deployed:

```bash
kubectl apply -f https://raw.githubusercontent.com/solo-io/gloo/v1.14.x/example/petstore/petstore.yaml
```

If we query the Gloo Edge Upstreams we should see it:

```bash
glooctl get upstream default-petstore-8080
```

```noop
+-----------------------+------------+----------+-------------------------+
|       UPSTREAM        |    TYPE    |  STATUS  |         DETAILS         |
+-----------------------+------------+----------+-------------------------+
| default-petstore-8080 | Kubernetes | Accepted | svc name:      petstore |
|                       |            |          | svc namespace: default  |
|                       |            |          | port:          8080     |
|                       |            |          |                         |
+-----------------------+------------+----------+-------------------------+
```

Now let's create a route to the petstore like [we did in the hello world tutorial]({{% versioned_link_path fromRoot="/guides/traffic_management/hello_world/" %}}):
```bash
glooctl add route \
    --path-exact /sample-route-1 \
    --dest-name default-petstore-8080 \
    --prefix-rewrite /api/pets
```

Since we didn't explicitly create a Virtual Service, adding this route will create a default Virtual Service named `default`.

```bash
glooctl get virtualservice default -o kube-yaml
```

```yaml
apiVersion: gateway.solo.io/v1
kind: VirtualService
metadata:
  name: default
  namespace: gloo-system
spec:
  virtualHost:
    domains:
    - '*'
    routes:
    - matchers:
      - exact: /sample-route-1
      options:
        prefixRewrite: /api/pets
      routeAction:
        single:
          upstream:
            name: default-petstore-8080
            namespace: gloo-system
status:
  reportedBy: gateway
  state: 1
  subresourceStatuses:
    '*v1.Proxy.gloo-system.gateway-proxy':
      reportedBy: gloo
      state: 1
```

If we want to query the service to verify routing is working, we can do so like this:

```bash
curl $(glooctl proxy url --port http)/sample-route-1
```

```json
[{"id":1,"name":"Dog","status":"available"},{"id":2,"name":"Cat","status":"pending"}]
```

Let's enable HTTPS by configuring TLS/SSL for our Virtual Service.

### Configuring TLS/SSL in a Virtual Service

Before we add the TLS/SSL configuration, let's create a private key and certificate to use in our Virtual Service. Obviously, if you have your own key/cert pair, you can use those instead of creating self-signed certs here.

```bash
openssl req -x509 -nodes -days 365 -newkey rsa:2048 \
   -keyout tls.key -out tls.crt -subj "/CN=petstore.example.com"
```

Now we should create the Kubernetes secrets to hold this cert:

```bash
kubectl create secret tls upstream-tls --key tls.key \
   --cert tls.crt --namespace gloo-system
```

You could also use `glooctl` to create the TLS `secret` which also allows storing a root certificate authority (CA) which can be used for client cert verification (for example, if you set up [downstream mTLS for your Virtual Services](#configuring-downstream-mtls-in-a-virtual-service)). `glooctl` adds extra annotations so we can catalog the different secrets we may need like `tls`, `aws`, `azure` to make it easier to serialize/deserialize in the correct format. For example, to create the TLS secret with `glooctl`:

```bash
glooctl create secret tls --name upstream-tls --certchain tls.crt --privatekey tls.key
```

If you've created your secret with `kubectl`, you don't need to use `glooctl` to do the same. 

Lastly, let's configure the Virtual Service to use this cert via the Kubernetes secrets:

```bash
glooctl edit virtualservice --name default --namespace gloo-system \
   --ssl-secret-name upstream-tls --ssl-secret-namespace gloo-system
```

Now if we get the `default` Virtual Service, we should see the new SSL configuration:

```bash
glooctl get virtualservice default -o kube-yaml
```

{{< highlight yaml "hl_lines=7-10" >}}
apiVersion: gateway.solo.io/v1
kind: VirtualService
metadata:
  name: default
  namespace: gloo-system
spec:
  sslConfig:
    secretRef:
      name: upstream-tls
      namespace: gloo-system
  virtualHost:
    domains:
    - '*'
    routes:
    - matchers:
      - exact: /sample-route-1
      options:
        prefixRewrite: /api/pets
      routeAction:
        single:
          upstream:
            name: default-petstore-8080
            namespace: gloo-system
status:
  reportedBy: gateway
  state: 1
  subresourceStatuses:
    '*v1.Proxy.gloo-system.gateway-proxy':
      reportedBy: gloo
      state: 1
{{< /highlight >}}

If we try to query the HTTP port, we should not get a successful response (it should hang, or timeout since we no longer have a route on the HTTP listener and Envoy will give a grace period to drain requests. After the drain is completed, the HTTP port will be closed if there are no other routes on the listener). By default when there are no routes for a listener, the port will not be opened.

```bash
curl $(glooctl proxy url --port http)/sample-route-1
```

If we try with the HTTPS port, it should work:

```bash
curl $(glooctl proxy url --port https)/sample-route-1
```
It's possible that if you used self-signed certs, `curl` cannot validate the certificate. In this case, SPECIFICALLY FOR THIS EXAMPLE, you can skip certificate validation with `curl -k ...`(note this is not secure):

```bash
curl -k $(glooctl proxy url --port https)/sample-route-1
```

```json
[{"id":1,"name":"Dog","status":"available"},{"id":2,"name":"Cat","status":"pending"}]
```

---

## Configuring downstream mTLS in a Virtual Service

Gloo Edge can be configured to verify downstream client certificates. As seen in the example above, you can reference a Kubernetes secret on your Virtual Service which allows Gloo Edge to verify the Upstream. If this secret also contains a root CA, Gloo Edge will use it to verify downstream client certificates.

We need to create a new set of self-signed certs to use in between the client and Gloo Edge.

```bash
openssl req -x509 -nodes -days 365 -newkey rsa:2048 \
   -keyout mtls.key -out mtls.crt -subj "/CN=gloo.gloo-system.com"
```

Since they are self-signed, we can use mtls.crt as both our client cert and our root CA file for Gloo Edge to verify the client.

We will use `glooctl` to create the TLS `secret`, adding the `rootca` with an additional flag:

```bash
glooctl create secret tls --name downstream-mtls --certchain tls.crt --privatekey tls.key --rootca mtls.crt
```
The cert and key files were generated from the previous example (tls.crt and tls.key). The root CA file comes from the self-signed cert provided in this example (mtls.crt).

Next, let's configure the Virtual Service to use this cert via the Kubernetes secrets:

```bash
glooctl edit virtualservice --name default --namespace gloo-system \
   --ssl-secret-name downstream-mtls --ssl-secret-namespace gloo-system
```

Now if we get the `default` Virtual Service, we should see the new SSL configuration:

```bash
glooctl get virtualservice default -o kube-yaml
```

{{< highlight yaml "hl_lines=7-10" >}}
apiVersion: gateway.solo.io/v1
kind: VirtualService
metadata:
  name: default
  namespace: gloo-system
spec:
  sslConfig:
    secretRef:
      name: downstream-mtls
      namespace: gloo-system
  virtualHost:
    domains:
    - '*'
    routes:
    - matchers:
      - exact: /sample-route-1
      options:
        prefixRewrite: /api/pets
      routeAction:
        single:
          upstream:
            name: default-petstore-8080
            namespace: gloo-system
status:
  reportedBy: gateway
  state: 1
  subresourceStatuses:
    '*v1.Proxy.gloo-system.gateway-proxy':
      reportedBy: gloo
      state: 1
{{< /highlight >}}

If we try query the HTTP port, we should not get a successful response (it should hang, or timeout since we no longer have a route on the HTTP listener and Envoy will give a grace period to drain requests. After the drain is completed, the HTTP port will be closed if there are no other routes on the listener). By default when there are no routes for a listener, the port will not be opened.

```bash
curl $(glooctl proxy url --port http)/sample-route-1
```

If we try with the HTTPS port, it should be denied due to not being verified:

Since we used self-signed certs, `curl` cannot validate the certificate. In this case, SPECIFICALLY FOR THIS EXAMPLE, you can skip certificate validation with `curl -k ...`(note this is not secure):

```bash
curl -k $(glooctl proxy url --port https)/sample-route-1
```

This will fail with Gloo Edge refusing the client connection because the client has not provided any certs.

```
curl: (35) error:1401E410:SSL routines:CONNECT_CR_FINISHED:sslv3 alert handshake failure
```

We can provide certs by passing in the mtls.key and mtls.crt files.

```bash
curl --cert mtls.crt --key mtls.key -k $(glooctl proxy url --port https)/sample-route-1
```

```json
[{"id":1,"name":"Dog","status":"available"},{"id":2,"name":"Cat","status":"pending"}]
```

---

## Serving certificates for multiple virtual hosts with SNI

Let's say we had another Virtual Service that serves a different certificate for a different virtual host. Gloo Edge allows you to serve multiple virtual hosts from a single HTTPS port and use [SNI](https://en.wikipedia.org/wiki/Server_Name_Indication) to determine which certificate to present to which virtual host. In the previous example, we create a certificate for the `petstore.example.com` domain. Let's create a new self-signed certificate for a different domain, `animalstore.example.com` and see how Gloo Edge can serve multiple virtual hosts on a single port/listener.

First we'll create the self-signed certificate for the domain `animalstore.example.com`.

```bash
openssl req -x509 -nodes -days 365 -newkey rsa:2048 \
   -keyout tls.key -out tls.crt -subj "/CN=animalstore.example.com"
```

Then we will create a Kubernetes secret to store the certificate:

```bash
kubectl create secret tls animal-certs --key tls.key \
    --cert tls.crt --namespace gloo-system
```

We'll also create a new Virtual Service and attach this new certificate to it. When we create the Virtual Service, let's also specify exactly which domains we'll match and to which we'll serve the `animalstore.example.com` certificate:

```bash
glooctl  create virtualservice --name animal --domains animalstore.example.com
```

Now add the TLS/SSL config *with the appropriate SNI domain information*:

```bash
glooctl edit virtualservice --name animal --namespace gloo-system \
   --ssl-secret-name animal-certs --ssl-secret-namespace gloo-system \
   --ssl-sni-domains animalstore.example.com
```

{{% notice warning %}}
As you can see in the previous step, we need to specify the SNI domains that will match for this certificate with the `--ssl-sni-domains` parameter. If you do NOT specify this parameter, Envoy will become confused about which certificates to serve because there will effectively be two (or more) with no qualifying information. If that's the case, you can expect to see logs similar to the following in your `gateway-proxy` logs:

```shell
gateway-proxy-9b55c99c7-x7r7c gateway-proxy [2019-03-20 19:01:01.763][6][warning][config] 
[bazel-out/k8-opt/bin/external/envoy/source/common/config/_virtual_includes/grpc_mux_subscription_lib/common/config/grpc_mux_subscription_impl.h:70] 
gRPC config for type.googleapis.com/envoy.api.v2.Listener 
rejected: Error adding/updating listener listener-::-8443: error adding listener 
'[::]:8443': multiple filter chains with the same matching rules are defined
```

If you end up with logs like that, double check your SNI settings.

{{% /notice %}}


Lastly, let's add a route for this Virtual Service:

```bash
glooctl add route --name animal\
   --path-exact /animals \
   --dest-name default-petstore-8080 \
   --prefix-rewrite /api/pets
```

Note, we're giving this service a different API, namely `/animals` instead of `/sample-route-1`.

Now if we get the Virtual Service, we should see this one set up with a different cert/secret:

```bash
glooctl get virtualservice animal -o kube-yaml
```

{{< highlight yaml "hl_lines=8-13" >}}
apiVersion: gateway.solo.io/v1
kind: VirtualService
metadata:
  name: animal
  namespace: gloo-system
spec:
  displayName: animal
  sslConfig:
    secretRef:
      name: animal-certs
      namespace: gloo-system
    sniDomains:
    - animalstore.example.com
  virtualHost:
    domains:
    - animalstore.example.com
    routes:
    - matchers:
      - exact: /animals
      options:
        prefixRewrite: /api/pets
      routeAction:
        single:
          upstream:
            name: default-petstore-8080
            namespace: gloo-system
status:
  reportedBy: gateway
  state: 1
  subresourceStatuses:
    '*v1.Proxy.gloo-system.gateway-proxy':
      reportedBy: gloo
      state: 1
{{< /highlight >}}     

If everything up to this point looks good, let's try to query the service and make sure to pass in the qualifying SNI information so that Envoy can serve the correct certificates.

Since we used self-signed certs, `curl` cannot validate the certificate. In this case, SPECIFICALLY FOR THIS EXAMPLE, you can skip certificate validation with `curl -k ...`(note this is not secure):

```shell script
curl -k --resolve animalstore.example.com:443:$(kubectl get svc -n gloo-system gateway-proxy -o=jsonpath='{.status.loadBalancer.ingress[0].ip}') https://animalstore.example.com/animals
```

```json
[{"id":1,"name":"Dog","status":"available"},{"id":2,"name":"Cat","status":"pending"}]
```


### Understanding how it all works

By default, when a Virtual Service does NOT have any SSL/TLS configuration, it will be attached to the HTTP listener that we have for Gloo Edge proxy (listening on port `8080` by default, but exposed in Kubernetes on port `80` in the `gateway-proxy` service). When we add the SSL/TLS configuration, that Virtual Service will automatically become bound to the HTTPS port (listening on port `8443` on the gateway-proxy, but mapped to port `443` on the Kubernetes service). 

To verify that, let's take a look at the Gloo Edge `Proxy` object. The Gloo Edge `Proxy` object is the lowest-level domain object that reflects the configuration Gloo Edge sends to Envoy. All of the other higher-level objects (like `Gateway` and `VirtualService`) drive the configuration of the `Proxy` object. 

```bash
kubectl get proxy -n gloo-system gateway-proxy -oyaml
```

{{% notice warning %}}
Note that the proxy's TLS listener (the one with `bindPort` 8443) has multiple sslConfigurations. If any of those valid TLS configs match a request, they can be routed to any route on the listener. This means that SSL config can be shared between virtual services if they are part of the same listener (i.e., HTTP or HTTPS).
{{% /notice %}}

{{< highlight yaml "hl_lines=18-19 74-82" >}}
apiVersion: gloo.solo.io/v1
kind: Proxy
metadata:
  name: gateway-proxy
  namespace: gloo-system
spec:
  listeners:
  - bindAddress: '::'
    bindPort: 8080
    httpListener: {}
    metadata:
      sources:
      - kind: '*v1.Gateway'
        name: gateway-proxy
        namespace: gloo-system
    name: listener-::-8080
    useProxyProto: false
  - bindAddress: '::'
    bindPort: 8443
    httpListener:
      virtualHosts:
      - domains:
        - animalstore.example.com
        metadata:
          sources:
          - kind: '*v1.VirtualService'
            name: animal
            namespace: gloo-system
        name: gloo-system.animal
        routes:
        - matchers:
          - exact: /animals
          metadata:
            sources:
            - kind: '*v1.VirtualService'
              name: animal
              namespace: gloo-system
          options:
            prefixRewrite: /api/pets
          routeAction:
            single:
              upstream:
                name: default-petstore-8080
                namespace: gloo-system
      - domains:
        - '*'
        metadata:
          sources:
          - kind: '*v1.VirtualService'
            name: default
            namespace: gloo-system
        name: gloo-system.default
        routes:
        - matchers:
          - exact: /sample-route-1
          metadata:
            sources:
            - kind: '*v1.VirtualService'
              name: default
              namespace: gloo-system
          options:
            prefixRewrite: /api/pets
          routeAction:
            single:
              upstream:
                name: default-petstore-8080
                namespace: gloo-system
    metadata:
      sources:
      - kind: '*v1.Gateway'
        name: gateway-proxy-ssl
        namespace: gloo-system
    name: listener-::-8443
    sslConfigurations:
    - secretRef:
        name: animal-certs
        namespace: gloo-system
      sniDomains:
      - animalstore.example.com
    - secretRef:
        name: upstream-tls
        namespace: gloo-system
    useProxyProto: false
status:
  reportedBy: gloo
  state: 1
{{< /highlight >}}

---

## Next Steps

As we mentioned earlier, you can configure Gloo Edge to perform mutual TLS (mTLS) and client side TLS with Upstreams. Check out these guides to learn more:

* **[Setting up Upstream TLS]({{% versioned_link_path fromRoot="/guides/security/tls/client_tls//" %}})**
* **[Setting up Upstream TLS with Service Annotations]({{% versioned_link_path fromRoot="/guides/security/tls/client_tls_service_annotations//" %}})**
* **[Gloo Edge mTLS mode]({{% versioned_link_path fromRoot="/guides/security/tls/mtls/" %}})**