---
title: TCP keepalive
weight: 105
description: Enable TCP keepalive on downstream and upstream connections
---

You can enable TCP keepalive on downstream and upstream connections, including static clusters.

## About TCP keepalive {#about}

TCP keepalive helps you provide stable connections in two main ways:

1) Keep the connection alive by sending out probes after the connection has been idle for a specific amount of time. This way, the connection does not have to be reestablished repeatedly, which could otherwise lead to latency spikes.
2) Detect and close stale connections when the probe fails, such as due to firewall or other network security settings. This way, you can avoid longer timeouts and retries on broken connections.

### Settings to configure TCP keepalive {#settings}

To configure TCP keepalive, you can use three settings. For more information, see the [Linux TCP manual page](https://man7.org/linux/man-pages/man7/tcp.7.html).

| Setting | Default value | Description |
| --- | --- | --- |
| tcp_keepalive_intvl | 75 | The number of seconds between probes. |
| tcp_keepalive_probes | 9 | The number of probes to send before closing the connection. |
| tcp_keepalive_time | 7200 | The number of seconds that a connection stays idle before probes are sent. |

### Guidance for settings {#guidance}

To help determine the right settings for your network environment, consider your app traffic patterns and the desired user experience. You might also find the following guidelines helpful.

* On a slow or lossy network, if `tcp_keepalive_intvl` or `tcp_keepalive_probes` values are set too low, you might generate unnecessary traffic or inadvertently drop the connections more often than needed.
* If you set the probe intervals or keepalive time durations too high, you might not detect broken connections soon enough.
* Many application layer protocols like HTTP and gRPC (using HTTP/2) have their own keepalive mechanisms that can change what you otherwise expect from the TCP keepalive. For example, the application might still close the connection after the keepalive timeout, even if TCP keepalive is in place. This unexpected closure happens because the TCP keepalive probe does not get up to the application layer.  

### Use cases for TCP keepalive {#use-cases}

**Stale connections**

Because closing a TCP connection involves a four-way handshake, you might notice stale connections arise in an unstable network, where one side has been responding but the other side has not detected a response yet. Both sides might listen for events and consider the other nonresponsive, even though one side just missed the events.

An example of this situation can occur between the Gloo Gateway control plane and the Envoy gateway proxy. Envoy might listen for xDS configuration updates but not receive any. Because Envoy thinks that the connection is still alive, it does not try to reconnect to the Gloo Gateway control plane. Even though Gloo Gateway has a default TCP keepalive setting, you might find that you need to update the values. For an example, see [TCP keepalive on static clusters]({{<ref "#tcp-keepalive-on-static-clusters">}}).

**Load balancer connection tracking**

Similar to the previous example with the Gloo Gateway control plane, the Envoy gateway proxy might miss connections with an external client. This situation happens when the Envoy gateway proxy is directly exposed on the public internet so that external end users can access your services.

In many production setups, you set up a load balancer between the end users and the gateway proxy, such as an [AWS application load balancer (ALB)](https://docs.aws.amazon.com/elasticloadbalancing/latest/application/introduction.html).

Some network load balancers (NLBs) use connection tracking to remember where to forward a packet to after the connection is established. If the connection is idle, the NLB might remove the connection from its connection tracking table. In this scenario, both sides still think that the connection is open. When the client sends a packet through the NLB, the NLB no longer knows where to forward the packet to and sends a `RESET` response to the client.

If the client does not automatically retry, this might show up as an error. You might see many `RESET` messages in the TCP stats and think that you have a network issue. By enabling TCP keepalive or adjusting its settings, you can help keep long-lived connections open and functional. For an example, see [TCP keepalive on downstream connections]({{<ref "#tcp-keepalive-on-downstream-connections">}}).

## TCP keepalive on downstream connections {#downstream}

{{% notice warning %}}
Currently Envoy does not support turning on TCP keepalive settings directly on downstream connections. Instead, you can use the generic socket options settings. Socket options can have considerable effects on performance, and might not be portable across platforms. As such, the configuration example in this guide is not suitable for every production environment. Also note that because these options are applied to the gateway listener, they affect all downstream connections. Be careful and adjust the settings for your specific environment.
{{% /notice %}}

Downstream connections are between the Envoy gateway proxy and the client that sends the request to your services. The client varies depending on your setup:

* The end user that sends a request.
* The Layer 4 or Layer 7 load balancer that forwards the request from the end user to the gateway proxy.

To enable TCP keepalive on downstream connections, configure the following settings in the listener options on the [Gateway]({{< versioned_link_path fromRoot="/reference/api/github.com/solo-io/gloo/projects/gateway/api/v1/gateway.proto.sk/" >}}) resource.

The following example enables TCP keepalive probes between the client and the gateway proxy. The first probe is sent when the connection has been idle for 60 seconds (`TCP_KEEPIDLE`). After, each probe is sent every 20 seconds (`TCP_KEEPINTVL`). After no response for 2 consecutive probes (`TCP_KEEPCNT`), the connection is dropped.

```yaml
apiVersion: gateway.solo.io/v1
kind: Gateway
metadata: # collapsed for brevity
spec:
  bindAddress: '::'
  bindPort: 8080
  options:
    socketOptions:
      - description: "enable keepalive" # socket level options
        level: 1 # means socket level options (SOL_SOCKET)
        name: 9 # means the keepalive parameter (SO_KEEPALIVE)
        intValue: 1 # a nonzero value means "yes" to enable
        state: STATE_PREBIND
      - description: "idle time before first keepalive probe is sent" # TCP protocol
        level: 6 # IPPROTO_TCP
        name: 4 # TCP_KEEPIDLE parameter - The time in seconds that the connection is idle before TCP starts sending keepalive probes.
        intValue: 60 # seconds
        state: STATE_PREBIND
      - description: "keepalive interval" # TCP protocol
        level: 6 # IPPROTO_TCP
        name: 5 # the TCP_KEEPINTVL parameter - The time in seconds between individual keepalive probes.
        intValue: 20 # seconds
        state: STATE_PREBIND
      - description: "keepalive probes count" # TCP protocol
        level: 6 # IPPROTO_TCP
        name: 6 # the TCP_KEEPCNT parameter - The maximum number of keepalive probes that TCP sends before dropping the connection.
        intValue: 2 # number of failed probes
        state: STATE_PREBIND
```

## TCP keepalive on upstream connections {#upstream}

Upstream connections are between the Envoy gateway proxy and your services, such as Gloo upstreams, Kubernetes services, external services, OTel collectors, TAP servers, cloud functions, or other destinations.

To enable TCP keepalive on upstream connections, configure the following [ConnectionConfig]({{< versioned_link_path fromRoot="/reference/api/github.com/solo-io/gloo/projects/gloo/api/v1/connection.proto.sk/" >}}) settings in the
[Upstream]({{< versioned_link_path fromRoot="/reference/api/github.com/solo-io/gloo/projects/gloo/api/v1/upstream.proto.sk/" >}}) resource.

The following example enables TCP keepalive probes between the gateway proxy and the upstream connection. The first probe is sent when the connection has been idle for 60 seconds (`keepaliveTime`). After, a TCP keepalive probe is sent every 20 seconds (`keepaliveInterval`). After no response for 2 consecutive probes (`keepaliveProbes`), the connection is dropped.

```yaml
apiVersion: gloo.solo.io/v1
kind: Upstream
metadata: # collapsed for brevity
spec:
# the destination settings are omitted for brevity
  connectionConfig:
    tcpKeepalive:
      keepaliveInterval: 20 # seconds
      keepaliveProbes: 2 # number of probes
      keepaliveTime: 60 # seconds
```

## TCP keepalive on static clusters {#static-clusters}

{{% notice note %}}
{{< readfile file="static/content/enterprise_only_feature_disclaimer" markdown="true">}}
{{% /notice %}}

If you used the `gloo.gatewayProxies.NAME.envoyStaticClusters[].NAME` Helm value to set up a static upstream cluster, you can change the TCP keepalive timeout value (default is 60s) with the `gloo.gatewayProxies.NAME.tcpKeepaliveTimeSeconds` Helm setting. 
setting to change the keepalive timeout value (default is 60s). For an overview of available Helm values, see the
[Enterprise Gloo Gateway]({{< versioned_link_path fromRoot="/reference/helm_chart_values/enterprise_helm_chart_values/" >}}) reference docs.

Note that you cannot change the `tcp_keepalive_intvl` and `tcp_keepalive_probes` values. The default system values are used instead. 

To check the default values for your system:

```bash
# sysctl -a | grep keepalive
net.ipv4.tcp_keepalive_intvl = 75
net.ipv4.tcp_keepalive_probes = 9
net.ipv4.tcp_keepalive_time = 7200
```
