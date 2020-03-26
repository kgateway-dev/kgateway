---
title: Timeouts
weight: 90
description: Implement upstream timeouts 
---

The maximum [Duration](https://developers.google.com/protocol-buffers/docs/reference/csharp/class/google/protobuf/well-known-types/duration)
to try to handle the request, inclusive of error retries.

{{< highlight yaml "hl_lines=19" >}}
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
    - matchers:
       - prefix: '/petstore'
      routeAction:
        single:
          upstream:
            name: 'default-petstore-8080'
            namespace: 'gloo-system'
      options:
        timeout: '20s'
        retries:
          retryOn: 'connect-failure'
          numRetries: 3
          perTryTimeout: '5s'
{{< /highlight >}}