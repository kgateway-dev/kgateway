---
title: Production Deployments
description: This document shows some tips and tricks for deploying Gloo Edge into a production environment
weight: 20
---

This document shows some of the Production options that may be useful. We will continue to add to this document and welcome users of Gloo Edge to send PRs to this as well.

## Dropping capabilities

One of the more important (and unique) things about Gloo Edge is the ability to significantly lock down the edge proxy. Other proxies require privileges to write to disk or access the Kubernetes API, while Gloo Edge splits those responsibilities between control plane and data plane. The data plane can be locked down with zero privileges while separating out the need to secure the control plane differently. 

For example, Gloo Edge's data plane (the `gateway-proxy` pod) has ReadOnly file system. Additionally it doesn't require any additional tokens mounted in or OS-level privileges. By default some of these options are enabled to simplify developer experience, but if your use case doesn't need them, you should lock them down. 

* **Disable service account token mount**
    - For example, when integrating with Istio's SDS (see integration with Istio), you need to have a service account token mounted. If you're not integrating with Istio, you can eliminate the need for the service account token. When installing Gloo Edge, set the `gateway.proxyServiceAccount.disableAutomount` field. 
* **Disable Kubernetes destinations**
    - Gloo Edge out of the box routes to upstreams. It can also route directly to Kubernetes destinations (bypassing upstreams). Upstreams is the recommended abstraction to which to route in VirtualServices, and you can disable the Kubernetes destinations with the `settings.gloo.disableKubernetesDestinations`. This saves on memory overhead so Gloo Edge pod doesn't cache both upstreams and Kubernetes destinations. 

## Enable replacing invalid routes

* **Configure invalidConfigPolicy**
    - In some cases, it may be desirable to update a virtual service even if its config becomes partially invalid. This is particularly useful when delegating to Route Tables as it ensures that a single Route Table will not block updates for other Route Tables which share the same Virtual Service. More information on why and how to enable this can be found [here]({{% versioned_link_path fromRoot="/guides/traffic_management/configuration_validation/invalid_route_replacement/" %}})

## Enable health checks

{{% notice warning %}}
Liveness/readiness probes on Envoy are disabled by default. This is because Envoy's behavior can be surprising: When there are no
routes configured, Envoy reports itself as un-ready. As it becomes configured with a nonzero number of routes, it will start to
report itself as ready.
{{% /notice %}}

* **Enable liveness/readiness probes for Envoy**
    - To enable liveness and readiness probes, specify `gatewayProxies.gatewayProxy.podTemplate.probes=true` in your Helm installation.
If you are running Gloo Edge Enterprise, you'll need to prefix that Helm values key with `"gloo."`; e.g. `gloo.gatewayProxies.gatewayProxy.podTemplate.probes=true`.
* **Configure your load balancer correctly**
    - If you are running Gloo Edge behind a load balancer, be sure to configure your load balancer properly to consume the readiness probe mentioned above.

#### Upstream health checks

In addition to defining health checks for Envoy, you should strongly consider defining health checks for your `Upstreams`.
These health checks are used by Envoy to determine the health of the various upstream hosts in an upstream cluster, for example checking the health of the various pods that make up a Kubernetes `Service`. This is known as "active health checking" and can be configured on the `Upstream` resource directly.
[See the documentation]({{% versioned_link_path fromRoot="/guides/traffic_management/request_processing/upstream_health_checks/" %}}) for additional info.

Additionally, "outlier detection" can be configured which allows Envoy to passively check the health of upstream hosts.
A helpful [overview of this feature is available in Envoy's documentation](https://www.envoyproxy.io/docs/envoy/latest/intro/arch_overview/upstream/outlier).
This can be configured via the `outlierDetection` field on the `Upstream` resource. See the {{< protobuf name="gloo.solo.io.Upstream" display="API reference for more detail" >}}.

## Envoy performance

* **Enable Envoy's gzip filter**
    - Optionally, you may choose to enable Envoy's gzip filter through Gloo Edge. More information on that can be found [here]({{% versioned_link_path fromRoot="/installation/advanced_configuration/gzip/" %}}).
* **Set up an EDS warming timeout**
    - Set up the endpoints warming timeout to some nonzero value. More details [here]({{%versioned_link_path fromRoot="/operations/upgrading/1.3.0/#recommended-settings" %}}).

## Configure appropriate resource usage

* Before running in production it is important to ensure you have correctly configured the resources allocated to the various components of Gloo Edge. Ideally this tuning will be done in conjunction with load/performance testing.
These values can be configured via helm values for the various deployments, such as `gloo.deployment.resources.requests.*` or `gatewayProxies.gatewayProxy.podTemplate.resources.requests.*`.
See the [helm chart value reference]({{%versioned_link_path fromRoot="/reference/helm_chart_values/" %}}) for a full list.

## Metrics and monitoring

{{% notice note %}}
Gloo Edge default prometheus server and grafana instance are not meant to be used `as-is` in production. Please provide your own instance or configure the provided one with production values
{{% /notice %}}

When running Gloo Edge (or any application for that matter) in a production environment, it is important to have a monitoring solution in place.
Gloo Edge Enterprise provides a simple deployment of Prometheus and Grafana to assist with this necessity.
However, depending on the requirements on your organization you may require a more robust solution, in which case you should make sure the metrics from the Gloo Edge components (especially Envoy) are available in whatever solution you are using.
The [general documentation for monitoring/observability]({{%versioned_link_path fromRoot="/guides/observability/" %}}) has more info.

Some metrics that may be useful to monitor (listed in Prometheus format):
* `envoy_control_plane_connected_state` -- This metric shows whether or not a given Envoy instance is connected to the control plane, i.e. the Gloo pod.
 This metric should have a value of `1` otherwise it indicates that Envoy is having trouble connecting to the Gloo pod.
* `container_cpu_cfs_throttled_seconds_total / container_cpu_cfs_throttled_periods_total` -- This is a generic expression that will show whether or not a given container is being throttled for CPU, which will result is performance issues and service degradation. If the Gloo Edge containers are being throttled, it is important to understand why and given the underlying cause, increase the resources allocated.

### Troubleshooting monitoring components

A common issue in production (or environments with high traffic) is to have sizing issues. This will result in an abnormal number of restarts like this:
```shell
$ kubectl get all -n gloo-system
NAME                                                       READY   STATUS             RESTARTS   AGE
pod/discovery-9d4c7fb4c-5wq5m                              1/1     Running            13         35d
pod/extauth-77bb4fc79b-dsl6q                               1/1     Running            0          35d
pod/gateway-f774b4d5b-jfhwn                                1/1     Running            0          35d
pod/gateway-proxy-7656d9df87-qtn2s                         1/1     Running            0          35d
pod/gloo-db4fb8c4-lfcrp                                    1/1     Running            13         35d
pod/glooe-grafana-78c6f96db-wgl5k                          1/1     Running            0          41d
pod/glooe-prometheus-kube-state-metrics-5dd77b76fc-s8prb   1/1     Running            0          41d
pod/glooe-prometheus-server-59dcf7bc5b-jt654               1/2     CrashLoopBackOff   10692      41d
pod/observability-656d47787-2fskq                          0/1     CrashLoopBackOff   9558       33d
pod/rate-limit-7d6cf64fbf-ldgbp                            1/1     Running            0          35d
pod/redis-55d6dbb6b7-ql89p                                 1/1     Running            0          41d
```

Looking at the cause of these restarts, we can see that the PV is exhausted:
```shell
kubectl logs -f pod/glooe-prometheus-server-59dcf7bc5b-jt654 -n gloo-system -c glooe-prometheus-server
evel=info ts=2021-07-07T05:12:29.474Z caller=main.go:574 msg="Stopping scrape discovery manager..."
level=info ts=2021-07-07T05:12:29.474Z caller=main.go:588 msg="Stopping notify discovery manager..."
level=info ts=2021-07-07T05:12:29.474Z caller=main.go:610 msg="Stopping scrape manager..."
level=info ts=2021-07-07T05:12:29.474Z caller=manager.go:908 component="rule manager" msg="Stopping rule manager..."
level=info ts=2021-07-07T05:12:29.474Z caller=manager.go:918 component="rule manager" msg="Rule manager stopped"
level=info ts=2021-07-07T05:12:29.474Z caller=notifier.go:601 component=notifier msg="Stopping notification manager..."
level=info ts=2021-07-07T05:12:29.474Z caller=main.go:778 msg="Notifier manager stopped"
level=info ts=2021-07-07T05:12:29.474Z caller=main.go:604 msg="Scrape manager stopped"
level=info ts=2021-07-07T05:12:29.474Z caller=main.go:570 msg="Scrape discovery manager stopped"
level=info ts=2021-07-07T05:12:29.474Z caller=main.go:584 msg="Notify discovery manager stopped"
level=error ts=2021-07-07T05:12:29.474Z caller=main.go:787 err="opening storage failed: open /data/wal/00000721: no space left on device"
```

Next step is to check the pv size and the retention time:
```shell
kubectl get deploy/glooe-prometheus-server -n gloo-system -oyaml|grep "image: prom/prometheus" -C 10
        - mountPath: /etc/config
          name: config-volume
          readOnly: true
      - args:
        - --storage.tsdb.retention.time=15d
        - --config.file=/etc/config/prometheus.yml
        - --storage.tsdb.path=/data
        - --web.console.libraries=/etc/prometheus/console_libraries
        - --web.console.templates=/etc/prometheus/consoles
        - --web.enable-lifecycle
        image: prom/prometheus:v2.21.0
        imagePullPolicy: IfNotPresent
        livenessProbe:
          failureThreshold: 3
          httpGet:
            path: /-/healthy
            port: 9090
            scheme: HTTP
          initialDelaySeconds: 30
          periodSeconds: 15
          successThreshold: 1
```
```shell
kubectl get pv -oyaml|grep "glooe-prometheus-server" -C 10
    selfLink: /api/v1/persistentvolumes/pvc-a616d9c6-9733-428f-bac5-c054ca8a025b
    uid: 816c7e99-55e3-4888-83f0-2f31a8314b47
  spec:
    accessModes:
    - ReadWriteOnce
    capacity:
      storage: 16Gi
    claimRef:
      apiVersion: v1
      kind: PersistentVolumeClaim
      name: glooe-prometheus-server
      namespace: gloo-system
      resourceVersion: "36337"
      uid: a616d9c6-9733-428f-bac5-c054ca8a025b
    gcePersistentDisk:
      fsType: ext4
      pdName: gke-jesus-lab-observability-9bc1029-pvc-a616d9c6-9733-428f-bac5-c054ca8a025b
    nodeAffinity:
      required:
        nodeSelectorTerms:
        - matchExpressions:
```

In this case, 16Gi volume size and 15d retention is not working well, so we must tune one or both parameters using these helm values:
```
prometheus.server.retention
prometheus.server.persistentVolume.size
```

Choosing the right values requires some business knowledge, but as a rule of thumb a fair approach is:
```
persistentVolume.size = retention_in_seconds * ingested_samples_per_second * bytes_per_sample
```

## Access Logging

Envoy provides a powerful access logging mechanism which enables users and operators to understand the various traffic flowing through the proxy.
Before deploying Gloo Edge in production, consider enabling access logging to help with monitoring traffic as well as to provide helpful information for troubleshooting.
The [access logging documentation]({{%versioned_link_path fromRoot="/guides/security/access_logging/" %}}) should be consulted for more details.

## Other Envoy-specific guidance

* Envoy has a list of edge proxy best-practices in their docs. You may also want to consult that to see what is applicable for your use case. Find those docs [here](https://www.envoyproxy.io/docs/envoy/latest/configuration/best_practices/edge#best-practices-edge).
    - In particular, you may especially want to set `use_remote_address` to true. More details [here](https://www.envoyproxy.io/docs/envoy/latest/api-v3/extensions/filters/network/http_connection_manager/v3/http_connection_manager.proto#envoy-v3-api-field-extensions-filters-network-http-connection-manager-v3-httpconnectionmanager-use-remote-address)
