---
title: Hybrid Gateway
weight: 10
description: Define multiple HTTP or TCP Gateways within a single Gateway
---

Hybrid Gateways allow users to define multiple HTTP or TCP Gateways for a single Gateway with distinct matching criteria. 

---

Hybrid gateways expand the functionality of HTTP and TCP gateways by exposing multiple gateways on the same port and letting you use request properties to choose which gateway the request routes to.
Selection is done based on `Matcher` fields, which map to a subset of Envoy [`FilterChainMatch`](https://www.envoyproxy.io/docs/envoy/latest/api-v3/config/listener/v3/listener_components.proto#config-listener-v3-filterchainmatch) fields.

## Only accept requests from a particular CIDR range

Hybrid Gateways allow us to treat traffic from particular IPs differently.
One case where this might come in handy is if a set of clients are at different stages of migrating to TLS >=1.2 support, and therefore we want to enforce different TLS requirements depending on the client.
If the clients originate from the same domain, it may be necessary to dynamically route traffic to the appropriate Gateway based on source IP.

In this example, we will allow requests only from a particular CIDR range to reach an upstream, while short-circuiting requests from all other IPs by using a direct response action.

**Before you begin**: Complete the [Hello World guide]({{< versioned_link_path fromRoot="/guides/traffic_management/hello_world" >}}) demo setup.

To start we will add a second VirtualService that also matches all requests and has a directResponseAction:

```yaml
kubectl apply -n gloo-system -f - <<EOF
apiVersion: gateway.solo.io/v1
kind: VirtualService
metadata:
  name: 'client-ip-reject'
  namespace: 'gloo-system'
spec:
  virtualHost:
    domains:
      - '*'
    routes:
      - matchers:
          - prefix: /
        directResponseAction:
          status: 403
          body: "client ip forbidden\n"
EOF
```


Next let's update the existing `gateway-proxy` Gateway CR, replacing the default `httpGateway` with a [`hybridGateway`]({{< versioned_link_path fromRoot="/reference/api/github.com/solo-io/gloo/projects/gateway/api/v1/gateway.proto.sk/#hybridgateway" >}}) as follows:
```bash
kubectl edit -n gloo-system gateway gateway-proxy
```

{{< highlight yaml "hl_lines=7-21" >}}
apiVersion: gateway.solo.io/v1
kind: Gateway
metadata: # collapsed for brevity
spec:
  bindAddress: '::'
  bindPort: 8080
  hybridGateway:
    matchedGateways:
      - httpGateway:
          virtualServices:
            - name: default
              namespace: gloo-system
        matcher:
          sourcePrefixRanges:
            - addressPrefix: 0.0.0.0
              prefixLen: 1
      - httpGateway:
          virtualServices:
            - name: client-ip-reject
              namespace: gloo-system
        matcher: {}
  proxyNames:
  - gateway-proxy
  useProxyProto: false
status: # collapsed for brevity
{{< /highlight >}}

{{% notice note %}}
The range of 0.0.0.0/1 provides a high chance of matching the client's IP without knowing the specific IP. If you know more about the client's IP, you can specify a different, narrower range.
{{% /notice %}}

Make a request to the proxy, which returns a `200` response because the client IP address matches to the 0.0.0.0/1 range:

```bash
$ curl "$(glooctl proxy url)/all-pets"
[{"id":1,"name":"Dog","status":"available"},{"id":2,"name":"Cat","status":"pending"}]
```

Note that a request to an endpoint that is not matched by the `default` VirtualService returns a `404` response, and the request _does not_ hit the `client-ip-reject` VirtualService:
```bash
$ curl -i "$(glooctl proxy url)/foo"
HTTP/1.1 404 Not Found
date: Tue, 07 Dec 2021 17:48:49 GMT
server: envoy
content-length: 0
```
This is because the `Matcher`s in the `HybridGateway` determine which `MatchedGateway` a request will be routed to, regardless of what routes that gateway has.

### Route requests from non-matching IPs to a catchall gateway 
Next, update the matcher to use a specific IP range that our client's IP is not a member of. Requests from this client IP will now skip this matcher, and will instead match to a catchall gateway that is configured to respond with `403`.

```bash
kubectl edit -n gloo-system gateway gateway-proxy
```
{{< highlight yaml "hl_lines=15-16" >}}
apiVersion: gateway.solo.io/v1
kind: Gateway
metadata: # collapsed for brevity
spec:
  bindAddress: '::'
  bindPort: 8080
  hybridGateway:
    matchedGateways:
      - httpGateway:
          virtualServices:
            - name: default
              namespace: gloo-system
        matcher:
          sourcePrefixRanges:
            - addressPrefix: 1.2.3.4
              prefixLen: 32
      - httpGateway:
          virtualServices:
            - name: client-ip-reject
              namespace: gloo-system
        matcher: {}
  proxyNames:
  - gateway-proxy
  useProxyProto: false
status: # collapsed for brevity
{{< /highlight >}}

The Proxy will update accordingly.

Make a request to the proxy, which now returns a `403` response for any endpoint:

```bash
$ curl "$(glooctl proxy url)/all-pets"
client ip forbidden
```

```bash
$ curl "$(glooctl proxy url)/foo"
client ip forbidden
```

This is expected since the IP of our client is not `1.2.3.4`.

## Hybrid Gateway Delegation

{{% notice note %}}
This feature is available in Gloo Edge version 1.10.x and later.
{{% /notice %}}

{{% notice warn %}}
Hybrid Gateway delegation is supported only for HTTP Gateways.
{{% /notice %}}


With Hybrid Gateways, you can define multiple HTTP and TCP Gateways, each with distinct matching criteria, on a single Gateway CR.

However, condensing all listener and routing configuration onto a single object can be cumbersome when dealing with a large number of matching and routing criteria.

Similar to how Gloo Edge provides delegation between Virtual Services and Route Tables, Hybrid Gateways can be assembled from separate resources. The root Gateway resource selects HttpGateways and assembles the Hybrid Gateway, as though it were defined in a single resource.


### Only accept requests from a particular CIDR range

We will use Hybrid Gateway delegation to achieve the same functionality that we demonstrated earlier in this guide.

1. Confirm that a Virtual Service exists which matches all requests and has a Direct Response Action.
   ```bash
   kubectl get -n gloo-system vs client-ip-reject
   ```

2. Create a MatchableHttpGateway to define the HTTP Gateway.
   ```yaml
   kubectl apply -n gloo-system -f - <<EOF
   apiVersion: gateway.solo.io/v1
   kind: MatchableHttpGateway
   metadata:
     name: client-ip-reject-gateway
     namespace: gloo-system
   spec:
     httpGateway:
       virtualServices:
         - name: client-ip-reject
           namespace: gloo-system
     matcher: {}
   EOF
   ```

3. Confirm the MatchableHttpGateway was created.
  ```bash
   kubectl get -n gloo-system hgw client-ip-reject-gateway
   ```

4. Modify the Gateway CR to reference this MatchableHttpGateway.
   ```bash
   kubectl edit -n gloo-system gateway gateway-proxy
   ```
   {{< highlight yaml "hl_lines=7-11" >}}
   apiVersion: gateway.solo.io/v1
   kind: Gateway
   metadata: # collapsed for brevity
   spec:
     bindAddress: '::'
     bindPort: 8080
     hybridGateway:
       delegatedHttpGateways:
         ref:
           name: client-ip-reject-gateway
           namespace: gloo-system
     proxyNames:
    - gateway-proxy
     useProxyProto: false
   status: # collapsed for brevity
   {{< /highlight >}}
    
5. Confirm expected routing behavior.

We now have a Gateway which has matching and routing behavior defined in the MatchableHttpGateway.

At this point, all requests (an empty matcher is treated as a match-all) are expected to be matched and delegated to the `client-ip-reject` Virtual Service.

```bash
$ curl "$(glooctl proxy url)/all-pets"
client ip forbidden
```

{{% notice note %}}
Although we demonstrate gateway delegation using reference selection in this guide, label selection is also supported.
{{% /notice %}}


### Pass through unsupported ciphers based on domains

You can use the Gloo Edge Hybrid Gateway delegation feature to set up one gateway that can perform both TLS termination for traffic with supported ciphers and TLS passthrough for traffic with unsupported ciphers based on domain names (SNI matching). To implement this capability, you use the `MatchableTCPGateway` and `MatchableHTTPGateway` resources as shown in the following table. 

| Use case | Description | Gloo Edge resource to configure | 
| -- | -- | -- | 
| Cipher is supported in Envoy | When TLS traffic with a supported cipher is received, Envoy terminates the TLS connection and forwards the unencrypted HTTP traffic to the upstream server. You have the option to secure the connection from the gateway to the upstream server. For more information, see [Setting up Upstream TLS]({{< versioned_link_path fromRoot="/guides/security/tls/client_tls/" >}}). | `MatchableHTTPGateway` |
| Cipher is not supported in Envoy | To accept incoming TLS traffic for unsupported ciphers, you must add the list of unsupported ciphers and SNI domains for which you want to allow TLS passthrough to the `MatchableTCPGateway` resource. If traffic with an unsupproted cipher is received for that domain and the cipher is part of the passthrough cipher list, no TLS termination is performed by the gateway. Instead, traffic is passed through to the upstream by using Envoy's TCP proxy feature. Note that the upstream server must be capable of terminating the incoming TLS traffic. | `MatchableTCPGateway` | 

1. Set up the petstore app that you use to test TLS termination for traffic with a supported cipher. 
   1. Follow the steps to [deploy the petstore app and set up server-side TLS]({{< versioned_link_path fromRoot="/guides/security/tls/server_tls/" >}}). 
   2. Create an upstream for the petstore app. In the upstream, you add the host name and port that you want to associate with this upstream. THIS SHOULD BE DONE BY THE EXAMPLE, BUT VERIFY. 
      ```yaml
      kubectl apply -f- <<EOF
      apiVersion: gloo.solo.io/v1
      kind: Upstream
      metadata:
        name: petstore-upstream
        namespace: gloo-system
      spec:
        static:
          hosts:
            - addr: petstore.example.com
              port: 443   
      EOF
      ```
   
   3. In your virtual service resource, add the cipher suites that you want to allow for the petstore app. Note that all of these ciphers are supported in Envoy. For a list of supported cipher suites, refer to the [Envoy docs](https://www.envoyproxy.io/docs/envoy/latest/api-v3/extensions/transport_sockets/tls/v3/common.proto#extensions-transport-sockets-tls-v3-tlsparameters).
      ```yaml
      kubectl apply -f- <<EOF
      apiVersion: gateway.solo.io/v1
      kind: VirtualService
      metadata:
        name: default
        namespace: gloo-system
      spec:
        sslConfig:
          secretRef:
            name: my-cert
            namespace: gloo-system
          alpnProtocols:
            - "http/1.1"
          parameters:
            minimumProtocolVersion: TLSv1_2
            maximumProtocolVersion: TLSv1_2
            cipherSuites:
              - "ECDHE-RSA-AES256-GCM-SHA384"
              - "ECDHE-RSA-AES256-SHA"
              - "ECDHE-RSA-AES128-GCM-SHA256"
              - "ECDHE-RSA-AES128-SHA"
              - "AES256-GCM-SHA384"
              - "AES256-SHA"
              - "AES128-GCM-SHA256"
              - "AES128-SHA"
          sniDomains:
            - petstore.example.com
        virtualHost:
          domains:
            - 'petstore.example.com'
          routes:
            - matchers:
                - prefix: /
              routeAction:
                single:
                  upstream:
                    name: petstore-upstream
                    namespace: gloo-system
      EOF
      ```

   4. Create a matchable HTTP gateway resource. 
      ```yaml
      kubectl apply -f- <<EOF
      apiVersion: gateway.solo.io/v1
      kind: MatchableHttpGateway
      metadata:
        name: http-gateway
        namespace: gloo-system
        labels:
          protocol: https
          tls: termination
      spec:
        httpGateway:
          virtualServices:
            - name: default
              namespace: gloo-system
      EOF
      ```

2. Set up the hello world app that you use to test TLS passthrough for traffic with unsupported ciphers. 
   1. Deploy the app. 
   2. Create the upstream for the TLS passthrough app.  
      ```yaml
      kubectl apply -f- <<EOF
      apiVersion: gloo.solo.io/v1
      kind: Upstream 
      metadata:
        name: tls-passthrough
        namespace: gloo-system
      spec:
        static:
          hosts:
            - addr: www.passthrough.com
              port: 443
          useTls: false
        proxyProtocolVersion: V1
      EOF
      ```

   3. Create a matchable TCP gateway resource, and add the domain name (`spec.matcher.sslConfig.sniDomains`) and the OpenSSL names of the passthrough ciphers (`spec.matcher.passthroughCipherSuites`) that you want to allow for this domain. In the `spec.tcpGateway.tcpHost` section, specify the upstream server that you want to forward incoming traffic to. Note that because the TLS connection is not terminated at the gateway, the upstream server must be capable of terminating the incoming TLS request. 
      ```yaml
      apiVersion: gateway.solo.io/v1
      kind: MatchableTcpGateway
      metadata:
        name: tcp
        namespace: gloo-system
        labels:
          protocol: tcp
          tls: passthrough
      spec:
        tcpGateway:
          tcpHosts:
            - name: one
              destination:
                single:
                  upstream:
                    name: tcp-upstream
                    namespace: gloo-system
        matcher:
          sslConfig:
            sniDomains:
              - www.passthrough.com
          passthroughCipherSuites:  
            - "ECDHE-RSA-AES256-SHA384" 
            - "ECDHE-RSA-AES128-SHA256"
            - "AES256-SHA256"
            - "AES128-SHA256"
      ```

3. Create the Hybrid gateway resource and reference the matchable gateway resources that you created earlier.
   ```yaml
   kubectl apply -f- <<EOF
   apiVersion: gateway.solo.io/v1
   kind: Gateway
   metadata:
     name: gateway-proxy-ssl
     namespace: gloo-system
   spec:
     bindAddress: '::'
     bindPort: 8443
     hybridGateway:
       delegatedHttpGateways:
         selector:
           labels:
             protocol: https
             tls: termination
       delegatedTcpGateways:
         selector:
           labels:
             protocol: tcp
             tls: passthrough
     proxyNames:
       - gateway-proxy-ssl
     useProxyProto: false
   EOF
   ```

4. Verify the routing behavior. 
   1. Send a request to the `www.passthrough.com` domain with a cipher that is not part of the passthrough cipher list. 
      ```sh
      curl -vik --ciphers "ECDHE-RSA-AES256-SHA400" --resolve www.passthrough.com:443:$(glooctl proxy url) https://www.passthrough.com:443/hello
      ```

      Example output: 
      ```
      ```

   2. Send another request. This time, you provide a cipher that is part of the passthrough cipher list.
      ```sh
      curl -vik --ciphers "ECDHE-RSA-AES256-SHA384" --resolve www.passthrough.com:443:$(glooctl proxy url) https://www.passthrough.com:443/hello
      ```

      Example output: 
      ```
      ```

   3. Send a request to the `petstore.example.com` domain with a cipher that is not part of the supported cipher list. 
      ```sh
      curl -vik --ciphers "ECDHE-RSA-AES256-SHA400" --resolve petstore.example.com:443:$(glooctl proxy url) https://petstore.example.com:443/pets
      ```

      Example output: 
      ```
      ```

   4. Send another request. This time, you provide a cipher that is part of the supported cipher list. 
      ```sh
      curl -vik --ciphers "ECDHE-RSA-AES256-SHA" --resolve petstore.example.com:443:$(glooctl proxy url) https://petstore.example.com:443/pets
      ```

      Example output:
      ```
      ```
   

### How are SSL Configurations managed in Hybrid Gateways?

Before Hybrid Gateways were introduced, SSL configuration was exclusively defined on Virtual Services. This enabled the teams owning those services to define the SSL configuration required to establish connections.

With Hybrid Gateways, SSL configuration can also be defined in the matcher on the Gateway.

To support the legacy model, the SSL configuration defined on the Gateway acts as the default, and SSL configurations defined on the Virtual Services override that default.
The presence of SSL configuration on the matcher determines whether a given matched Gateway will have any SSL configuration. Therefore one can define empty SSL configuration on Gateway matchers in order to exclusively use SSL configuration from Virtual Services.