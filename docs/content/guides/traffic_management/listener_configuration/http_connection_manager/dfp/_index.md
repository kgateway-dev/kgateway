---
title: Dynamic Forward Proxy
weight: 10
---

You can set up an [HTTP Dynamic Forward Proxy (DFP) filter](https://www.envoyproxy.io/docs/envoy/latest/intro/arch_overview/http/http_proxy) in Gloo Edge.

In a highly dynamic environment with services coming up and down and with no service registry being able to list the available endpoints, one option is to somehow "blindly" route the client requests upstream. 

Another popular use case is to deploy a forward proxy for all egress traffic. This way, you can observe and control outbound traffic. A common security policy applied here is rate-limiting and, of course, gathering access logs.

So there are two sorts of usage for a forward proxy:
- Route all the traffic of your local network to an exit node with the forward proxy, to monitor and control all the egress traffic.
- Apply the forward proxy filter only to certain routes, which don’t have a pre-defined destination, and dynamically build the final Host value.

Before implementing a dynamic forward proxy, consider the downsides to such flexibility:
- Because no pre-defined {{< protobuf name="gloo.solo.io.Upstream" display="Upstream" >}} designates the upstream service, you cannot configure failover policies or client load-balancing.
- DNS resolution is done at runtime. Typically, when Envoy encounters a domain name the first time, Envoy pauses the request and synchronously resolves this domain to get the endpoints (IP addresses). Then, these entries are put into a local cache.

Of course, you might still decide to use a dynamic forward proxy in an API Gateway for benefits such as the following:
- You easily get metrics on the egress traffic that goes through the forward proxy.
- You can enforce authentication and authorization policies.
- You can leverage other policies available in Gloo Edge Enterprise, like Web Application Firewall (WAF) or Data Loss Prevention (DLP).

## Enabling the Dynamic Forward Proxy

First, enable the DFP filter in your Gateway configuration.

```bash
kubectl -n gloo-system patch gw/gateway-proxy --type merge -p "
spec:
  httpGateway:
    options:
      dynamicForwardProxy: {}
"
```

Then, set the actual destination of the client request. The destination can be the `Host` header in the most basic setup. However, the destination might also be hidden in other client request headers or body parts. In this latter case, you can create the header dynamically by using a transformation template.

Review the following basic example to see how you can apply the dynamic forward option to a route.

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




