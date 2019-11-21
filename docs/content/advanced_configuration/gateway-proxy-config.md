---
title: Envoy Configuration Overrides
weight: 60
description: Persistent configuration for `gateway-proxy`
---

## What is the gateway-proxy?

The gateway-proxy is a component of Gloo that is essentially a wrapper around [Envoy](https://www.envoyproxy.io/learn/).
When you deploy Gloo, running `k describe pods` should show a `gateway-proxy` pod in the namespace that
you've installed Gloo in (usually `gloo-system`).

## Configuring the gateway-proxy

Envoy's [bootstrap configuration](https://www.envoyproxy.io/docs/envoy/latest/configuration/overview/v2_overview#bootstrap-configuration)
can be done in two ways: 1) with a configuration file that we represent as a `gateway-proxy-v2-envoy-config` configmap
and 2) with command-line arguments that we pass in to the `gateway-proxy` container.

To dynamically edit either of these, it is simply a matter of using `k edit configmap -n gloo-system gateway-proxy-v2-envoy-config`
or `k edit deployment -n gloo-system gateway-proxy-v2 -oyaml`.

Continue reading if you'd like to set this configuration at install time.

### ConfigMap

We use Helm charts and Helm templates to configure the gateway-proxy config map. To see the entire
list of Gloo Helm Overrides, see our [list of Helm Chart values](https://docs.solo.io/gloo/latest/installation/gateway/kubernetes/#list-of-gloo-helm-chart-values).

To see an example config map, look no further than [Envoy's configuration documentation](https://www.envoyproxy.io/docs/envoy/latest/configuration/overview/v2_overview#bootstrap-configuration).

An example `values.yaml` file setting up a gateway-proxy config map is:
```cassandraql
gatewayProxies:
  gatewayProxy:
    configMap:
      data: null # override our default envoy configmap here
```

### Container Command-line Arguments

We use Helm charts and Helm templates to configure the gateway-proxy container. To see the entire
list of Gloo Helm Overrides, see our [list of Helm Chart values](https://docs.solo.io/gloo/latest/installation/gateway/kubernetes/#list-of-gloo-helm-chart-values).

Among these, you'll notice a `gatewayProxies.NAME.extraEnvoyArgs` string override. This is where
you will pass in the extra envoy command line arguments.

To see a list of available Envoy command line arguments, see their [latest documentation on it](https://www.envoyproxy.io/docs/envoy/latest/operations/cli).

An example `values.yaml` file that you could pass in to configure the `gatewayProxy` is:
```cassandraql
gatewayProxies:
  gatewayProxy:
    extraEnvoyArgs:
      - component-log-level
      - upstream:debug,connection:trace
      - disable-hot-restart
```

