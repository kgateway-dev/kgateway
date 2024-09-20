---
title: TCP Keepalive
weight: 105
description: Enabling TCP Keepalive on Downstream and Upstream Connections
---

You can enable TCP keepalive on Downstream Connections or Upstream Connections. By default, TCP Keepalive is disabled.

## TCP Keepalive on Downstream Connections

{{% notice note %}}
Available in Gloo Gateway as of v1.7.0-beta11, v1.6.6 and v1.5.16.
{{% /notice %}}

The client for the downstream connections can vary depending on your setup. If the gateway is
directly exposed to the public internet, the client would be the direct external end users. However, in a
typical production setup, there is usually a form of load balancer between the end users and the gateway.
Depending on the type of load balancer, TCP keepalive might be needed to keep a long-lived connection open
and functional.

To enable TCP keepalive on downstream connections, the following settings can be set on the listener options
in the [Gateway]({{< versioned_link_path fromRoot="/reference/api/github.com/solo-io/gloo/projects/gateway/api/v1/gateway.proto.sk/" >}})
or [Proxy]({{< versioned_link_path fromRoot="/reference/api/github.com/solo-io/gloo/projects/gloo/api/v1/proxy.proto.sk/" >}})
resources.

The following example will enable TCP keepalive probe between the client and the gateway and send out the first
probe when the connection has been idled for 60 seconds (TCP_KEEPIDLE). Once triggered, each probe will be
sent every 20 seconds (TCP_KEEPINTVL). If there is no response for 2 consecutive probes (TCP_KEEPCNT), the
connection will be dropped.

```yaml
apiVersion: gateway.solo.io/v1
kind: Gateway
metadata: # collapsed for brevity
spec:
  bindAddress: '::'
  bindPort: 8080
  options:
    socketOptions:
      - description: "enable keep-alive" # socket level options
        level: 1 # means socket level options (SOL_SOCKET)
        name: 9 # means the keep-alive parameter (SO_KEEPALIVE)
        intValue: 1 # a nonzero value means "yes"
        state: STATE_PREBIND
      - description: "idle time before first keep-alive probe is sent" # TCP protocol
        level: 6 # IPPROTO_TCP
        name: 4 # TCP_KEEPIDLE parameter - The time (in seconds) the connection needs to remain idle before TCP starts sending keepalive probes
        intValue: 60 # seconds
        state: STATE_PREBIND
      - description: "keep-alive interval" # TCP protocol
        level: 6 # IPPROTO_TCP
        name: 5 # the TCP_KEEPINTVL parameter - The time (in seconds) between individual keepalive probes.
        intValue: 20 # seconds
        state: STATE_PREBIND
      - description: "keep-alive probes count" # TCP protocol
        level: 6 # IPPROTO_TCP
        name: 6 # the TCP_KEEPCNT parameter - The maximum number of keepalive probes TCP should send before dropping the connection
        intValue: 2 # number of failed probes
        state: STATE_PREBIND
```

{{% notice warning %}}
Socket options can have considerable effects. The configurations provided in this guide are not production proven, so please be careful! These socket options only get applied when the listener starts up, so it would
not have effects without restarting the gateway.
{{% /notice %}}

## TCP Keepalive on Upstream Connections

Upstream connections are the connections between the gateway and the destination which can be other K8s
service within or outside the cluster, OTEL collectors, tap servers or other cloud services and endpoints.

You can set upstream TCP keepalive with the
[ConnectionConfig]({{< versioned_link_path fromRoot="/reference/api/github.com/solo-io/gloo/projects/gloo/api/v1/connection.proto.sk/" >}})
settings in the
[Upstream]({{< versioned_link_path fromRoot="/reference/api/github.com/solo-io/gloo/projects/gloo/api/v1/upstream.proto.sk/" >}}) resource.

The following example will enable TCP keepalive probe between the gateway and the upstream connection and send out the first probe when the connection has been idled for 60 seconds (keepaliveTime). Once triggered,
a TCP keepalive probe will be sent out every 20 seconds (keepaliveInterval). If there is no response for 2 consecutive probes (keepaliveProbes), the connection will be dropped.

```yaml
apiVersion: gloo.solo.io/v1
kind: Upstream
metadata: # collapsed for brevity
spec:
  connectionConfig:
    tcpKeepalive:
      keepaliveInterval: 20
      keepaliveProbes: 2
      keepaliveTime: 60
```
