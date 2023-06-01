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

## Pass TLS traffic through for deprecated cipher suites {#tls-passthrough}

{{% notice note %}}
This feature requires a Gloo Edge Enterprise license. 
{{% /notice %}}

By default, Gloo Edge only supports cipher suites that are available in Envoy, which uses [BoringSSL](https://github.com/google/boringssl) for TLS. If you provide services to clients that do not yet have support for the ciphers that are supported by BoringSSL, you can configure Gloo Edge to pass TLS traffic through to your upstream directly. Because TLS traffic is not terminated at the gateway, the upstream service must be capable of terminating and unencrypting the incoming TLS connection.

In this guide, you deploy a sample NGINX server and configure the server for HTTPS traffic. You use the server to try out the TLS passthrough feature for deprecated cipher suites.

{{% notice tip %}}
This guide assumes that you want to configure TLS passthrough in the gateway resource directly. If you have a hybrid gateway and you use the hybrid gateway delegation, follow the steps in [Pass TLS traffic through for depcrecated cipher suites with hybrid gateway delegation](#tls-passthrough-delegated). 
{{% /notice %}}

1. Create a root certificate for the `example.com` domain. You use this certificate to sign the certificate for your NGINX service later. 
   ```shell
   mkdir example_certs
   openssl req -x509 -sha256 -nodes -days 365 -newkey rsa:2048 -subj '/O=example Inc./CN=example.com' -keyout example_certs/example.com.key -out example_certs/example.com.crt
   ```
   
2. Create a server certificate and private key for the `nginx.example.com` domain. 
   ```
   openssl req -out example_certs/nginx.example.com.csr -newkey rsa:2048 -nodes -keyout example_certs/nginx.example.com.key -subj "/CN=nginx.example.com/O=some organization"
   openssl x509 -req -sha256 -days 365 -CA example_certs/example.com.crt -CAkey example_certs/example.com.key -set_serial 0 -in example_certs/nginx.example.com.csr -out example_certs/nginx.example.com.crt
   ```

3. Create a secret that stores the certificate and key for the NGINX server. 
   ```shell
   kubectl create secret tls nginx-server-certs \
    --key example_certs/nginx.example.com.key \
    --cert example_certs/nginx.example.com.crt
   ```
   
4. Prepare your NGINX configuration. The following example configures NGINX for HTTPS traffic with the certificate that you created earlier.
   ```shell
   cat <<\EOF > ./nginx.conf
   events {
   }

   http {
     log_format main '$remote_addr - $remote_user [$time_local]  $status '
     '"$request" $body_bytes_sent "$http_referer" '
     '"$http_user_agent" "$http_x_forwarded_for"';
     access_log /var/log/nginx/access.log main;
     error_log  /var/log/nginx/error.log;

     server {
       listen 443 ssl;

       root /usr/share/nginx/html;
       index index.html;

       server_name nginx.example.com;
       ssl_certificate /etc/nginx-server-certs/tls.crt;
       ssl_certificate_key /etc/nginx-server-certs/tls.key;
     }
   }
   EOF
   ```
   
5. Store the NGINX configuration in a configmap. 
   ```shell
   kubectl create configmap nginx-configmap --from-file=nginx.conf=./nginx.conf
   ```

6. Deploy the NGINX server. 
   ```yaml
   kubectl apply -f- <<EOF
   apiVersion: v1
   kind: Service
   metadata:
     name: my-nginx
     labels:
       run: my-nginx
   spec:
     ports:
     - port: 443
       protocol: TCP
     selector:
       run: my-nginx
   EOF
   ```
   ```yaml
   kubectl apply -f- <<EOF
   apiVersion: apps/v1
   kind: Deployment
   metadata:
     name: my-nginx
   spec:
     selector:
       matchLabels:
         run: my-nginx
     replicas: 1
     template:
       metadata:
         labels:
           run: my-nginx
           sidecar.istio.io/inject: "true"
       spec:
         containers:
         - name: my-nginx
           image: nginx
           ports:
           - containerPort: 443
           volumeMounts:
           - name: nginx-config
             mountPath: /etc/nginx
             readOnly: true
           - name: nginx-server-certs
             mountPath: /etc/nginx-server-certs
             readOnly: true
         volumes:
         - name: nginx-config
           configMap:
             name: nginx-configmap
         - name: nginx-server-certs
           secret:
             secretName: nginx-server-certs
   EOF
   ```

7. Verify that an upstream was created for your nginx server.
   ```sh
   kubectl get upstream default-my-nginx-443 -n gloo-system 
   ```

8. Create a gateway resource and specify the deprecated cipher suites for which you want to pass TLS traffic through to your upstream in the `passthroughCipherSuites` field of your `tcpGateway`. You can optionally log the ciphers that were sent by the clients by adding the `spec.options.accessLoggingService` section to your gateway configuration as shown in the following example. 
   {{< highlight yaml "hl_lines=12-16 31-35" >}}
   kubectl apply -f- <<EOF
   apiVersion: gateway.solo.io/v1
   kind: Gateway
   metadata:
     name: gateway-proxy-ssl
     namespace: gloo-system
   spec:
     bindAddress: '::'
     bindPort: 8443
     options:
       accessLoggingService:
         accessLog:
         - fileSink:
             path: /dev/shm/access_log.txt
             stringFormat: >
               [%START_TIME%] %REQUESTED_SERVER_NAME% %FILTER_STATE(io.solo.cipher_detection_info)%
     hybridGateway:
       matchedGateways:
       - tcpGateway:
           options:
             tcpProxySettings:
               accessLogFlushInterval: 2s
           tcpHosts:
           - destination:
               single:
                 upstream:
                   name: default-my-nginx-443
                   namespace: gloo-system
             name: one
         matcher:
           passthroughCipherSuites:
           - ECDHE-RSA-AES256-SHA384
           - ECDHE-RSA-AES128-SHA256
           - AES256-SHA256
           - AES128-SHA256
           sslConfig:
             sniDomains:
             - nginx.example.com
     proxyNames:
     - gateway-proxy
     useProxyProto: false
   EOF
   ```

9. Get the IP address of the gateway.
   ```sh
   export GATEWAY_IP=$(kubectl get svc -n gloo-system gateway-proxy -o jsonpath='{.status.loadBalancer.ingress[0].ip}')
   ```

11. Send a request to the gateway. In your output, verify that you see the TLS handshake between the client and the gateway and that the cipher suite that you sent as part of the request is accepted. In addition, verify that you get back a 200 HTTP response code from the NGINX server and that you see the NGINX server certificate in your CLI output. 
    ```sh
    curl -vi --resolve nginx.example.com:443:${GATEWAY_IP} --cacert example_certs/nginx.example.com.crt --cipher "AES256-SHA256" "https://nginx.example.com:443"
    ```

    Example output: 
    ```
    * Added nginx.example.com:443:34.134.215.40 to DNS cache
    * Hostname nginx.example.com was found in DNS cache
    *   Trying 34.134.215.40:443...
    * Connected to nginx.example.com (34.134.215.40) port 443 (#0)
    * ALPN, offering h2
    * ALPN, offering http/1.1
    * Cipher selection: AES256-SHA256
    * successfully set certificate verify locations:
    *  CAfile: example_certs/nginx.example.com.crt
    *  CApath: none
    * TLSv1.2 (OUT), TLS handshake, Client hello (1):
    * TLSv1.2 (IN), TLS handshake, Server hello (2):
    * TLSv1.2 (IN), TLS handshake, Certificate (11):
    * TLSv1.2 (IN), TLS handshake, Server finished (14):
    * TLSv1.2 (OUT), TLS handshake, Client key exchange (16):
    * TLSv1.2 (OUT), TLS change cipher, Change cipher spec (1):
    * TLSv1.2 (OUT), TLS handshake, Finished (20):
    * TLSv1.2 (IN), TLS change cipher, Change cipher spec (1):
    * TLSv1.2 (IN), TLS handshake, Finished (20):
    * SSL connection using TLSv1.2 / AES256-SHA256
    * ALPN, server accepted to use http/1.1
    * Server certificate:
    *  subject: CN=nginx.example.com; O=some organization
    *  start date: May 22 18:46:27 2023 GMT
    *  expire date: May 21 18:46:27 2024 GMT
    *  common name: nginx.example.com (matched)
    *  issuer: O=example Inc.; CN=example.com
    *  SSL certificate verify ok.
    > GET / HTTP/1.1
    > Host: nginx.example.com
    > User-Agent: curl/7.77.0
    > Accept: */*

    * Mark bundle as not supporting multiuse
    < HTTP/1.1 200 OK
    HTTP/1.1 200 OK
    ```

12. Send another request to the gateway. This time, you provide a cipher that is not listed in the deprecated cipher suites. Note that the TLS connection fails as no supported cipher can be found in the request. 
    ```sh
    curl -vi --resolve nginx.example.com:443:${GATEWAY_IP} --cacert example_certs/nginx.example.com.crt --cipher "AES128" "https://nginx.example.com:443" 
    ```

    Example output: 
    ```
    * Added nginx.example.com:443:34.134.215.40 to DNS cache
    * Hostname nginx.example.com was found in DNS cache
    *   Trying 34.134.215.40:443...
    * Connected to nginx.example.com (34.134.215.40) port 443 (#0)
    * ALPN, offering h2
    * ALPN, offering http/1.1
    * Cipher selection: AES128
    * successfully set certificate verify locations:
    *  CAfile: example_certs/nginx.example.com.crt
    *  CApath: none
    * TLSv1.2 (OUT), TLS handshake, Client hello (1):
    * LibreSSL SSL_connect: Connection reset by peer in connection to nginx.example.com:443 
    * Closing connection 0
    curl: (35) LibreSSL SSL_connect: Connection reset by peer in connection to nginx.example.com:443 
    ```

## Hybrid Gateway Delegation

{{% notice note %}}
This feature is available in Gloo Edge version 1.10.x and later.
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
    
5. Confirm expected routing behavior. We now have a Gateway which has matching and routing behavior defined in the `MatchableHttpGateway`. At this point, all requests (an empty matcher is treated as a match-all) are expected to be matched and delegated to the `client-ip-reject` Virtual Service.

   ```bash
   $ curl "$(glooctl proxy url)/all-pets"
   client ip forbidden
   ```

{{% notice note %}}
Although we demonstrate gateway delegation using reference selection in this guide, label selection is also supported.
{{% /notice %}}


### Pass TLS traffic through for deprecated ciphers suites with hybrid gateway delegation {#tls-passthrough-delegated}

{{% notice note %}}
This feature requires a Gloo Edge Enterprise license. 
{{% /notice %}}

By default, Gloo Edge supports cipher suites that are available in Envoy. If you have an upstream service that can accept only cipher suites that are deprecated or not available in Envoy, you can configure Gloo Edge to pass TLS traffic through to your upstream directly. Because TLS traffic is not terminated at the gateway, the upstream service must be capable of terminating and unencrypting the incoming TLS connection.

In this guide, you deploy a sample NGINX server and configure the server for HTTPS traffic. You use the server to try out the TLS passthrough feature for deprecated cipher suites.

{{% notice tip %}}
This guide assumes that you want to use hybrid gateway delegatin to configure TLS passthrough for your upstream. If you want to put this configuration in your gateway directly, follow the steps in [Pass TLS traffic through for depcrecated cipher suites](#tls-passthrough). 
{{% /notice %}}

1. Follow steps 1-7 in [Pass TLS traffic through for deprecated cipher suites](#tls-passthrough) to deploy the NGINX server and set it up for HTTPS traffic. 
2. Create a `MatchableTCPGateway` 
   ```yaml
   kubectl apply -f- <<EOF
   apiVersion: gateway.solo.io/v1
   kind: MatchableTcpGateway
   metadata:
     name: nginx-tpc-gateway
     namespace: gloo-system
     labels:
       protocol: tcp
       tls: passthrough
   spec:
     tcpGateway:
       options:
         tcpProxySettings:
           accessLogFlushInterval: 2s
       tcpHosts:
       - destination:
           single:
             upstream:
               name: default-my-nginx-443
               namespace: gloo-system
         name: one
     matcher:
       passthroughCipherSuites:
       - ECDHE-RSA-AES256-SHA384
       - ECDHE-RSA-AES128-SHA256
       - AES256-SHA256
       - AES128-SHA256
       sslConfig:
         sniDomains:
         - nginx.example.com
   EOF
   ```

3. Create a gateway resource. In the `hybridGateway` section, add a `delegatedTcpGateways` section and make sure that the protocol is set to `tcp` and the TLS mode to `passthrough`. You can optionally log the ciphers that were sent by the clients by adding the `spec.options.accessLoggingService` section to your gateway configuration as shown in the following example. 
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
     options:
       accessLoggingService:
         accessLog:
         - fileSink:
             path: /dev/shm/access_log.txt
             stringFormat: >
               [%START_TIME%] %REQUESTED_SERVER_NAME% %FILTER_STATE(io.solo.cipher_detection_info)%
     hybridGateway:
       delegatedTcpGateways:
         selector:
           labels:
             protocol: tcp
             tls: passthrough
     proxyNames:
     - gateway-proxy
     useProxyProto: false
   EOF
   ```
  
4. Get the IP address of the gateway.
   ```sh
   export GATEWAY_IP=$(kubectl get svc -n gloo-system gateway-proxy -o jsonpath='{.status.loadBalancer.ingress[0].ip}')
   ```

5. Send a request to the gateway. In your output, verify that you see the TLS handshake between the client and the gateway and that the cipher suite that you sent as part of the request is accepted. In addition, verify that you get back a 200 HTTP response code from the NGINX server and that you see the NGINX server certificate in your CLI output. 
   ```sh
   curl -vi --resolve nginx.example.com:443:${GATEWAY_IP} --cacert example_certs/nginx.example.com.crt --cipher "AES256-SHA256" "https://nginx.example.com:443"
   ```
  
   Example output: 
   ```
   * Added nginx.example.com:443:34.134.215.40 to DNS cache
   * Hostname nginx.example.com was found in DNS cache
   *   Trying 34.134.215.40:443...
   * Connected to nginx.example.com (34.134.215.40) port 443 (#0)
   * ALPN, offering h2
   * ALPN, offering http/1.1
   * Cipher selection: AES256-SHA256
   * successfully set certificate verify locations:
   *  CAfile: example_certs/nginx.example.com.crt
   *  CApath: none
   * TLSv1.2 (OUT), TLS handshake, Client hello (1):
   * TLSv1.2 (IN), TLS handshake, Server hello (2):
   * TLSv1.2 (IN), TLS handshake, Certificate (11):
   * TLSv1.2 (IN), TLS handshake, Server finished (14):
   * TLSv1.2 (OUT), TLS handshake, Client key exchange (16):
   * TLSv1.2 (OUT), TLS change cipher, Change cipher spec (1):
   * TLSv1.2 (OUT), TLS handshake, Finished (20):
   * TLSv1.2 (IN), TLS change cipher, Change cipher spec (1):
   * TLSv1.2 (IN), TLS handshake, Finished (20):
   * SSL connection using TLSv1.2 / AES256-SHA256
   * ALPN, server accepted to use http/1.1
   * Server certificate:
   *  subject: CN=nginx.example.com; O=some organization
   *  start date: May 22 18:46:27 2023 GMT
   *  expire date: May 21 18:46:27 2024 GMT
   *  common name: nginx.example.com (matched)
   *  issuer: O=example Inc.; CN=example.com
   *  SSL certificate verify ok.
   > GET / HTTP/1.1
   > Host: nginx.example.com
   > User-Agent: curl/7.77.0
   > Accept: */*

   * Mark bundle as not supporting multiuse
   < HTTP/1.1 200 OK
   HTTP/1.1 200 OK
   ```

6. Send another request to the gateway. This time, you provide a cipher that is not listed in the deprecated cipher suites. Note that the TLS connection fails as no supported cipher can be found in the request. 
   ```sh
   curl -vi --resolve nginx.example.com:443:${GATEWAY_IP} --cacert example_certs/nginx.example.com.crt --cipher "AES128" "https://nginx.example.com:443" 
   ```

   Example output: 
   ```
   * Added nginx.example.com:443:34.134.215.40 to DNS cache
   * Hostname nginx.example.com was found in DNS cache
   *   Trying 34.134.215.40:443...
   * Connected to nginx.example.com (34.134.215.40) port 443 (#0)
   * ALPN, offering h2
   * ALPN, offering http/1.1
   * Cipher selection: AES128
   * successfully set certificate verify locations:
   *  CAfile: example_certs/nginx.example.com.crt
   *  CApath: none
   * TLSv1.2 (OUT), TLS handshake, Client hello (1):
   * LibreSSL SSL_connect: Connection reset by peer in connection to nginx.example.com:443 
   * Closing connection 0
   curl: (35) LibreSSL SSL_connect: Connection reset by peer in connection to nginx.example.com:443 
   ```

### How are SSL Configurations managed in Hybrid Gateways?

Before Hybrid Gateways were introduced, SSL configuration was exclusively defined on Virtual Services. This enabled the teams owning those services to define the SSL configuration required to establish connections.

With Hybrid Gateways, SSL configuration can also be defined in the matcher on the Gateway.

To support the legacy model, the SSL configuration defined on the Gateway acts as the default, and SSL configurations defined on the Virtual Services override that default.
The presence of SSL configuration on the matcher determines whether a given matched Gateway will have any SSL configuration. Therefore one can define empty SSL configuration on Gateway matchers in order to exclusively use SSL configuration from Virtual Services.
