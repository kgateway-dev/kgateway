---
title: Hybrid Gateway
weight: 10
description: Define multiple HTTP or TCP Gateways within a single Gateway CRD
---

Hybrid Gateways allow you to define multiple HTTP or TCP Gateways for a single Gateway CRD with distinct matching criteria.

---

## Only accept requests from a particular IP

Hybrid Gateways allow us to treat traffic from particular IPs differently.

In this example we will demonstrate how to only allow requests from one IP to reach an upstream while short-circuiting all other IPs with a direct response action.

We will pick up where the [Hello World guide]({{< versioned_link_path fromRoot="/guides/traffic_management/hello_world" >}}) leaves off.

To start we will add a second VirtualService that also matches the `/all-pets` endpoint but which has a directResponseAction:

```yaml
kubectl apply -f - << EOF
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
          - exact: /all-pets
        directResponseAction:
          status: 401
          body: 'client ip forbidden'
EOF
```


Next let's update the existing `gateway-proxy` Gateway CRD, replacing the default `httpGateway` with a [`hybridGateway`]({{< versioned_link_path fromRoot="/reference/api/github.com/solo-io/gloo/projects/gateway/api/v1/gateway.proto.sk/#hybridgateway" >}}) as follows:

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

This results in a proxy that looks like:

```yaml
apiVersion: gloo.solo.io/v1
kind: Proxy
metadata: # collapsed for bevity
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
                metadata: # collapsed for bevity
                name: gloo-system.default
                routes:
                  - matchers:
                      - exact: /all-pets
                    metadata: # collapsed for bevity
                    options:
                      prefixRewrite: /api/pets
                    routeAction:
                      single:
                        upstream:
                          name: default-petstore-8080
                          namespace: gloo-system
          matcher:
            sourcePrefixRanges:
              - addressPrefix: 1.2.3.4
                prefixLen: 32
        - httpListener:
            virtualHosts:
              - domains:
                  - '*'
                metadata: # collapsed for bevity
                name: gloo-system.client-ip-reject
                routes:
                  - directResponseAction:
                      body: client ip forbidden
                      status: 401
                    matchers:
                      - exact: /all-pets
                    metadata: # collapsed for bevity
                    options:
                      prefixRewrite: /api/pets
          matcher: {}
    metadata: # collapsed for bevity
    name: listener-::-8080
    useProxyProto: false
  - bindAddress: '::'
    bindPort: 8443
    httpListener: {}
    metadata: # collapsed for bevity
status: # collapsed for bevity
```

We can make a request to the proxy and will find that we get the `401` response:

```bash
$ curl "$(glooctl proxy url)/all-pets"
client ip forbidden
```

This is expected since the IP of our client is not `1.2.3.4`.

TODO: IP spoof in order to hit the other filter chain.