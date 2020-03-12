---
title: Debugging Gloo
description: This document shows how some common ways to debug Gloo and Envoy
weight: 10
---

At times you may need to debug Gloo and misconfigurations. Gloo is based on Envoy and often times these misconfigurations are observed as a result of behavior seen at the proxy. This guide will help you debug issues with Gloo and Envoy. 


## The Proxy Resources
One of the first places to look is the Gloo configurations: {{< protobuf name="gateway.solo.io.VirtualService" display="VirtualService">}}, {{< protobuf name="gateway.solo.io.Gateway" display="Gateway">}}, and {{< protobuf name="gloo.solo.io.Proxy" display="Proxy">}}. For example, when you specify routing configurations, you do that in `VirtualService` resources. Ultimately, these resources get compiled down into the `Proxy` resource which ends up being the source of truth of the configuration that is served over xDS to Envoy. Your best bet is to start by checking the `Proxy` resource:

```bash
kubectl get proxy gateway-proxy -n gloo-system -o yaml
```
This combines both `Gateway` and `VirtualService` resources into a single document. Here you can verify whether your `VirtualService` or `Gateway` configurations were properly picked up. If not, you should check the `gateway` and `gloo` pods for error logs (see next section). 

## Dumping Envoy configuration
If the `Proxy` object looks okay, your next "source of truth" is what Envoy sees. Ultimately, the proxy behavior is based on what configuration is served to Envoy, so this is a top candidate to see what's actually happening. 

You can easily see the Envoy proxy configuration by running the following command:

```bash
glooctl proxy dump
```

This dumps the entire Envoy configuration including all static and dynamic resources. Typically at the bottom you can see the VirtualHost and Route sections to verify your settings were picked up correctly.

## Viewing Envoy logs

If things look okay (within your ability to tell), another good place to look is the Envoy proxy logs. You can very quickly turn on `debug` logging to Envoy as well as `tail` the logs with this handy `glooctl` command:

```bash
glooctl proxy logs -f
```

When you have the logging window up, send requests through to the proxy and you can get some very detailed debugging logging going through the log tail. 

Additionally, you can enable access logging to dump specific parts of the request into the logs. Please see the [doc on access logging]({{< versioned_link_path fromRoot="/gloo_routing/gateway_configuration/access_logging/" >}}) to configure that. 


## Viewing Envoy stats
Envoy collects a wealth of statistics and makes them available for metric-collection systems like Prometheus, Statsd, and Datadog (to name a few). You can also very quickly get access to the stats from the cli:

```bash
glooctl proxy stats
```

## All else with Envoy: bootstrap and Admin

There may be more limited times where you need direct access to the Envoy Admin API. You can view both the Envoy bootstrap config as well as access the Admin API with the following commands:

```bash
kubectl exec -it -n gloo-system deploy/gateway-proxy \
-- cat /etc/envoy/envoy.yaml
```

You can port-forward the Envoy Admin API similarly:

```bash
kubectl port-forward -n gloo-system deploy/gateway-proxy 19000:19000
```

That way you can `curl localhost:19000` and get access to the Envoy Admin API. 

## Pull the rip cord

If all else fails, you can capture the state of Gloo configurations and logs and join us on our Slack (https://slack.solo.io) and one of our engineers will be able to help:

```bash
glooctl debug logs -f gloo-logs.log
glooctl debug yaml -f gloo-yamls.yaml
```