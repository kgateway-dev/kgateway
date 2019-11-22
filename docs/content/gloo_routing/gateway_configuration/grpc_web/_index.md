---
title: gRPC Web
weight: 3
---

In order to serve gRPC Web clients, the server must first transcode the message into a format that the web client can understand [details](https://github.com/grpc/grpc/blob/master/doc/PROTOCOL-WEB.md#protocol-differences-vs-grpc-over-http2). Gloo configures Envoy to do this by default. If you would like to disable this behavior, you can do so with:

{{< highlight yaml "hl_lines=7-9" >}}
apiVersion: gateway.solo.io/v1
kind: Gateway
metadata: # collapsed for brevity
spec:
  bindAddress: '::'
  bindPort: 8080
  options:
    grpcWeb:
      disable: true
  useProxyProto: false
status: # collapsed for brevity
{{< /highlight >}}