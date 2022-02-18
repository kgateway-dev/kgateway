---
title: Debugging Gloo Edge
description: This document shows how some common ways to debug Gloo Edge and Envoy
weight: 10
---

At times you may need to debug Gloo Edge misconfigurations. Gloo Edge is based on [Envoy](https://www.envoyproxy.io) and often times these misconfigurations are observed as a result of behavior seen at the proxy. This guide will help you debug issues with Gloo Edge and Envoy. 

The guide is broken into 3 main sections:

* General debugging tools and tips
* Debugging the data plane 
* Debugging the control plane


### Pulling the rip cord

This guide is intended to help you understand where to look if things aren't working as expected. After going through, if all else fails, you can capture the state of Gloo Edge configurations and logs and join us on our Slack (https://slack.solo.io) and one of our engineers will be able to help:

```bash
glooctl debug logs -f gloo-logs.log
glooctl debug yaml -f gloo-yamls.yaml
```

This will dump all of the relevant configuration into to files, `gloo-logs.log` and `gloo-yamls.yaml` which gives a complete picture of your deployment. 

## General debugging tools and tips

If you're experiencing unexpected behavior after installing and configuring Gloo Edge, the first thing to do is verify [installation]({{< versioned_link_path fromRoot="/installation/" >}}) and configuration. The fastest way to do that is to run the `glooctl check` [command]({{< versioned_link_path fromRoot="/reference/cli/glooctl_check/" >}}). This command will go through the deployments, pods and Gloo Edge resources to make sure they're in a healthy/Accepted/OK status. Typically if there is some problem syncing resources, you'd find an issue here.

```bash
glooctl check
```

Output should be similar to:

```
Checking deployments... OK
Checking pods... OK
Checking upstreams... OK
Checking upstream groups... OK
Checking secrets... OK
Checking virtual services... OK
Checking gateways... OK
Checking proxies... OK
No problems detected.
```

### The VirtualService, Gateway, and Proxy resource
One of the first places to look is the Gloo Edge configurations: {{< protobuf name="gateway.solo.io.VirtualService" display="VirtualService">}}, {{< protobuf name="gateway.solo.io.Gateway" display="Gateway">}}, and {{< protobuf name="gloo.solo.io.Proxy" display="Proxy">}}. For example, when you specify routing configurations, you do that in `VirtualService` resources. Ultimately, these resources get compiled down into the `Proxy` resource which ends up being the source of truth of the configuration for the control plane that is served over xDS to Envoy. Your best bet is to start by checking the `Proxy` resource:

```bash
kubectl get proxy gateway-proxy -n gloo-system -o yaml
```
This combines both `Gateway` and `VirtualService` resources into a single document. Here you can verify whether your `VirtualService` or `Gateway` configurations were properly picked up. If not, you should check the `gateway` and `gloo` pods for error logs (see next section). 

### Upstreams

When using [dynamic Upstream discovery]({{< versioned_link_path fromRoot="/guides/traffic_management/destination_types/discovered_upstream/" >}}) (default, out of the box), and making changes to those upstreams (ie, adding TLS), you may end up with misconfigured or `Rejected` Upstreams that can cause resources that depend on them to show failures (ie, VirtualServices, RouteTable, etc). To determine whether your Upstreams are in a healthy state, run the following and examine the `STATUS` column:

```bash
glooctl get upstreams
```

## Debugging the data plane

Gloo Edge is based on Envoy proxy which means there is a lot of [generic Envoy debugging knowledge](https://www.envoyproxy.io/docs/envoy/latest/operations/operations) that is applicable to Gloo Edge. When you find unexpected behaviors with your request handling, here are a few areas to look in Envoy that can aid in debugging. Note, we've created some convenience tooling in the `glooctl` CLI tool which is tremendously helpful here.


### Dumping Envoy configuration
If the `Proxy` resource that gets compiled from your `VirtualService` and `Gateway` resources looks okay, your next "source of truth" is what Envoy sees. Ultimately, the proxy behavior is based on what configuration is served to Envoy, so this is a top candidate to see what's actually happening. 

You can easily see the Envoy proxy configuration by running the following command:

```bash
glooctl proxy dump
```

This dumps the entire Envoy configuration including all static and dynamic resources. Typically at the bottom you can see the VirtualHost and Route sections to verify your settings were picked up correctly.

### Viewing Envoy logs

If things look okay (within your ability to tell), another good place to look is the Envoy proxy logs. You can very quickly turn on `debug` logging to Envoy as well as `tail` the logs with this handy `glooctl` command:

```bash
glooctl proxy logs -f
```

When you have the logging window up, send requests through to the proxy and you can get some very detailed debugging logging going through the log tail. 

Additionally, you can enable access logging to dump specific parts of the request into the logs. Please see the [doc on access logging]({{< versioned_link_path fromRoot="/guides/security/access_logging//" >}}) to configure that. 

### Viewing Envoy stats
Envoy collects a wealth of statistics and makes them available for metric-collection systems like Prometheus, Statsd, and Datadog (to name a few). You can also very quickly get access to the stats from the cli:

```bash
glooctl proxy stats
```

### All else with Envoy: bootstrap and administration

There may be more limited times where you need direct access to the [Envoy Admin API](https://www.envoyproxy.io/docs/envoy/latest/operations/admin). You can view both the Envoy bootstrap config as well as access the [Admin API](https://www.envoyproxy.io/docs/envoy/latest/operations/admin) with the following commands:

```bash
kubectl exec -it -n gloo-system deploy/gateway-proxy \
-- cat /etc/envoy/envoy.yaml
```

You can port-forward the Envoy Admin API similarly:

```bash
kubectl port-forward -n gloo-system deploy/gateway-proxy 19000:19000
```

Now you can `curl localhost:19000` and get access to the Envoy Admin API. 

Note that after enabling `debug` logging, the Envoy proxy does not automatically revert to the default `info` logging. To reset logging to the default `info` level, you can click **logging** in the Admin UI or run the following `curl` command in the CLI:
```bash
curl -X POST http://localhost:19000/logging\?level\=info
```

## Debugging the control plane

The Gloo Edge control plane is made up of the following components:

```bash
NAME                             READY   STATUS    RESTARTS   AGE
discovery-857796b8fb-gcphh       1/1     Running   0          15h
gateway-5d7dd58d5f-8z48k         1/1     Running   0          15h
gateway-proxy-8689c55fb8-7swfq   1/1     Running   0          15h
gloo-66fb8974c9-8sgll            1/1     Running   0          15h
```

You will see more components for the [Enterprise installation]({{< versioned_link_path fromRoot="/installation/enterprise/" >}})

```bash
NAME                                                  READY   STATUS    RESTARTS   AGE
api-server-8575657b8-sqdnv                            3/3     Running   0          7m22s
discovery-5bbbc474b9-kvhkb                            1/1     Running   0          7m22s
extauth-6f976948cf-q6hbf                              1/1     Running   0          7m21s
gateway-79cb559db-d9lx2                               1/1     Running   0          7m22s
gateway-proxy-79cb47d5b6-6qmph                        1/1     Running   0          7m22s
gloo-79d584c959-x8rk2                                 1/1     Running   0          2m29s
glooe-grafana-f58f664c-84txh                          1/1     Running   0          7m22s
glooe-prometheus-kube-state-metrics-64fd97986-fb2wl   1/1     Running   0          7m22s
glooe-prometheus-server-694dc99cd4-k6g75              2/2     Running   0          7m22s
observability-6df4b5d9fd-pvlbb                        1/1     Running   0          7m22s
rate-limit-598fbc996d-skfmz                           1/1     Running   1          7m21s
redis-5bf75869f4-4v2j7                                1/1     Running   0          7m22s
```

Each component keeps logs about the sync loops it does (syncing with various environment signals like the Kube API, or Consul, etc). You can get all of logs for the components with the following command:

```bash
glooctl debug logs
```

This returns a LOT of logging. You can save the logs off by passing in a filename:

```bash
glooctl debug logs -f gloo.log
```

If you just want to see errors (most useful):

```bash
glooctl debug logs --errors-only
```

Likely you just want to see each individual components logs. You can use `kubectl logs` command for that. For example, to see the `gloo` components logs:

```bash
kubectl logs -f -n gloo-system -l gloo=gloo
```

To follow the logs of other Gloo Edge deployments, simply change the value of the `gloo` label as shown in the table below.

| Component | Command |
| ------------- | ------------- |
| Discovery | `kubectl logs -f -n gloo-system -l gloo=discovery` |
| External Auth (Enterprise) | `kubectl logs -f -n gloo-system -l gloo=extauth` |
| Gateway | `kubectl logs -f -n gloo-system -l gloo=gateway`  |
| Gloo Control Plane | `kubectl logs -f -n gloo-system -l gloo=gloo` |
| Observability (Enterprise) | `kubectl logs -f -n gloo-system -l gloo=observability` |
| Rate Limiting (Enterprise) | `kubectl logs -f -n gloo-system -l gloo=rate-limit` |

### Changing logging levels and more

Each Gloo Edge control plane component comes with an optional debug port that you can enable with the `START_STATS_SERVER` environment variable. To get access to the port, you can forward the port of the Kubernetes deployment such as with the following command:

```bash
kubectl port-forward -n gloo-system deploy/gloo 9091:9091
```

Now you can navigate to `http://localhost:9091` and you get a simple page with some additional endpoints:

* `/debug/pprof`
* `/logging`
* `/metrics`
* `/zpages`

With these endpoints, you can profile the behavior of the component, adjust its logging, view the prometheus-style telemetry signals, as well as view tracing spans within the process. This is a very handy page to understand the behavior of a particular component. 

To change the log levels of individual Gloo Edge deployments from the CLI instead of the Admin UI, use commands similar to the following example with the `discovery` deployment.

```bash
kubectl port-forward -n gloo-system deploy/discovery 9091:9091
# Change log level to debug for discovery deployment
% curl -X PUT -H "Content-Type: application/json" -d '{"level": "debug"}' http://localhost:9091/logging
# Change log level to info for discovery deployment
% curl -X PUT -H "Content-Type: application/json" -d '{"level": "info"}' http://localhost:9091/logging
```

#### Declaratively setting log levels on start

Setting the `LOG_LEVEL` environment variable within `gloo`, `discovery`, `gateway` or gateway proxy deployments will change the level at which the stats server logs. The default log level for the stats server is `info`.

Other acceptable log levels for Gloo Edge components are:

* `debug`
* `error`
* `warn`
* `panic`
* `fatal`

With Helm installations, these can be set for you by providing the desired level as a value for the `logLevel` key for each of those components. You can also define the [Envoy log level](https://www.envoyproxy.io/docs/envoy/latest/start/quick-start/run-envoy#debugging-envoy) by setting the `envoyLogLevel` value on gateway proxies.


### All else fails

Again, if all else fails, you can capture the state of Gloo Edge configurations and logs and join us on our Slack (https://slack.solo.io) and one of our engineers will be able to help:

```bash
glooctl debug logs -f gloo-logs.log
glooctl debug yaml -f gloo-yamls.yaml
```
