---
title: Dashboards
weight: 1
description: Documentation about the dynamically generated dashboards created by the observability component in Gloo Edge
---

1. [Whole-Cluster Dashboard](#whole-cluster-dashboard)
1. [Dynamically Generated Dashboards](#dynamically-generated-dashboards)

## Whole-Cluster Dashboard
> **Note**: This dashboard is packaged by default with Gloo Edge Enterprise

A dashboard showing whole-cluster metrics can be found in the `gloo/Envoy Statistics` dashboard. In that dashboard you can find panels showing:

* Requests per Second
* Percent Response Code per Second
* Response Codes per Second
* Upstream Request Time Percentiles
* Round Trip Time Percentiles
* Average Request Time


## Dynamically Generated Dashboards
Gloo Edge Enterprise's observability component generates a dashboard per watched upstream. These dashboards are updated and recreated every time an upstream changes. It renders a Go template that can be found in a configmap, which gets loaded into the `observability` pod's env at startup:

```bash
~ > kubectl -n gloo-system get cm gloo-observability-config -o yaml | head -n 10
apiVersion: v1
data:
  GRAFANA_URL: http://glooe-grafana.gloo-system.svc.cluster.local:80
  UPSTREAM_DASHBOARD_JSON_TEMPLATE: |2

    {
      "annotations": {
        "list": [
          {
            "builtIn": 1,
```
If you want to customize how these per-upstream dashboards look, you can provide your own template to use by writing a Grafana dashboard JSON representation to that config map key. Currently the only available variables that are available when the Go template is rendered are:

* `.NameTemplate`
* `.Uid`
* `.EnvoyClusterName`
