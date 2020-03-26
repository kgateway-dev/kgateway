---
title: Observability
weight: 40
description: Monitoring, metrics, and logging for Gloo
---

> **Note**: Observability features are available only in enterprise Gloo

As an API gateway built on the Envoy proxy, Gloo is well-equipped to use the wealth of metrics exported by Envoy to provide a detailed view into the health of your system, from both a high-level system view and a detailed look at each upstream.

By default, the enterprise deployment of Gloo ships with deployments of two leaders in the open source monitoring space, Prometheus and Grafana. See the links below for more info on how we use each one:

{{% children description="true" %}}

Example dashboard that Gloo provides, showing a service going down briefly and then recovering (click to enlarge):
<img alt="Example Dashboards" src="./dashboard-example.png" style="border: dashed 2px;" />
