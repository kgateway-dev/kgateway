---
title: Prometheus
weight: 2
description: Info about Gloo Edge's Prometheus Instance
---
{{% notice note %}}
Observability features are available only in Gloo Edge Enterprise
{{% /notice %}}
This functionality is turned on by default, and can be turned off a couple of different ways: through [Helm chart install
options]({{< versioned_link_path fromRoot="/installation/gateway/kubernetes/#installing-the-gloo-gateway-on-kubernetes" >}}); and through environment variables.

## Default Installation
{{% notice warning %}}
Gloo is shipped by default with prometheus 11.x charts, and provides a set of default values that are not suitable for production usage in most cases. Please provide your own instance of prometheus or configure the helm chart options with production values
{{% /notice %}}

{{% notice note %}}
For a complete set of options, please refer to: https://github.com/prometheus-community/helm-charts/blob/main/charts/prometheus/values.yaml, or run `helm show values prometheus-community/prometheus`
{{% /notice %}}

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
