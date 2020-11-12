---
title: gRPC Web
weight: 20
description: Enable or disable the transcoding of messages to support gRPC web clients
---

gRPC-Web is a Javascript library that lets browser clients using HTTP-1 or HTTP-2 access a gRPC service. Envoy supports the gRPC-Web protocol out of the box, functioning as a proxy between web clients and the gRPC service. In this guide you'll see how to enable or disable the gRPC-Web protocol on Envoy through Gloo Edge's gateway Custom Resource.

---

## Configuring Gateway gRPC Options

In order to serve gRPC-Web clients, the server must first transcode the message into a format that the web client can understand [details](https://github.com/grpc/grpc/blob/master/doc/PROTOCOL-WEB.md#protocol-differences-vs-grpc-over-http2). Gloo Edge configures Envoy to do this by default. If you would like to disable this behavior, you can do so by editing the gateway Custom Resource for the Gateway you would like to alter.

The relevant section of the gateway CR is highlighted below.

{{< highlight yaml "hl_lines=7-9" >}}
apiVersion: gateway.solo.io/v1
kind: Gateway
metadata: # collapsed for brevity
spec:
  bindAddress: '::'
  bindPort: 8080
  httpGateway:
    options:
      grpcWeb:
        disable: true
  useProxyProto: false
status: # collapsed for brevity
{{< /highlight >}}

---

## Next Steps

For more information on how to edit the gateway CR, take a look at the [listener configuration guide]({{< versioned_link_path fromRoot="/guides/traffic_management/listener_configuration/" >}}). You may also want to take a look at the Envoy docs regarding the [use of gRPC](https://www.envoyproxy.io/docs/envoy/v1.9.0/intro/arch_overview/grpc#arch-overview-grpc) with Envoy.