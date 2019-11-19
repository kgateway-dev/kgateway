---
title: Retries
weight: 50
description: Implement upstream retries when experiencing transient network errors
---

Specifies the retry policy for the route where you can say for a specific error condition how many times to retry and
for how long to try.

* `retryOn` : specifies the condition under which to retry the forward request to the upstream. Same as [Envoy x-envoy-retry-on](https://www.envoyproxy.io/docs/envoy/latest/configuration/http/http_filters/router_filter#x-envoy-retry-on).
* `numRetries` : (default: 1) optional attribute that specifies the allowed number of retries.
* `perTryTimeout` : optional attribute that specifies the timeout per retry attempt. Is of type [Google.Protobuf.WellKnownTypes.Duration](https://developers.google.com/protocol-buffers/docs/reference/csharp/class/google/protobuf/well-known-types/duration).

{{< highlight yaml "hl_lines=20-23" >}}
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
        retries:
          retryOn: 'connect-failure'
          numRetries: 3
          perTryTimeout: '5s'
{{< /highlight >}}
