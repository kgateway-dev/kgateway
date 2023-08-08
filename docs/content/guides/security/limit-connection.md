---
title: Limit active connections
weight: 35
description: Restrict the number of active TCP connections for a gateway. 
---

You can restrict the number of active TCP connections for a gateway and optionally instruct the gateway to wait before closing a connection by using the `optionsConnectionLimit` parameter in the gateway resource. Similar to the [rate limit filter]({{< versioned_link_path fromRoot="/guides/security/rate_limiting/" >}}) where requests are limited based on connection rate, the connection limit filter limits traffic based on active connections which greatly reduces the risk of malicious attacks and makes sure that each gateway has its fair share of compute resources to process incoming requests. 

{{% notice note %}}
The TCP connection filter is a Layer 4 filter and is executed before the HTTP Connection Manager plug-in and related filters. 
{{% /notice %}}

For more information about the connection limit settings, see the [Envoy documentation](https://www.envoyproxy.io/docs/envoy/latest/configuration/listeners/network_filters/connection_limit_filter)

## Before you begin

Follow the [Hello World guide]({{< versioned_link_path fromRoot="/guides/traffic_management/hello_world/" >}}) to deploy the petstore app as an upstream and configure routing to the upstream. 

## Configure connection limits

1. Verify that you can send requests to the petstore app. 
   ```sh
   curl $(glooctl proxy url)/all-pets
   ```

2. Create a gateway resource with connection limit settings. In this example, the gatewy accepts only one active connection at any given time. 
   ```yaml
   kubectl apply -f- <<EOF
   apiVersion: gateway.solo.io/v1
   kind: Gateway
   metadata: 
     name: tcp-limit
     namespace: gloo-system
   spec:
     bindAddress: '::'
     bindPort: 8080
     httpGateway:
       options:
         connectionLimit:
           delayBeforeClose: 3s
           maxActiveConnections: 1
     useProxyProto: false
   EOF
   ```

3. To do

