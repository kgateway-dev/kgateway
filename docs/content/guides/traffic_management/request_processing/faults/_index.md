---
title: Faults
weight: 60
description: Inject faults into your services for resilience and chaos testing
---

This can be used for testing the resilience of your services by intentionally injecting faults (errors and delays) into
a percentage of your requests.

Abort specifies the percentage of request to error out.

* `percentage` : (default: 0) float value between 0.0 - 100.0
* `httpStatus` : (default: 0) int value for HTTP Status to return, e.g., 503

Delay specifies the percentage of requests to delay.

* `percentage` : (default: 0) float value between 0.0 - 100.0
* `fixedDelay` : (default: 0) [Duration](https://developers.google.com/protocol-buffers/docs/reference/csharp/class/google/protobuf/well-known-types/duration)
value for how long to delay selected requests

{{< highlight yaml "hl_lines=20-26" >}}
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
        faults:
          abort:
            percentage: 2.5
            httpStatus: 503
          delay:
            percentage: 5.3
            fixedDelay: '5s'
{{< /highlight >}}