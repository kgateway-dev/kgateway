---
title: Gloo Edge Metrics
weight: 40
description: Configuring Gloo Edge to ship telemetry/metrics to Prometheus
---



All Gloo Edge pods ship with optional [Prometheus](https://prometheus.io/) monitoring capabilities.

This functionality is turned on by default, and can be turned off a couple of different ways: through [Helm chart install
options]({{< versioned_link_path fromRoot="/installation/gateway/kubernetes/#installing-the-gloo-gateway-on-kubernetes" >}}); and through environment variables.

You can take a look at the [Help strings](#metrics-context) we publish to see what kind of metrics are available.

### Toggling Pod Metrics

#### Helm Chart Options

The first way is via the Helm chart. A global settings value for enabling metrics and debug endpoints on all pods part
of the Gloo Edge installation can be toggled using `global.glooStats.enabled` (default `true`). 

In addition, all deployment resources in the chart accept an argument `stats` which when set, override any default
value inherited from `global.glooStats`.

For example, to add stats to the Gloo Edge `gateway`, when installing with Helm add  `--set discovery.deployment.stats.enabled=true`.

For example, to add stats to the Gloo Edge `discovery` pod, first write your values file. Run:

```shell script
echo "crds:
  create: true # see our installation guide- only required if you are using Helm 2
discovery:
  deployment:
    stats:
      enabled: true
" > stats-values.yaml
```

Then install using one of the following methods:

{{< tabs >}}
{{< tab name="glooctl" codelang="shell" >}}
glooctl install gateway --values stats-values.yaml
{{< /tab >}}
{{% tab name="Helm 2" %}}
Either:

```shell script
helm install gloo/gloo --name gloo --namespace gloo-system -f stats-values.yaml
```

or:

```shell script
helm template gloo --namespace gloo-system --values stats-values.yaml  | kubectl apply -f - -n gloo-system
```
{{% /tab %}}
{{< tab name="Helm 3" codelang="shell">}}
helm install gloo gloo/gloo --namespace gloo-system -f stats-values.yaml
{{< /tab >}}
{{< /tabs >}}

Here's what the resulting `discovery` manifest would look like. Note the additions of the `prometheus.io` annotations,
and the `START_STATS_SERVER` environment variable.

{{< highlight yaml "hl_lines=18-21 32-33" >}}
apiVersion: apps/v1
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

#### Environment Variables

The other method is to manually set the `START_STATS_SERVER=1` in the pod.

### Monitoring Gloo Edge with Prometheus

Prometheus has great support for monitoring kubernetes pods. Docs for that can be found
[here](https://prometheus.io/docs/prometheus/latest/configuration/configuration/#kubernetes_sd_config). If the stats
are enabled through the Helm chart than the Prometheus annotations are automatically added to the pod spec. And those
Prometheus stats are available from the admin page in our pods.

For example, assuming you installed Gloo Edge as previously using Helm, and enabled stats for discovery, you
could then `kubectl port-forward <pod> 9091:9091` those pods (or deployments/services selecting those pods) to access
their admin page as follows.

```shell
kubectl --namespace gloo-system port-forward deployment/discovery 9091:9091
```

And then open <http://localhost:9091> for the admin page, including the Prometheus metrics at <http://localhost:9091/metrics>.

More information on Gloo Edge's admin ports can be found [here]({{% versioned_link_path fromRoot="/introduction/observability/#grafana-and-prometheus" %}}).

#### Metrics Context

You can see exactly what metrics are published from a particular pod by taking a look at our Prometheus
[Help strings](https://prometheus.io/docs/instrumenting/writing_exporters/#help-strings). For a given
pod you're interested in, you can curl `/metrics` on its stats port (usually `9091`) to see this content.

For example, here's a look at the Help strings published by our `gloo` pod as of 0.20.13. You can do the
same thing for any of our pods, including the closed-source ones in the case of Gloo Edge Enterprise.

```bash
$ kubectl port-forward deployment/gloo 9091 &
$ portForwardPid=$!
$ curl -s localhost:9091/metrics | grep HELP
# HELP api_gloo_solo_io_emitter_resources_in The number of resource lists received on open watch channels
# HELP api_gloo_solo_io_emitter_snap_in Deprecated. Use api.gloo.solo.io/emitter/resources_in. The number of snapshots updates coming in.
# HELP api_gloo_solo_io_emitter_snap_missed The number of snapshots updates going missed. this can happen in heavy load. missed snapshot will be re-tried after a second.
# HELP api_gloo_solo_io_emitter_snap_out The number of snapshots updates going out
# HELP eds_gloo_solo_io_emitter_resources_in The number of resource lists received on open watch channels
# HELP eds_gloo_solo_io_emitter_snap_in Deprecated. Use eds.gloo.solo.io/emitter/resources_in. The number of snapshots updates coming in.
# HELP eds_gloo_solo_io_emitter_snap_out The number of snapshots updates going out
# HELP gloo_solo_io_setups_run The number of times the main setup loop has run
# HELP grpc_io_server_completed_rpcs Count of RPCs by method and status.
# HELP grpc_io_server_received_bytes_per_rpc Distribution of received bytes per RPC, by method.
# HELP grpc_io_server_received_messages_per_rpc Distribution of messages received count per RPC, by method.
# HELP grpc_io_server_sent_bytes_per_rpc Distribution of total sent bytes per RPC, by method.
# HELP grpc_io_server_sent_messages_per_rpc Distribution of messages sent count per RPC, by method.
# HELP grpc_io_server_server_latency Distribution of server latency in milliseconds, by method.
# HELP kube_events_count The number of events sent from kuberenets to us
# HELP kube_lists_count The number of list calls
# HELP kube_req_in_flight The number of requests in flight
# HELP kube_updates_count The number of update calls
# HELP kube_watches_count The number of watch calls
# HELP runtime_goroutines The number of goroutines
# HELP setup_gloo_solo_io_emitter_resources_in The number of resource lists received on open watch channels
# HELP setup_gloo_solo_io_emitter_snap_in Deprecated. Use setup.gloo.solo.io/emitter/resources_in. The number of snapshots updates coming in.

$ kill $portForwardPid
```
