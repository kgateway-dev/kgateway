---
title: External processing
weight: 55
description: 
---

Use the [Envoy external processing (ExtProc) filter](https://www.envoyproxy.io/docs/envoy/latest/configuration/http/http_filters/ext_proc_filter) to connect an external gRPC processing server to the Envoy filter chain. The external service can manipulate headers, body, and trailers of a request or response before it is forwarded to an upstream or downstream service. The request or response can also be terminated at any given time.

{{% children description="true"%}}

