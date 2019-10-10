---
title: Gloo Metrics
weight: 40
description: Configuring Gloo to ship telemetry/metrics to Prometheus
---


# Metrics
All Gloo pods ship with optional [Prometheus](https://prometheus.io/) monitoring capabilities.

This functionality is turned off by default, and can be turned on a couple of different ways: through [Helm chart install
options]({{< versioned_link_path fromRoot="/installation/gateway/kubernetes/#installing-the-gloo-gateway-on-kubernetes" >}}); and through environment variables.

### Helm Chart Options

The first way is via the helm chart. All deployment objects in the helm templates accept an argument `stats` which
when set to true, start a stats server on the given pod.

For example, to add stats to the Gloo `gateway`, when installing with Helm add  `--set discovery.deployment.stats=true`.

```shell
helm install gloo/gloo \
  --name gloo \
  --namespace gloo-system \
  --set discovery.deployment.stats=true
```

Here's what the resulting `discovery` manifest would look like. Note the additions of the `prometheus.io` annotations,
and the `START_STATS_SERVER` environment variable.

{{< highlight yaml "hl_lines=18-21 32-33" >}}
apiVersion: extensions/v1beta1
kind: Deployment
metadata:
  labels:
    app: gloo
    gloo: discovery
  name: discovery
  namespace: gloo-system
spec:
  replicas: 1
  selector:
    matchLabels:
      gloo: discovery
  template:
    metadata:
      labels:
        gloo: discovery
      annotations:
        prometheus.io/path: /metrics
        prometheus.io/port: "9091"
        prometheus.io/scrape: "true"
    spec:
      containers:
      - image: "quay.io/solo-io/discovery:0.11.1"
        imagePullPolicy: Always
        name: discovery
        env:
          - name: POD_NAMESPACE
            valueFrom:
              fieldRef:
                fieldPath: metadata.namespace
          - name: START_STATS_SERVER
            value: "true"
{{< /highlight >}}

This flag will set the `START_STATS_SERVER` environment variable to true in the container which will start the stats
server on port `9091`.

### Environment Variables

The other method is to manually set the `START_STATS_SERVER=1` in the pod.

## Monitoring Gloo with Prometheus

Prometheus has great support for monitoring kubernetes pods. Docs for that can be found
[here](https://prometheus.io/docs/prometheus/latest/configuration/configuration/#kubernetes_sd_config). If the stats
are enabled through the Helm chart than the Prometheus annotations are automatically added to the pod spec. And those
Prometheus stats are available from the admin page in our pods.

For example, assuming you installed Gloo as previously using Helm, and enabled stats for discovery, you
could then `kubectl port-forward <pod> 9091:9091` those pods (or deployments/services selecting those pods) to access
their admin page as follows.

```shell
kubectl --namespace gloo-system port-forward deployment/discovery 9091:9091
```

And then open <http://localhost:9091> for the admin page, including the Prometheus metrics at <http://localhost:9091/metrics>.

More information on Gloo's admin ports can be found [here](../ports).
