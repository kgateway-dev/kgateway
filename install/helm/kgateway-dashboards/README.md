# Overview

This Helm chart can be used to deploy optional [Grafana](https://grafana.com/grafana/) dashboards,
which can be used for monitoring control-plane and data-plane metrics.

- Dashboards install as ConfigMaps, which can be detected automatically, if using kube-prometheus-stack.
- They are optional and are not required to use kgateway.
- The dashboards are intended only as a reference implementation for working with control-plane and data-plane metrics.
- Users are encouraged to extend or expand upon these dashboards based on their particular monitoring needs.

# Dashboards

- [Envoy](dashboards/envoy.json): A basic dashboard for monitoring data-plane metrics.
- [Kgateway Operations](dashboards/kgateway.json): A basic dashboard for monitoring control-plane metrics.
