---
title: TCP gateway
weight: 30
description: Apply local rate limiting settigns to the TCPP Envoy filter for Layer 4 traffic.
---

Use the local rate limiting settings on the TCP gateway resource to limit the number of incoming TCP requests. The local rate limiting filter is applied before the TLS handshake between the client and server is started. If no tokens are available in the TCP gateway, the connection is dropped immediately. 

For more information, see [About local rate limiting]({{% versioned_link_path fromRoot="/guides/security/local_rate_limiting/overview/" %}})

1. Deploy the TCP echo service in your cluster.
   {{< tabs >}}
   {{% tab %}}
   ```yaml
   kubectl apply -f- <<EOF
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

   Example output:
   ```
   pod/tcp-echo created
   service/tcp-echo created
   ```
   {{% /tab %}}
   {{< /tabs >}}

2. Verify that an upstream was automatically created for the echo service.
   ```sh
   kubectl get upstreams default-tcp-echo-1025 -n gloo-system
   ```

3. Create a TCP gateway with local rate limiting settings. The following gateway configures the token bucket with 1 token that is refilled every 100 seconds. 
   ```yaml
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
       options: 
         localRatelimit: 
           maxTokens: 1
           tokensPerFill: 1
           fillInterval: 100s
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

4. Open a TCP port on the `gateway-proxy` service in your cluster and bind it to the gateway port 8000.
   1. Edit the `gateway-proxy` service. 
      ```sh
      kubectl edit service gateway-proxy -n gloo-system
      ```
   2. In the `spec.ports` section, add the TCP port.
      ```yaml
      ...
      - name: tcp
        nodePort: 30197
        port: 8000
        protocol: TCP
        targetPort: 8000
      ```

      Your `spec.ports` section looks similar to the following:
      ```yaml
      ports:
      - name: http
        nodePort: 32653
        port: 80
        protocol: TCP
        targetPort: 8080
      - name: https
        nodePort: 30550
        port: 443
        protocol: TCP
        targetPort: 8443
      - name: tcp
        nodePort: 30197
        port: 8000
        protocol: TCP
      ```

5. Get the public IP address of your gateway proxy. Note that the following command returns the IP address and the default port. 
   ```sh
   glooctl proxy address
   ```

6. Open a telnet session to the public IP address of the gateway and port 8000.
   ```sh
   telnet <public-gateway-IP> 8000
   ```

   Example output:
   ```
   Connected to 113.21.184.35.bc.googleusercontent.com.
   Escape character is '^]'.
   ```

8. Enter any string and verify that the echo service returns the same string. For example, you can enter `hello`.
   ```sh
   hello
   ```

   Example output:
   ```
   hello
   hello
   ```

9. Open another terminal window and try to establish another connection to the gateway on port 8000. Because the gateway is configured with a maxinum number of 1 token at any given time, the new connection is terminated immediately as no tokens are available that can be assigned to the connection. 
   ```sh
   telnet <public-gateway-IP> 8000
   ```

   Example output:
   ```
   Connected to 113.21.184.35.bc.googleusercontent.com.
   Escape character is '^]'.
   Connection closed by foreign host.
   ```


## Cleanup

You can optionally clean up the resources that you created as part of this guide. 

1. Remove the TCP gateway.
   ```sh
   kubectl delete gateway tcp -n gloo-system
   ```

2. Remove the echo pod and service.
   ```sh
   kubectl delete service tcp-echo
   kubectl delete pod tcp-echo
   ```

3. Edit the `gateway-proxy` service and remove the TCP port settings.
   ```sh
   kubectl edit service gateway-proxy -n gloo-system
   ```



