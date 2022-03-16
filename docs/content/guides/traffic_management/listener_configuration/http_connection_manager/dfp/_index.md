---
title: Dynamic Forward Proxy
weight: 10
---

This document introduces the **Dynamic Forward Proxy** HTTP filter in Gloo Edge.

In a highly dynamic environment with services coming up and down and with no service registry being able to list the available endpoints, one option is to somehow "blindly" route the client requests upstream. 

There are a few downsides to such flexibility:
- since there is no pre-defined {{< protobuf name="gloo.solo.io.Upstream" display="Upstream" >}} to designate the upstream service, you cannot configure failover policies or client load-balancing
- DNS resolution is done at runtime. Typically, when a domain name is met for the first time, Envoy will pause the request and synchronously resolve this domain to get the endpoints (IP addresses). Then, these entries are put into a local cache

Of course, there are also good reasons why this still makes sense in an API Gateway:
- you will easily get metrics on the traffic going through the proxy
- you can enforce authentication and authorization policies
- you can leverage other policies available in Gloo Edge Enterprise, like the WAF (Web Application Firewall) or DLP (Data Loss Prevention)

## Enabling the Dynamic Forward Proxy

First, you need to enable the DFP filter at the Gateway level:

```bash
kubectl -n gloo-system patch gw/gateway-proxy --type merge -p "
spec:
  httpGateway:
    options:
      dynamicForwardProxy: {}
"
```

Then you need to capture the actual destination of the client request. It can simply be the `Host` header in the most basic setup, but it can be hidden in other client request headers or body parts. In this latter case, you can create that header dynamically, using a transformation template.

Below is a simple example showing how you can apply the dynamic forward option to a route:

```yaml
apiVersion: gateway.solo.io/v1
kind: VirtualService
metadata:
  name: test-static
  namespace: gloo-system
spec:
  virtualHost:
    domains:
      - 'foo'
    routes:
      - matchers:
         - prefix: /
        routeAction:
          dynamicForwardProxy:
            autoHostRewriteHeader: "x-rewrite-me" # host header will be rewritten to the value of this header
```




