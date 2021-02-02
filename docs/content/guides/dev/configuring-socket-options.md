---
title: "Configuring Socket Options"
weight: 6
---

{{% notice note %}}
Available in Gloo Edge as of v1.7.0-beta11, v1.6.6 and v1.5.16.
{{% /notice %}}

{{% notice warning %}}
Socket options can have considerable effects. The configurations provided in this guide are not production proven, so please be careful!
{{% /notice %}}


## Configuring Keep-Alive For Downstream Connections to Envoy

One use case for this, is when an AWS NLB is deployed in front of Gloo Edge. This is a powerful combination that we recommend. However, AWS NLB's have an idle timeout of 350 seconds that [cannot be changed](https://docs.aws.amazon.com/elasticloadbalancing/latest/network/network-load-balancers.html#connection-idle-timeout). Therefore, we need to configure TCP keep alive, to keep the socket open during long idle periods.

{{% notice note %}}
Some users avoid this issue altogether by using a [kubernetes controller for elastic load balancers](https://github.com/kubernetes-sigs/aws-load-balancer-controller), instead of an AWS NLB
{{% /notice %}}

### Without Keep-Alive

Without using socket options to configure keep-alive, the connection between the Gloo Edge proxy and AWS NLB is silently closed after a period less than 350 seconds. The client then makes a request, and a reset packet (RST) is returned by the NLB. Since the client doesn't know how to handle the reset packet, it closes the socket.

### With Keep-Alive

With keep-alive configured, the Gloo Edge proxy will send a TCP_KEEPALIVE packet at a regular interval, ensuring that the socket remains open.

### Example Socket Options to Configure Keep-Alive

Here is an example set of socket options to configure keep alive:

```yaml
- description: "enable keep-alive" # socket level options
  level: 1 # means socket level options
  name: 9 # means the keep-alive parameter
  intValue: 1 # a nonzero value means "yes"
  state: STATE_LISTENING
- description: "idle time before first keep-alive probe is sent" # TCP protocol
  level: 6 # IPPROTO_TCP
  name: 4 # TCP_KEEPIDLE parameter - The time (in seconds) the connection needs to remain idle before TCP starts sending keepalive probes
  intValue: 90 # seconds
  state: STATE_LISTENING
- description: "keep-alive probes count" # TCP protocol
  level: 6 # IPPROTO_TCP
  name: 6 # the TCP_KEEPCNT parameter - The maximum number of keepalive probes TCP should send before dropping the connection
  intValue: 8 # number of failed probes
  state: STATE_LISTENING
```
