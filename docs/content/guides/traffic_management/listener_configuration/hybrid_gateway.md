---
title: Hybrid Gateway
weight: 10
description: Define multiple HTTP or TCP Gateways within a single Gateway CRD
---

Hybrid Gateways allow users to define multiple HTTP or TCP Gateways for a single Gateway CRD with distinct matching criteria. 

---

Hybrid Gateways provide all of the functionality of HTTP and TCP Gateways with the added ability to dynamically select which Gateway a given request is routed to based on request properties.
Selection is done based on `Matcher` fields, which map to a subset of Envoy [`FilterChainMatch`](https://www.envoyproxy.io/docs/envoy/latest/api-v3/config/listener/v3/listener_components.proto#config-listener-v3-filterchainmatch) fields.

## Only accept requests from a particular CIDR range

Hybrid Gateways allow us to treat traffic from particular IPs differently.
One case where this might come in handy is if a set of clients are at different stages of migrating to TLS >=1.2 support, and therefore we want to enforce different TLS requirements depending on the client.
If the clients originate from the same domain, it may be necessary to dynamically route traffic to the appropriate Gateway based on source IP.

In this example we will demonstrate how to only allow requests from one IP to reach an upstream while short-circuiting all other IPs with a direct response action.

We will pick up where the [Hello World guide]({{< versioned_link_path fromRoot="/guides/traffic_management/hello_world" >}}) leaves off.

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


Next let's update the existing `gateway-proxy` Gateway CRD, replacing the default `httpGateway` with a [`hybridGateway`]({{< versioned_link_path fromRoot="/reference/api/github.com/solo-io/gloo/projects/gateway/api/v1/gateway.proto.sk/#hybridgateway" >}}) as follows:
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

Note: We use a range of 0.0.0.0/1 in order to have a high chance of matching the client's IP without knowing it specifically. A different and/or narrower range may be used if we know more about the client's IP.

This results in a proxy that looks like:

```yaml
apiVersion: gloo.solo.io/v1
kind: Proxy
metadata: # collapsed for brevity
spec:
  listeners:
  - bindAddress: '::'
    bindPort: 8080
    hybridListener:
      matchedListeners:
        - httpListener:
            virtualHosts:
              - domains:
                  - '*'
                metadata: # collapsed for brevity
                name: gloo-system.default
                routes:
                  - matchers:
                      - exact: /all-pets
                    metadata: # collapsed for brevity
                    options:
                      prefixRewrite: /api/pets
                    routeAction:
                      single:
                        upstream:
                          name: default-petstore-8080
                          namespace: gloo-system
          matcher:
            sourcePrefixRanges:
              - addressPrefix: 0.0.0.0
                prefixLen: 1
        - httpListener:
            virtualHosts:
              - domains:
                  - '*'
                metadata: # collapsed for brevity
                name: gloo-system.client-ip-reject
                routes:
                - directResponseAction:
                    body: |
                      client ip forbidden
                    status: 403
                  matchers:
                  - prefix: /
                  metadata: # collapsed for brevity
            matcher: {}
    metadata: # collapsed for brevity
    name: listener-::-8080
    useProxyProto: false
  - bindAddress: '::'
    bindPort: 8443
    httpListener: {}
    metadata: # collapsed for brevity
status: # collapsed for brevity
```

We can make a request to the proxy and will find that we get the `200` response:

```bash
$ curl "$(glooctl proxy url)/all-pets"
[{"id":1,"name":"Dog","status":"available"},{"id":2,"name":"Cat","status":"pending"}]
```

Also observe that if we make a request to an endpoint not matched by the `default` VirtualService we get a `404` response and _do not_ hit the `client-ip-reject` VirtualService:
```bash
$ curl -i "$(glooctl proxy url)/foo"
HTTP/1.1 404 Not Found
date: Tue, 07 Dec 2021 17:48:49 GMT
server: envoy
content-length: 0
```
This is because the `Matcher`s in the `HybridGateway` determine which `MatchedGateway` a request will be routed to, regardless of what routes that gateway has.

### Observe that request from unmatched IP hits catchall gateway 
If we update the matcher to have a specific IP range that our client's IP is not a member of, we will expect our request to miss the matcher and fall through to the catchall gateway which is configured to respond `403`.

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

We can now make a request to the proxy and will find that we get the `403` response for any endpoint:

```bash
$ curl "$(glooctl proxy url)/all-pets"
client ip forbidden
```

```bash
$ curl "$(glooctl proxy url)/foo"
client ip forbidden
```

This is expected since the IP of our client is not `1.2.3.4`.