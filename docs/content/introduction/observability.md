---
title: Observability
weight: 40
---

An API gateway is meant to be a central point of management for ingress traffic to a variety of destinations. It can also be a central point of observance, since it is uniquely qualified to know about all traffic traveling between clients and services. Gloo Edge is built on the Envoy proxy, which exposes a wealth of metrics providing a view into the health of your system as a whole and a detailed look at each Upstream.

---

## Grafana and Prometheus

The default installation of Gloo Edge Enterprise includes an instance of both [Prometheus](https://prometheus.io/docs/introduction/overview/) and [Grafana](https://grafana.com/), as well as the Gloo Edge Observability service.

{{% notice note %}}
The Gloo Edge Observability service is a Gloo Edge Enterprise feature.
{{% /notice %}}

Prometheus is an open-source systems monitoring and alerting toolkit. The Envoy proxy managed by Gloo Edge publishes metrics to on port 19000 and the Gloo Edge pods publish metrics on port 9091. You can run your own instance of Prometheus to harvest the metrics or use the instance of Prometheus created as part of the Gloo Edge Enterprise installation.

Grafana is an open source analytics and monitoring solution that allows you to query, visualize, alert on and understand metrics. Grafana can use Prometheus as a data source to generate its dashboards. You can run your own instance of Grafana and connect it to the instance of Prometheus that is harvesting metrics from Gloo Edge.

Gloo Edge Enterprise's deployment of Prometheus is configured to scrape metrics from all of the Gloo Edge pods including the Envoy proxy. The default Grafana deployment uses Prometheus as a data source to generate dashboards and visualizations. The Gloo Edge Observability service interacts with Grafana to create dynamically generated dashboards for the cluster and individual Upstreams.

While Gloo Edge Enterprise includes an installation of Prometheus and Grafana, it is possible to use your own existing instances of either application. Please reference the configuration guides for [Grafana]({{% versioned_link_path fromRoot="/guides/observability/grafana/" %}}) and [Prometheus]({{% versioned_link_path fromRoot="/guides/observability/prometheus/metrics/" %}}) for more information.

---

## Tracing

Tracking the life of a request as it passes through the API gateway and to other services can be challenging. You want to understand how a flow traversed your system, where there is latency, and how the request was processed. Envoy has [built-in tracing capabilities](https://www.envoyproxy.io/docs/envoy/latest/intro/arch_overview/observability/tracing.html) to enable system wide tracing using request ID generation, client trace ID joining, and external trace service integration. Gloo Edge makes it simple to enable and configure tracing in your environment.

Envoy will send its tracing information to an external trace service, such as [Zipkin](https://zipkin.io/) or [Lightstep](https://lightstep.com/). The tracing service provider settings for Envoy can be set during installation by editing the Helm chart values, or post installation by updating the ConfigMap that holds the Envoy configuration.

Once a tracing service provider has been configured, tracing can be enabled on a per-listener basis in Gloo Edge. To assist in identifying the path of a flow, a tracing annotation can be added by each route in a Virtual Service.

Please refer to the [tracing guide]({{% versioned_link_path fromRoot="/guides/observability/tracing/" %}}) for more information on setup and configuration.

---

## Stats and Admin Ports

### Envoy Admin

The admin port for Envoy is set to `19000` by Gloo Edge. Through the admin port you can view the metrics for Envoy as well as a large number of other features. You can find more information about the Envoy admin port in the [Envoy docs](https://www.envoyproxy.io/docs/envoy/v1.7.0/operations/admin). Gloo Edge configures port `8081` on the Envoy proxy for metric scraping by Prometheus. If you plan to use your own instance of Prometheus, you will be connecting to port `8081` for metrics collection.

### Gloo Edge Admin

The admin port for all of the Gloo Edge pods is `9091`. If the `START_STATS_SERVER` environment variable is set to `true` in Gloo Edge's pods, they will listen on port `9091`. Functionality available on that port includes Prometheus metrics at `/metrics` (see more on Gloo Edge metrics [here]({{% versioned_link_path fromRoot="/guides/observability/prometheus/metrics/" %}})), as well as admin functionality like changing the logging levels and getting a stack dump.

---

## Next Steps

Now that you have an understanding of how Gloo Edge supports observability we have a few suggested paths:

* **[Security]({{% versioned_link_path fromRoot="/introduction/security/" %}})** - learn more about Gloo Edge and its security features
* **[Setup]({{% versioned_link_path fromRoot="/installation/" %}})** - Deploy your own instance of Gloo Edge
* **[Observability guides]({{% versioned_link_path fromRoot="/guides/observability/" %}})** - Set up Grafana and Prometheus or configure tracing