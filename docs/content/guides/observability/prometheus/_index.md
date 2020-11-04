---
title: Prometheus
weight: 2
description: Info about Gloo Edge's Prometheus Instance
---

> **Note**: Observability features are available only in Gloo Edge Enterprise

## Run Your Own Prometheus
A common setup may be to run your own prometheus, separate from the gloo-provided one. The Gloo Edge Enterprise UI makes use of its own grafana to display dashboards for Envoy and Kubernetes, leveraging gloo custom resources such as `Upstreams`. You can point gloo's system grafana toward your prometheus by overriding grafana's datasources tag, i.e.

```yaml
grafana:
  datasources:
    datasources.yaml:
      apiVersion: 1
      datasources:
        - name: gloo
          type: prometheus
          access: proxy
          url: http://{{ your.prometheus }}:{{ your.port }}  # fill this in!
          isDefault: true
```

## Find Envoy's Prometheus Stats
The envoy pod publishes its fairly comprehensive metrics on port 19000. You can view the available ones by running:
```bash
# Port-forward to envoy's admin port:
kubectl port-forward deployment/gateway-proxy 19000

curl http://localhost:19000/stats/prometheus
```

You can use these to customize the dashboards that get created for every upstream, as described [here]({{% versioned_link_path fromRoot="/guides/observability/grafana/dashboards/#dynamically-generated-dashboards" %}}).

{{% children description="true" %}}
