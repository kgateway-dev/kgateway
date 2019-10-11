---
title: Dashboards
weight: 1
description: Documentation about the dynamically generated dashboards created by Gloo
---

> **Note**: Observability features are available only in enterprise Gloo

1. [Whole-Cluster Dashboard](#whole-cluster-dashboard)
1. [Dynamically Generated Dashboards](#dynamically-generated-dashboards)

## Whole-Cluster Dashboard
A dashboard showing whole-cluster metrics can be found in the `gloo/Envoy Statistics` dashboard. In that dashboard you can find panels showing:

* Requests per Second
* Percent Response Code per Second
* Response Codes per Second
* Upstream Request Time Percentiles
* Round Trip Time Percentiles
* Average Request Time


## Dynamically Generated Dashboards
Gloo generates a dashboard per watched upstream. These dashboards are updated and recreated every time an upstream changes. It renders a Go template that can be found in a configmap, which gets loaded into the `observability` pod's env at startup:

```bash
~ > kubectl -n gloo-system get cm glooe-observability-config -o yaml | head -n 10
apiVersion: v1
data:
  DASHBOARD_JSON_TEMPLATE: |
    {
      "annotations": {
        "list": [
          {
            "builtIn": 1,
            "datasource": "-- Grafana --",
```
If you want to customize how these per-upstream dashboards look, you can provide your own template to use by writing a Grafana dashboard JSON representation to that config map key. Currently the only available variables that are available when the Go template is rendered are:

* `.NameTemplate`
* `.Uid`
* `.EnvoyClusterName`
