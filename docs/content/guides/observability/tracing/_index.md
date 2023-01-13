---
title: Tracing
weight: 3
description: Sample request traces to monitor the traffic and health of your Gloo Edge environment.
---

Sample request traces to monitor the traffic and health of your Gloo Edge environment.

Tracing helps you obtain visibility and track requests as they pass through your API gateway to distributed backends, such as services, databases, or other endpoints in your ecosystem. This visability allows you to monitor and optimize the performance and latency of requests, and to perform root cause analyses to find bottlenecks and pinpoint failures.

To get started, choose one of the following options.
- [OpenTelemetry tracing]({{< versioned_link_path fromRoot="/guides/observability/tracing/otel/" >}}): Configure OpenTelemetry (OTel) as the trace span collector in your Gloo Edge installation. Then, use your preferred distributed tracing platform to collect and visualize the sampled spans.
- [Default Envoy tracing]({{< versioned_link_path fromRoot="/guides/observability/tracing/envoy/" >}}): Enable the default Envoy tracing capabilities in your Gloo Edge installation to trace requests. Then, use Zipkin, Jeager, or Datadog to collect and visualize the sampled spans.