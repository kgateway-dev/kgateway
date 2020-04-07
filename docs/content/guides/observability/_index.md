---
title: Observability
weight: 40
description: Monitoring, metrics, and logging for Gloo
---

Gloo exposes a lot of metrics from both the data plane (Envoy) as well as the control plane components. 
These metrics are available in Open Source Gloo, and can be scraped into a standard metrics backend like Prometheus. 

Enterprise Gloo comes with a few extra enhancements around observability. First, it installs by default with a prometheus 
server to scrape all of the Gloo metrics, and a Grafana instance to provide dashboards from those metrics. Alternatively, 
customers may wish to use their own Prometheus and Grafana deployments and integrate Gloo with those. 


Gloo Enterprise comes with a built-in grafana dashboard to show a high-level health of the Envoy proxy, 
and also comes with a component called `observability` that dynamically creates a new Grafana dashboard for each Gloo `Upstream`. 


{{% children description="true" %}}

Example dashboard that Gloo provides, showing a service going down briefly and then recovering (click to enlarge):
<img alt="Example Dashboards" src="./dashboard-example.png" style="border: dashed 2px;" />
