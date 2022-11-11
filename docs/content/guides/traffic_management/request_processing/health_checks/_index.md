---
title: Health Checks
weight: 50
description: Enable a health check plugin to respond with common HTTP codes
---

Gloo Edge includes an HTTP health checking plug-in that you can enable in a {{< protobuf display="Gateway" name="gateway.solo.io.Gateway" >}} (which becomes an [Envoy Listener](https://www.envoyproxy.io/docs/envoy/latest/configuration/listeners/listeners)). This plug-in responds to health check requests directly with either a `200 OK` or `503 Service Unavailable` message, depending on the current draining state of Envoy.

**Before you begin**: To activate a health check endpoint on a Gateway, you must first configure a virtual service. For example, you can follow one of the [destination selection guides]({{% versioned_link_path fromRoot="/guides/traffic_management/destination_selection/" %}}) to create a virtual service.
 
1. Edit the gateway proxy that you want to add the health check to. **Note**: If you use TLS termination in Gloo Edge, use the `gateway-proxy-ssl` tab to configure the Gateway Proxy SSL.
   {{< tabs >}} 
{{% tab name="gateway-proxy" %}}
```shell
kubectl --namespace gloo-system edit gateway gateway-proxy
```
{{% /tab %}} 
{{% tab name="gateway-proxy-ssl" %}}
```shell
kubectl --namespace gloo-system edit gateway gateway-proxy-ssl
```
{{% /tab %}} 
   {{< /tabs >}}

2. Add the `healthCheck` stanza to the `spec.options` section of the gateway. Note that the HTTP path of any health check requests must be *exact* matches to the value of `healthCheck.path`.
   {{< tabs >}} 
{{% tab name="gateway-proxy" %}}
```yaml
apiVersion: gateway.solo.io/v1
kind: Gateway
metadata:
  name: gateway-proxy
  namespace: gloo-system
spec:
  bindAddress: '::'
  bindPort: 8080
  httpGateway:
    options:
      healthCheck:
        path: /any-path-you-want
```
{{% /tab %}} 
{{% tab name="gateway-proxy-ssl" %}}
```yaml
apiVersion: gateway.solo.io/v1
kind: Gateway
metadata:
  labels:
    app: gloo
  name: gateway-proxy-ssl
  namespace: gloo-system
spec:
  bindAddress: '::'
  bindPort: 8443
  httpGateway:
    options:
      healthCheck:
        path: /any-path-you-want
  proxyNames:
  - gateway-proxy
  ssl: true
  useProxyProto: false
```
{{% /tab %}} 
   {{< /tabs >}}

3. To test the health check, drain the Envoy connections by sending an `HTTP POST` request to the Envoy admin port on `<envoy-ip>:<admin-addr>/healthcheck/fail`. This port defaults to `19000`.
4. Send a request to the health check path. Because Envoy is in a draining state, the `503 Service Unavailable` message is returned.
   ```shell
   curl $(glooctl proxy url)/<path>
   ```