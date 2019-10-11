---
title: Extensions
weight: 60
description: Support for Gloo extensions and additional configuration
---

Extensions are arbitrary key-value pairs that can be stored on
Gloo routes for the purposes of extending Gloo. This is used to support external plugins, as well as Gloo-Enterprise plugins.

{{< highlight yaml "hl_lines=19-21" >}}
apiVersion: gateway.solo.io/v1
kind: VirtualService
metadata:
  name: 'default'
  namespace: 'gloo-system'
spec:
  virtualHost:
    domains:
    - '*'
    routes:
    - matcher:
        prefix: '/petstore'
      routeAction:
        single:
          upstream:
            name: 'default-petstore-8080'
            namespace: 'gloo-system'
      routePlugins:
        extensions:
          configs:
            my-custom-key: my-custom-value
{{< /highlight >}}