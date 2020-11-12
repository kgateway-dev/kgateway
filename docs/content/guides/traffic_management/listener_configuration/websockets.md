---
title: Websockets
weight: 40
description: Learn how to configure Websockets support in Gloo Edge
---

Websockets is a protocol allowing full-duplex communication through a single TCP connection. Gloo Edge _enables websocket upgrades by default_ without any additional configuration changes. This document will show how to fine tune the websocket support as needed.

---

## Configuring Websockets on entire listener

Since websocket upgrades are enabled by default on an entire [Listener](https://www.envoyproxy.io/docs/envoy/latest/configuration/listeners/listeners), we can use the [HttpConnectionManager]({{% versioned_link_path fromRoot="/guides/traffic_management/listener_configuration/http_connection_manager/" %}}) configuration on the [Gateway]({{% versioned_link_path fromRoot="/introduction/architecture/concepts#gateways" %}}) to turn off websocket upgrades for all connections on that listener. 

For example, the default `gateway-proxy` can be configured like the following:

{{< highlight yaml "hl_lines=14-16" >}}
apiVersion: gateway.solo.io/v1
kind: Gateway
metadata:
  labels:
    app: gloo
  name: gateway-proxy
  namespace: gloo-system
spec:
  bindAddress: '::'
  bindPort: 8080
  httpGateway:
    options:
      httpConnectionManagerSettings:
        upgrades:
        - websocket:
            enabled: false
  proxyNames:
  - gateway-proxy
  useProxyProto: false
{{< /highlight >}}

Note the `httpGateway` configuration above with settings for the `httpConnectionManager`. 

We can also configure whether websocket is enabled per route as we see in the next section.

---

## Configuring websocket per route

If you need more fine-grained control configuring websockets, you can use the {{< protobuf name="gateway.solo.io.Route" display="Route">}} configuration in a {{< protobuf name="gateway.solo.io.VirtualService" display="VirtualService">}}. For example, if you have websockets disabled for your listener (as seen in previous section), you can enable it per route with the following:

{{< highlight yaml "hl_lines=8-10" >}}
- matchers:
    - prefix: /foo
   routeAction:
     single:
       upstream:
         name: foo
         namespace: gloo-system
    options:
      upgrades:
      - websocket: {}
{{< /highlight >}}

Please reach out to us on [Slack](https://slack.solo.io) or [File an Issue](https://github.com/solo-io/gloo/issues/new) if you're having trouble configuring websockets. 

---

## Next Steps

For background on Traffic Management, check out the [concepts page]({{% versioned_link_path fromRoot="/introduction/traffic_management/" %}}) dealing with the topic. You may also be interested in how [routing decisions]({{% versioned_link_path fromRoot="/guides/traffic_management/destination_selection/" %}}) are made and the possible [destinations for routing]({{% versioned_link_path fromRoot="/guides/traffic_management/destination_types/" %}}).
