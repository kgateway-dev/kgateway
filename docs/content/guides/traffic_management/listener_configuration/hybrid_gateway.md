---
title: Hybrid Gateway
weight: 10
description: Define multiple HTTP or TCP Gateways within a single Gateway CRD
---

Hybrid Gateways allow you to define multiple HTTP or TCP Gateways for a single Gateway CRD with distinct matching criteria.

---

If we add a virtual service to the Hello World setup called `client-ip-reject`, in this example with a direct response action that responds `401`, then we can ensure that traffic originating from a particular IP, and only that IP, gets the `401` response rather than reaching the petstore upstream, by creating a hybrid gateway can be created by editing the `Gateway` CRD like so:

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
        matcher: {}
      - httpGateway:
          virtualServices:
            - name: client-ip-reject
              namespace: gloo-system
        matcher:
          sourcePrefixRanges:
            - addressPrefix: 1.2.3.4
              prefixLen: 32
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
            metadata:
              sources:
              - kind: '*v1.VirtualService'
                name: default
                namespace: gloo-system
                observedGeneration: 13
            name: gloo-system.default
            routes:
            - matchers:
              - exact: /all-pets
              metadata:
                sources:
                - kind: '*v1.VirtualService'
                  name: default
                  namespace: gloo-system
                  observedGeneration: 13
              options:
                prefixRewrite: /api/pets
              routeAction:
                single:
                  upstream:
                    name: default-petstore-8080
                    namespace: gloo-system
        matcher: {}
      - httpListener:
          virtualHosts:
          - domains:
            - '*'
            metadata:
              sources:
              - kind: '*v1.VirtualService'
                name: client-ip-reject
                namespace: gloo-system
                observedGeneration: 1
            name: gloo-system.client-ip-reject
            routes:
            - directResponseAction:
                body: sni domain forbidden
                status: 401
              matchers:
              - exact: /all-pets
              metadata:
                sources:
                - kind: '*v1.VirtualService'
                  name: client-ip-reject
                  namespace: gloo-system
                  observedGeneration: 1
              options:
                prefixRewrite: /api/pets
        matcher:
          sourcePrefixRanges:
          - addressPrefix: 1.2.3.4
            prefixLen: 32
    metadata: # collapsed for bevity
    name: listener-::-8080
    useProxyProto: false
  - bindAddress: '::'
    bindPort: 8443
    httpListener: {}
    metadata: # collapsed for bevity
status: # collapsed for bevity
```