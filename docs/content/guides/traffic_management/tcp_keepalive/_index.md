---
title: TCP Keepalive
weight: 105
description: Enabling TCP Keepalive on Downstream and Upstream Connections
---

TCP Keepalive serves two main purposes:

1) the obvious one is to keep the connection alive by sending out probe after the connection has been idled for specific time
2) the less obvious one is to detect stale connections when the probe fails and drop the connection.

There are 3 settings in TCP keepalive:

- tcp_keepalive_intvl
- tcp_keepalive_probes
- tcp_keepalive_time

The explanation can be found in the [Linux tcp manpage](https://man7.org/linux/man-pages/man7/tcp.7.html).

Here are some considerations when determining the proper values for you environment:

1) On a slow or lossy network, if tcp_keepalive_intvl is set too low, it can inadvertently drop the connections more often
then it should.
2) Many application layer protocol like HTTP, GRPC(using HTTP2/2) has it's own keepalive mechanism that change what you
expected from TCP keepalive. For example, the application can still close the connection after it's keep-alive timeout even
TCP keepalive is in place because TCP keepalive probe does not get up to the application layer.  

## Potential issues that can be mitigated by turning on TCP keepalive {#potential_issues}

### Stale Connections

Because tcp connection close is a 4-way handshake, it is possible to have stale connection where one side has been gone
but the other side is not aware if the network is unstable. If the unaware side is just listening for events, it might think
that there is just no event but in reality has been missing all the event.

An example of this could be Gloo Gateway Control Plane has closed the connection but envoy is not aware and never get any xds
configuration update. We have enabled tcp keepalive between envoy and Gloo Gateway control plane but if that needs to be
changed, see [TCP Keepalive on Static Clusters]({{<ref "#tcp-keepalive-on-static-clusters">}}) section below.

### Network Load Balancer connection tracking

If the gateway proxy (envoy) is directly exposed to the public internet, the client would be the direct external end users.
However, in a typical production setup, there is usually a form of load balancer between the end users and the gateway proxy.

Some Network Load Balancer (NLB) use connection tracking to remember where a packet to get forwarded to once a connection is established.
If the connection has been idling, The NLB might stop tracking the connection. In this scenario, both sides still think the connection
is open but when the client send a packet through the NLB, the NLB now would not know where to send the packet and will send a RESET
to the client. If the client does not automatically retry, this might show up as an error or you will see a lot of RESET from the tcp stats
thinking it's a network issue. Enabling TCP keep alive will address this issue and help keep long-lived connection open and functional.

## TCP Keepalive on Downstream Connections {#downstream}

{{% notice note %}}
Available in Gloo Gateway as of v1.7.0-beta11, v1.6.6 and v1.5.16.
{{% /notice %}}

{{% notice warning %}}
Currently envoy does not directly support turning on TCP keepalive on downstream connections. It can only be done with generic socket options
setting. Socket options can have considerable effects and may not be portable on all platforms. The configurations provided in this guide are
not production proven, so please be careful! Because these options are applied to the Listener, they would affect all downstream connections
if they are mis-configured.
{{% /notice %}}

The client for the downstream connections can vary depending on your setup. It can be the actual end user or a Load Balancer (layer 4 or layer 7)

To enable TCP keepalive on downstream connections, the following settings can be set on the listener options
in the [Gateway]({{< versioned_link_path fromRoot="/reference/api/github.com/solo-io/gloo/projects/gateway/api/v1/gateway.proto.sk/" >}})
or [Proxy]({{< versioned_link_path fromRoot="/reference/api/github.com/solo-io/gloo/projects/gloo/api/v1/proxy.proto.sk/" >}})
resources.

The following example will enable TCP keepalive probe between the client and the gateway proxy and send out the first
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

## TCP Keepalive on Upstream Connections {#upstream}

Upstream connections are the connections between the gateway proxy (envoy) and the destination or backend services which can be other K8s
service within or outside the cluster, OTEL collectors, tap servers or other cloud services and endpoints.

You can set upstream TCP keepalive with the
[ConnectionConfig]({{< versioned_link_path fromRoot="/reference/api/github.com/solo-io/gloo/projects/gloo/api/v1/connection.proto.sk/" >}})
settings in the
[Upstream]({{< versioned_link_path fromRoot="/reference/api/github.com/solo-io/gloo/projects/gloo/api/v1/upstream.proto.sk/" >}}) resource.

The following example will enable TCP keepalive probe between the gateway proxy and the upstream connection and send out the first probe when the connection has been idled for 60 seconds (keepaliveTime). Once triggered,
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

## TCP Keepalive on Static Clusters

{{% notice note %}}
Available in Gloo Gateway Enterprise version only
{{% /notice %}}

For static upstream clusters setup through helm templates, the `gloo.gatewayProxies.NAME.tcpKeepaliveTimeSeconds`
setting can be used to change the keepalive timeout value (default is 60s). See
[Enterprise Gloo Gateway]({{ < versioned_link_path fromRoot="/reference/helm_chart_values/enterprise_helm_chart_values/" >}}) helm chare values for reference.

The tcp_keepalive_intvl and tcp_keepalive_probes cannot be changed and the default value for your system will be used. To check the default values for your system:

```bash
# sysctl -a | grep keepalive
net.ipv4.tcp_keepalive_intvl = 75
net.ipv4.tcp_keepalive_probes = 9
net.ipv4.tcp_keepalive_time = 7200
```
