---
title: Buffering
weight: 105
description: Set buffering limits on different Gloo Edge resources.
---

You can set buffering limits on different Gloo Edge resources to help fine-tune connection read and write speeds.

## Gateway listener

You can set buffer limits and other connection options with the [ListenerOptions]({{< versioned_link_path fromRoot="/reference/api/github.com/solo-io/gloo/projects/gloo/api/v1/options.proto.sk/#listeneroptions" >}}) settings for a [Gateway]({{< versioned_link_path fromRoot="/reference/api/github.com/solo-io/gloo/projects/gateway/api/v1/gateway.proto.sk/" >}}).

The listener options that you set on the Gateway apply to all routes that the Gateway serves.

```yaml
apiVersion: gateway.solo.io/v1
kind: Gateway
metadata: # collapsed for brevity
spec:
  bindAddress: '::'
  bindPort: 8080
  options:
    perConnectionBufferLimitBytes: 10485760
status: # collapsed for brevity
```

## Route

You can set buffer limits and other connection options with the [Buffer]({{< versioned_link_path fromRoot="/reference/api/github.com/solo-io/gloo/projects/gloo/api/external/envoy/extensions/filters/http/buffer/v3/buffer.proto.sk/" >}}) settings in the options of a [Route]({{< versioned_link_path fromRoot="/reference/api/github.com/solo-io/gloo/projects/gateway/api/v1/virtual_service.proto.sk/#route" >}}) in a [RouteTable]({{< versioned_link_path fromRoot="/reference/api/github.com/solo-io/gloo/projects/gateway/api/v1/route_table.proto.sk/" >}}).

```yaml
apiVersion: gateway.solo.io/v1
kind: RouteTable
metadata:
  name: 'petstore'
  namespace: 'default'
spec:
  routes:
    - matchers:
      - prefix: '/api/pets'
      routeAction:
        single:
          upstream:
            name: default-petstore-8080
            namespace: gloo-system
      options:
        prefixRewrite: '/'
        bufferPerRoute: 
          buffer: 
            maxRequestBytes: 10485760  
```

## Upstream

You can set buffer limits and other connection options with the [ConnectionConfig]({{< versioned_link_path fromRoot="/reference/api/github.com/solo-io/gloo/projects/gloo/api/v1/connection.proto.sk/" >}}) settings for an [Upstream]({{< versioned_link_path fromRoot="/reference/api/github.com/solo-io/gloo/projects/gloo/api/v1/upstream.proto.sk/" >}}).

```yaml
apiVersion: gloo.solo.io/v1
kind: Upstream
metadata: # collapsed for brevity
spec:
  connectionConfig:
    maxRequestsPerConnection: 0
    perConnectionBufferLimitBytes: 10485760
status: # collapsed for brevity
```