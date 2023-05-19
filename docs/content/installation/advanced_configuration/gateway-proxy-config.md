---
title: Envoy Bootstrap Configuration
weight: 60
description: Bootstrap configuration for Envoy
---

## Configuring Envoy

Envoy's [bootstrap configuration](https://www.envoyproxy.io/docs/envoy/latest/configuration/overview/bootstrap) can be done in two ways: 1) with a configuration file that we represent as the config map `gateway-proxy-envoy-config` and 2) with command-line arguments that we pass in to the `gateway-proxy` pod.

You do not need to set either of these manually - gloo has default settings for both in its Helm chart.

### Configuration File

The Helm value that overrides our default bootstrap configuration is `gatewayProxies.$PROXY_NAME.configMap`. To see an example config map, look no further than [Envoy's configuration documentation](https://www.envoyproxy.io/docs/envoy/latest/configuration/overview/bootstrap).

To see the entire list of Gloo Edge Helm Overrides, see our [list of Helm Chart values]({{< versioned_link_path fromRoot="/reference/helm_chart_values/" >}}).

### Command-line Arguments

The Helm value that sets additional Envoy command line arguments is `gatewayProxies.NAME.extraEnvoyArgs`. 

To see a list of available Envoy command line arguments, see their [latest command line documentation](https://www.envoyproxy.io/docs/envoy/latest/operations/cli).

{{% notice note %}}
We will always set `--disable-hot-restart` regardless of any value provided to `extraEnvoyArgs`.
{{% /notice %}}

An example `values.yaml` file that you could pass in to configure Envoy is:
```
gatewayProxies:
  gatewayProxy:
    extraEnvoyArgs:
      - --component-log-level
      - upstream:debug,connection:trace
```

This sets the log levels of individual Envoy components - setting the upstream log levels to `debug` and the `connection` component's log level to `trace`.
 
