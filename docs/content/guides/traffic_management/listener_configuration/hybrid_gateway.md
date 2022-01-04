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