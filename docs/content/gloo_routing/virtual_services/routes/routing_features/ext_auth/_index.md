---
title: Authentication (Enterprise)
weight: 56
description: Route-specific external authorization configuration.
---

As desired, authorization can be disabled on a route-by-route basis by setting `routePlugins.extensions.configs.extauth.disable: true` on the target route, as shown below.

{{< highlight yaml "hl_lines=21-25" >}}
apiVersion: gateway.solo.io/v1
kind: VirtualService
metadata:
  name: default
  namespace: gloo-system
spec:
  virtualHost:
    domains:
    - '*'
    routes:
    - matcher:
        prefix: /auth-disabled-for-route
        methods:
        - POST
      routeAction:
        single:
          upstream:
            name: default-echo-server-8080
            namespace: gloo-system
      routePlugins:
        extensions:
          configs:
            extauth:
              disable: true
    - matcher:
        prefix: /auth-enabled-for-route
        methods:
        - GET
      routeAction:
        single:
          upstream:
            name: default-echo-server-8080
            namespace: gloo-system
    virtualHostPlugins:
      extensions:
        configs:
          extauth:
            customAuth: {}
{{< /highlight >}}
