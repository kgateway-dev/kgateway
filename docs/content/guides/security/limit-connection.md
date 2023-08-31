---
title: Limit active connections
weight: 35
description: Restrict the number of active TCP connections for a gateway. 
---

You can configure the `options.ConnectionLimit` parameters in the gateway resource. These parameters let you restrict the number of active TCP connections for the gateway. You can also optionally make the gateway wait before closing a connection. Similar to the [rate limit filter]({{< versioned_link_path fromRoot="/guides/security/rate_limiting/" >}}) that limits requests based on connection rate, the connection limit filter limits traffic based on active connections. This connection limit reduces the risk of malicious attacks. In turn, the limit helps ensure that each gateway has enough compute resources to process incoming requests.

{{% notice note %}}
The TCP connection filter is a Layer 4 filter and is executed before the HTTP Connection Manager plug-in and related filters. 
{{% /notice %}}

For more information about the connection limit settings, see the [Envoy documentation](https://www.envoyproxy.io/docs/envoy/latest/configuration/listeners/network_filters/connection_limit_filter).

## Before you begin

Install the `telnet` client on your local machine. You use this client to establish TCP connections to the gateway. For example, on macOS you can run `brew install telnet` to install the client. 

## Configure connection limits

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

3. Create a TCP gateway with connection limit settings. The following gateway accepts only one active connection at any given time. Before closing a new connection, the gateway waits 2 seconds. 
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
         connectionLimit:
           maxActiveConnections: 1
           delayBeforeClose: 2s
       tcpHosts:
       - name: one
         destination:
           single:
             upstream:
               name: default-tcp-echo-1025
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

9. Open another terminal window and try to establish another connection to the gateway on port 8000. Because the gateway is configured to allow only one connection at a time, the connection is terminated after the 2 second delay. 
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

