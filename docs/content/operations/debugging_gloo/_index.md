---
title: Debugging Gloo Edge
description: This document shows how some common ways to debug Gloo Edge and Envoy
weight: 10
---

At times, you may need to debug Gloo Edge misconfigurations. Gloo Edge is based on [Envoy](https://www.envoyproxy.io) and often times these misconfigurations are observed as a result of behavior seen at the proxy. This guide will help you debug issues with Gloo Edge and Envoy. 

The guide is broken into three main sections:

* General debugging tools and tips
* Debugging the data plane 
* Debugging the control plane

This guide is intended to help you understand where to look if things aren't working as expected. After going through, if [all else fails](#all-else-fails), you can capture the state of Gloo Edge configurations and logs and join us on our [Slack](https://slack.solo.io) and one of our engineers will be able to help.



## General debugging tools and tips

Review general troubleshooting steps that you can take to start troubleshooting your Gloo Edge setup. 

### Check the Gloo Edge installation and resources {#glooctl-check}

If you experience issues with Gloo Edge, the first thing you can do is to verify your Gloo Edge setup and resources. You can do that by using the `glooctl check` [command]({{< versioned_link_path fromRoot="/reference/cli/glooctl_check/" >}}) that quickly checks the health of Gloo Edge deployments, pods, and custom resources, and verifies Gloo resource configuration. Any issues that are found are reported back in the CLI output.

A common issue that you can detect with the `glooctl check` command is a misconfigured or rejected upstream. For example, if [dynamic upstream discovery]({{< versioned_link_path fromRoot="/guides/traffic_management/destination_types/discovered_upstream/" >}}) is enabled in your environment, and you introduced a faulty upstream configuration, such as by adding a misconfigured TLS section, the resources that reference the misconfigured upstream, such as a virtual service or route table, start reporting errors. These errors can be seen in the output of the `glooctl check` command. 

{{% notice note %}}
Make sure to use the version of `glooctl` that matches your installed version, such as {{< readfile file="static/content/version_gee_latest.md" markdown="true">}}.
{{% /notice %}}

```bash
glooctl check
```

Example output for a healthy setup: 
```
Checking deployments... OK
Checking pods... OK
Checking upstreams... OK
Checking upstream groups... OK
Checking auth configs... OK
Checking rate limit configs... OK
Checking VirtualHostOptions... OK
Checking RouteOptions... OK
Checking secrets... OK
Checking virtual services... OK
Checking gateways... OK
Checking proxies... OK
No problems detected.
```

### Verify the Envoy configuration in the Gloo Edge xDS server

When you deploy virtual service, gateway, or route table resources, these resources are translated into valid Envoy configuration and made available to the gateway proxies in your cluster. As part of the translation process, Gloo Edge creates an internal proxy resource that includes all of the Envoy configuration that you want to apply to the proxies. This proxy is sent to the Gloo Edge xDS server. 

To see the Envoy configuration that is currently served by the Gloo Edge xDS server, you can use the following command. Note that this command returns a lot of information and can be hard to decrypt. However, if you cannot find a specific configuration in the command output, the configuration was likely rejected by Gloo Edge due to errors or conflicts. 

```bash
glooctl proxy served-config
```


## Debugging the data plane

Gloo Edge is based on Envoy proxies. If requests are handled incorrectly, use the following `glooctl` CLI tools to debug your Envoy configuration.  

### General steps

1. Find the resources that report errors. You can use the [`glooctl check` command](#glooctl-check) to check all of your resources or get the details of the virtual service, gateway, or route table that behave incorrectly. Check if the resource shows any errors in their status. 
   ```sh
   glooctl check
   kubectl get virtualservice <virtualservice-name> -n <namespace> -o yaml
   kubectl get routetable <routetable-name> -n <namespace> -o yaml
   kubectl get gateway <gateway-name> -n <namespace> -o yaml
   ```

2. If the resources seem to be ok, you can check any referenced upstreams to verify that they are configured correctly. In particular, check that the following settings are correct:
   * Label selectors for the app service
   * The port that the app serves on
   * The IP address of the app service that the upstream points to
   ```sh
   kubectl get upstream <upstream-name> -n <namespace> -o yaml
   ```

3. If the upstream is configured correctly, make sure that the service it references also specifies the right port and selector, and that the app pods are up and running. 
   ```sh
   kubectl get service <service-name> -n <namespace> -o yaml
   kubectl get pod <pod-name> -n <namespace> -o yaml
   ```

4. Next, check the proxy configuration that is served by the Gloo Edge xDS server. When you create Gloo Edge resources, these resources are translated into Envoy configuration and sent to the xDS server. If Gloo Edge resources are configured correctly, the configuration must be included in the proxy configuration that is served by the xDS server. 
   ```sh
   glooctl proxy served config
   ```

5. If the Gloo Edge xDS server has the correct configuration, you can then check what configuration is served by the gateway proxies in your cluster. 
   ```sh
   glooctl proxy dump
   ```

   To dive deeper into your Envoy configuration and view logs, stats, and other information, see the following links. 
   * [Dumping Envoy configuration](#dumping-envoy-configuration)
   * [Viewing Envoy logs](#viewing-envoy-logs)
   * [Viewing Envoy stats](#viewing-envoy-stats)
   * [View Envoy bootstrap config and access the Admin API](#view-envoy-bootstrap-config-and-access-the-admin-api)

6. If you find that the Envoy proxy serves incorrect configuration, you can increase the number of gateway proxy replicas so that new gateway proxy pods are created. The new gateway proxy pods pull the latest Envoy configuration from the Gloo Edge xDS server. You can then use the `glooctl proxy dump` command to verify that the correct Envoy configuration is applied. Then, you can remove replicas with stale configuration and scale down the number of gateway proxies again. 
   1. Scale the gateway proxy deployment. 
      ```sh
      kubectl scale deployment gateway-proxy -n gloo-system replicas=2
      ```

   2. Verify that the proxy pod is up and running. 
      ```sh
      kubectl get pods -n gloo-system
      ```

   3. Port-forward the new proxy pod on port 19000. 
      ```sh
      kubectl -n gloo-system port-forward <pod name> 19000 &
      ```
   
   4. Generate the Envoy proxy dump and save the output to a file. Verify that the correct Envoy configuration is served by the new proxy pod. 
      ```sh
      curl -X POST 127.0.0.1:19000/config_dump\?include_eds > gateway-config.json
      ```

   5. Remove the old proxy pod that served the stale Envoy configuration.
      ```sh
      kubectl delete pod <pod-name> -n gloo-system
      ```
   
   6. Scale down the gateway proxy deployment. 
      ```sh
      kubectl scale deployment gateway-proxy -n gloo-system replicas=1
      ```

### Dumping Envoy configuration
If the `Proxy` resource that gets compiled from your `VirtualService` and `Gateway` resources looks okay, your next "source of truth" is what Envoy sees. Ultimately, the proxy behavior is based on what configuration is served to Envoy, so this is a top candidate to see what's actually happening. 

You can easily see the Envoy proxy configuration by running the following command:

```bash
glooctl proxy dump
```

This command dumps the entire Envoy configuration, including all static and dynamic resources. Typically near the end, you can see the VirtualHost and Route sections to verify that your settings are picked up correctly.

A more advanced way of generating the Envoy config dump is to port-forward to one of the gateway-proxy pods and to run the following commands:

```bash
# 1. pick a gateway-proxy pod
kubectl -n gloo-system get pod -l "gloo=gateway-proxy"
# 2. port-forward on port 19000
kubectl -n gloo-system port-forward <pod name> 19000 &
# 3a. generate the config dump
curl -X POST 127.0.0.1:19000/config_dump > gateway-config.json
# 3b. optionally include the upstream endpoints in the config dump
curl -X POST 127.0.0.1:19000/config_dump\?include_eds > gateway-config.json
```

Finally, you can use the Solo.io Envoy UI to browse the config. You can safely upload your config-dump on the website (it will stay offline) and visit or search through the different configuration nodes: https://envoyui.solo.io/

![Envoy UI]({{% versioned_link_path fromRoot="/img/envoy-ui.png" %}})



### Viewing Envoy logs

If things look okay (within your ability to tell), another good place to look is the Envoy proxy logs. You can very quickly turn on `debug` logging to Envoy as well as `tail` the logs with this handy `glooctl` command:

```bash
glooctl proxy logs -f
```

When you have the logging window up, send requests through to the proxy and you can get some very detailed debugging logging going through the log tail.

{{% notice warning %}}
Keep in mind that this command will actually [change](https://github.com/solo-io/gloo/blob/c2e025728df3c66c67275ac718e251a275d32bd3/projects/gloo/cli/pkg/cmd/gateway/logs.go#L65) the log level to `debug`. You might want to revert it to `info` after that, as shown in the following commands.
{{% /notice %}}

A more advanced way of changing the log level, globally or on a per-logger basis, is through the Envoy Admin endpoints:

```bash
# 1. port-forward to the gateway-proxy pod
kubectl -n gloo-system port-forward deploy/gateway-proxy 19000 &
# 2. globally change the log level to debug
curl -X POST "127.0.0.1:19000/logging?level=debug"
# 3. change the log level only for the aws logger
curl -X POST "127.0.0.1:19000/logging?aws=debug"
```

For a full list of the different Envoy loggers, visit the following endpoint: `http://localhost:19000/logging`

Additionally, you can configure access logging to dump specific parts of the request into the logs. For more information, see [Access Logging]({{< versioned_link_path fromRoot="/guides/security/access_logging//" >}}). 


### Viewing Envoy stats
Envoy collects a wealth of statistics and makes them available for metric-collection systems like Prometheus, Statsd, and Datadog (to name a few). You can also very quickly get access to the stats from the cli:

```bash
glooctl proxy stats
# example with filtering on a particular upstream name:
glooctl proxy stats -n gloo-system --name internal-proxy | grep -i default-httpbin-8000
```

### View Envoy bootstrap config and access the Admin API

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
NAME                                                      READY   STATUS    RESTARTS   AGE
discovery-68dbd794-ssx7b                                  1/1     Running   0          107m
extauth-67557744dd-5wc2p                                  1/1     Running   0          107m
gateway-595cc67f54-tr6ps                                  1/1     Running   0          107m
gateway-proxy-79c9f44b5d-cprg7                            1/1     Running   0          107m
gloo-74bb8b9df7-72t8m                                     1/1     Running   0          107m
gloo-fed-857964dd9f-gq8np                                 1/1     Running   0          107m
gloo-fed-console-6f99dddccd-ls64k                         3/3     Running   0          107m
glooe-grafana-865bb9cd45-cdshq                            1/1     Running   0          107m
glooe-prometheus-kube-state-metrics-v2-55ffc89cbb-kr8jx   1/1     Running   0          107m
glooe-prometheus-server-7d5b85764c-2nl2w                  2/2     Running   0          107m
observability-5f8ffc8bdc-zggxb                            1/1     Running   0          107m
rate-limit-6d66688567-5tcx8                               1/1     Running   3          107m
redis-57fd559c5c-hcd6n                                    1/1     Running   0          107m
```

Each component logs the sync loops that it runs, such as syncing with various environment signals like the Kubernetes API, Consul, etc. 

You can fetch the latest logs for all the components with the following command:

```bash
glooctl debug logs
# save the logs to a file
glooctl debug logs -f gloo.log
# only print errors
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

### Declaratively setting the log levels

Setting the `LOG_LEVEL` environment variable within `gloo`, `discovery`, `gateway` or gateway proxy deployments will change the level at which the stats server logs. The default log level for the stats server is `info`.

Other acceptable log levels for Gloo Edge components are:

* `debug`
* `error`
* `warn`
* `panic`
* `fatal`

With Helm installations, these can be set for you by providing the desired level as a value for the `logLevel` key for each of those components. You can also define the [Envoy log level](https://www.envoyproxy.io/docs/envoy/latest/start/quick-start/run-envoy#debugging-envoy) by setting the `envoyLogLevel` value on gateway proxies.

Example for Helm values:

```yaml
gloo:
  gloo:
    logLevel: error
  discovery:
    logLevel: error
  gatewayProxies:
    gatewayProxy:
      envoyLogLevel: error
```

Additionally, you can change the logging level for other services in the following Helm values example:

```yaml
global:
  extensions:
    extAuth:
      deployment:
        logLevel: error
    rateLimit:
      deployment:
        logLevel: error
    caching:
      deployment:
        logLevel: error
observability:
  deployment:
    logLevel: error
```

### Dev Mode and Gloo Debug Endpoint
In non-production environments `settings.devMode` can be set to `true` to enable a debug endpoint on the gloo deployment on port `10010`. If this flag set at install time, the port will be exposed automatically. To set it on an existing installation:
* Enable in the settings CR:
```
spec:
  devMode: true
``` 
* Expose the port in the gloo deployment CR by adding the existing list of ports in the gloo deployment image definition:
```
      - ports
        - containerPort: 10010
          name: dev-admin
          protocol: TCP
```
* Enable port forwarding:
```
kubectl port-forward -n gloo-system deployment/gloo 10010
```

The following endpoints are then available:
* `http://localhost:10010/` :   a "Hello World" type page that displays the text `Developer API`
* `http://localhost:10010/xds` : gets status keys from the xds cache
* `http://localhost:10010/xds/{key}` : gets the snapshot of the object referred to by  key from the xds cache
* `http://localhost:10010/api ` : gets the latest ApiSnapshot


### All else fails

Again, if all else fails, you can capture the state of Gloo Edge configurations and logs and join us on our Slack (https://slack.solo.io) and one of our engineers will be able to help:

```bash
glooctl debug logs -f gloo-logs.log
glooctl debug yaml -f gloo-yamls.yaml
```

These commands dump all of the relevant configuration into `gloo-logs.log` and `gloo-yamls.yaml` files, which gives a complete picture of your Gloo Edge deployment. 
