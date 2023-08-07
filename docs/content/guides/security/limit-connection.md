---
title: Limit active TCP connections
weight: 50
description: Restrict the number of active TCP connections for a gateway. 
---

You can restrict the number of active TCP connections for a gateway and optionally wait before closing a connection by using the `optionsConnectionLimit` parameter in the gateway resource. These settings greatly reduce the risk of malicious attacks and make sure that each gateway has its fair share of compute resources to process incoming requests. 

For more information about these settings, see the [Envoy documentation](https://www.envoyproxy.io/docs/envoy/latest/configuration/listeners/network_filters/connection_limit_filter)

## Before you begin

Follow the [Hello World guide]({{< versioned_link_path fromRoot="/guides/traffic_management/hello_world/" >}}) to deploy the petstore app as an upstream and configure routing to the upstream. 

## Configure connection limits

1. Verify that you can send requests to the petstore app. 
   ```sh
   curl $(glooctl proxy url)/all-pets
   ```

2. Create a gateway resource. 
   ```yaml
   kubectl apply -f- <<EOF
   apiVersion: gateway.solo.io/v1
   kind: Gateway
   metadata: # collapsed for brevity
   spec:
     bindAddress: '::'
     bindPort: 8080
     httpGateway:
       options:
         ConnectionLimit:
           delayBeforeClose: 3s
           maxActiveConnections: 3
     useProxyProto: false
   EOF
   ```

2. 

