---
title: Setting up Server TLS
weight: 50
description: Understanding how to set up Server-side TLS for Gloo
---

## Configuring TLS for Gloo

Gloo can encrypt traffic coming from external clients over TLS/HTTPS. [We can also configure Gloo to do mTLS with external clients as well](../client_tls). In this document, we'll explore configuring Gloo for server TLS.

## Server TLS

Gloo supports server-side TLS where the server presents a certificate to the client based on the domain for what the client asked. This means we can support multiple virtual hosts on a single port and use SNI to determine what certificate to serve depending what domain the client is requesting. In Gloo, we associate our TLS configuration with a specific VirtualService which can then describe which SNI hosts would be candidates for both the TLS certificates as well as the routing rules that are defined in the VirtualService. Let's look at setting up TLS.

### Prepare sample environment

Before we walk through setting up TLS for our virtual hosts, let's deploy our sample applications with a default VirtualService and routes.

To start, let's make sure the `petstore` application is deployed:

```bash
kubectl apply -f https://raw.githubusercontent.com/solo-io/gloo/v1.2.9/example/petstore/petstore.yaml
```

If we query the gloo upstreams we should see it:

```bash
glooctl get upstream default-petstore-8080
```

```noop
+-----------------------|------------|----------|-------------------------+
|       UPSTREAM        |    TYPE    |  STATUS  |         DETAILS         |
+-----------------------|------------|----------|-------------------------+
| default-petstore-8080 | Kubernetes | Accepted | svc name:      petstore |
|                       |            |          | svc namespace: default  |
|                       |            |          | port:          8080     |
|                       |            |          | REST service:           |
|                       |            |          | functions:              |
|                       |            |          | - addPet                |
|                       |            |          | - deletePet             |
|                       |            |          | - findPetById           |
|                       |            |          | - findPets              |
|                       |            |          |                         |
+-----------------------|------------|----------|-------------------------+
```

Now let's create a route to the petstore like [we did in the hello world tutorial](../../../gloo_routing/hello_world/ )

```bash
glooctl add route \
    --path-exact /sample-route-1 \
    --dest-name default-petstore-8080 \
    --prefix-rewrite /api/pets
```

Since we didn't explicitly create a VirtualService, adding this route will create a default VirtualService named `default`.

```bash
glooctl get virtualservice default -o yaml
```

```yaml
---
metadata:
  name: default
  namespace: gloo-system
  resourceVersion: "21723"
status:
  reportedBy: gateway
  state: Accepted
  subresourceStatuses:
    '*v1.Proxy gloo-system gateway-proxy':
      reportedBy: gloo
      state: Accepted
virtualHost:
  domains:
  - '*'
  routes:
  - matchers:
     - exact: /sample-route-1
    routeAction:
      single:
        upstream:
          name: default-petstore-8080
          namespace: gloo-system
    options:
      prefixRewrite: /api/pets
```

If we want to query the service to verify routing is working, we can like this:

```bash
curl $(glooctl proxy url --port http)/sample-route-1
```

```json
[{"id":1,"name":"Dog","status":"available"},{"id":2,"name":"Cat","status":"pending"}]
```

Let's enable HTTPS by configuring TLS/SSL for our VirtualService.

### Configuring TLS/SSL in a VirtualService

Before we add the TLS/SSL configuration, let's create a private key and certificate to use in our VirtualService. Obviously if you have your own key/cert pair, you can use those instead of creating self-signed certs here.

```bash
openssl req -x509 -nodes -days 365 -newkey rsa:2048 \
   -keyout tls.key -out tls.crt -subj "/CN=petstore.example.com"
```

Now we should create the Kubernetes secrets to hold this cert:

```bash
kubectl create secret tls gateway-tls --key tls.key \
   --cert tls.crt --namespace gloo-system
```

Note, you could also use `glooctl` to create the tls `secret` which also allows storing a RootCA which can be used for client cert verification (for example, if you set up mTLS for your VirtualServices). `glooctl` adds extra annotations so we can catalog the different secrets we may need like `tls`, `aws`, `azure` to make it easier to serialize/deserialize in the correct format. For example, to create the tls secret with `glooctl`:

```bash
glooctl create secret tls --certchain $CERT --privatekey $KEY
```

If you've created your secret with `kubectl`, you don't need to use `glooctl` to do the same. 

Lastly, let's configure the VirtualService to use this cert via the Kubernetes secrets:

```bash
glooctl edit virtualservice --name default --namespace gloo-system \
   --ssl-secret-name gateway-tls --ssl-secret-namespace gloo-system
```

Now if we get the `default` VirtualService, we should see the new SSL configuration:

```bash
glooctl get virtualservice default -o yaml
```

{{< highlight yaml "hl_lines=6-9" >}}
---
metadata:
  name: default
  namespace: gloo-system
  resourceVersion: "22639"
sslConfig:
  secretRef:
    name: gateway-tls
    namespace: gloo-system
status:
  reportedBy: gateway
  state: Accepted
  subresourceStatuses:
    '*v1.Proxy gloo-system gateway-proxy':
      reportedBy: gloo
      state: Accepted
virtualHost:
  domains:
  - '*'
  routes:
  - matchers:
     - exact: /sample-route-1
    routeAction:
      single:
        upstream:
          name: default-petstore-8080
          namespace: gloo-system
    options:
      prefixRewrite: /api/pets
{{< /highlight >}}

If we try query the HTTP port, we should not get a successful response (it should hang, or timeout since we no longer have a route on the HTTP listener and Envoy will give a grace period to drain requests. After the drain is completed, the HTTP port will be closed if there are no other routes on the listener). By default when there are no routes for a listener, the port will not be opened.

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

## Serving certificates for multiple virtual hosts with SNI

Let's say we had another VirtualService that serves a different certificate for a different virtual host. Gloo allows you to serve multiple virtual hosts from a single HTTPS port and use [SNI](https://en.wikipedia.org/wiki/Server_Name_Indication) to determine which certificate to present to which virtual host. In the previous example, we create a certificate for the `petstore.example.com` domain. Let's create a new self-signed certificate for a different domain, `animalstore.example.com` and see how Gloo can serve multiple virtual hosts on a single port/listener.

```bash
openssl req -x509 -nodes -days 365 -newkey rsa:2048 \
   -keyout tls.key -out tls.crt -subj "/CN=animalstore.example.com"
```

And create Kubernetes secrets:

```bash
kubectl create secret tls animal-certs --key tls.key \
    --cert tls.crt --namespace gloo-system
```

We'll also create a new VirtualService and attach this new certificate to it. When we create the VirtualService, let's also specify exactly which domains we'll match and to which we'll serve the `animalstore.example.com` certificate:

```bash
glooctl  create virtualservice --name animal --domains animalstore.example.com
```
Now add the TLS/SSL config *with the appropriate SNI domain information*

```bash
glooctl edit virtualservice --name animal --namespace gloo-system \
   --ssl-secret-name animal-certs --ssl-secret-namespace gloo-system \
   --ssl-sni-domains animalstore.example.com
```

{{% notice note %}}
As you can see in the previous step, we need to specify the SNI domains that will match for this certificate with the `--ssl-sni-domains` parameter. If you do NOT specify this parameter, Envoy will become confused about which certificates to serve because there will effectively be two (or more) with no qualifying information. If that's the case, you can expect to see logs similar to the following in your `gateway-proxy` logs:

{{< highlight bash "nowrap=false" >}}
gateway-proxy-9b55c99c7-x7r7c gateway-proxy [2019-03-20 19:01:01.763][6][warning][config] 
[bazel-out/k8-opt/bin/external/envoy/source/common/config/_virtual_includes/grpc_mux_subscription_lib/common/config/grpc_mux_subscription_impl.h:70] 
gRPC config for type.googleapis.com/envoy.api.v2.Listener 
rejected: Error adding/updating listener listener-::-8443: error adding listener 
'[::]:8443': multiple filter chains with the same matching rules are defined
{{< /highlight >}}  

If you end up with logs like that, double check your SNI settings.

{{% /notice %}}


Lastly, let's add a route for this VirtualService:

```bash
glooctl add route --name animal\
   --path-exact /animals \
   --dest-name default-petstore-8080 \
   --prefix-rewrite /api/pets
```

Note, we're giving this service a different API, namely `/animals` instead of `/sample-route-1`.

Now if we get the VirtualService, we should see this one set up with a different cert/secret:

```bash
glooctl get virtualservice animal -o yaml
```

{{< highlight yaml "hl_lines=2-5" >}}
---
displayName: animal
sslConfig:
  secretRef:
    name: animal-certs
    namespace: gloo-system
status:
  reportedBy: gateway
  state: Accepted
virtualHost:
  domains:
  - '*'
  routes:
  - matchers:
     - exact: /animals
    routeAction:
      single:
        upstream:
          name: default-petstore-8080
          namespace: gloo-system
    options:
      prefixRewrite: /api/pets
{{< /highlight >}}     

If everything up to this point looks good, let's try to query the service and make sure to pass in the qualifying `Host` information so that Envoy can serve the correct certificates.


```bash
curl -k -H "Host: animalstore.example.com"  \
$(glooctl proxy url --port https)/animals
```

```json
[{"id":1,"name":"Dog","status":"available"},{"id":2,"name":"Cat","status":"pending"}]
```


### Understanding how it all works

By default, when a VirtualService does NOT have any SSL/TLS configuration, it will be attached to the HTTP listener that we have for Gloo proxy (listening on port `8080` by default, but exposed in Kubernetes on port `80` in the `gateway-proxy` service). When we add the SSL/TLS configuration, that VirtualService will automatically become bound to the HTTPS port (listening on port `8443` on the gateway-proxy, but mapped to port `443` on the Kubernetes service). 

To verify that, let's take a look at the Gloo `Proxy` object. The Gloo `Proxy` object is the lowest-level domain object that reflects the configuration Gloo sends to Envoy. All of the other higher-level objects (like `Gateway` and `VirtualService`) drive the configuration of the `Proxy` object. 

```bash
kubectl get proxy -n gloo-system -o yaml
```

{{< highlight yaml "hl_lines=14-15 33-36" >}}
apiVersion: v1
items:
- apiVersion: gloo.solo.io/v1
  kind: Proxy
  metadata:
    name: gateway-proxy
    namespace: gloo-system
  spec:
    listeners:
    - bindAddress: '::'
      bindPort: 8080
      httpListener: {}
      name: listener-::-8080
    - bindAddress: '::'
      bindPort: 8443
      httpListener:
        virtualHosts:
        - domains:
          - '*'
          name: gloo-system.default
          routes:
          - matchers:
             - exact: /sample-route-1
            routeAction:
              single:
                upstream:
                  name: default-petstore-8080
                  namespace: gloo-system
            options:
              prefixRewrite: /api/pets
      name: listener-::-8443
      sslConfiguations:
      - secretRef:
          name: gateway-tls
          namespace: gloo-system
{{< /highlight >}}
