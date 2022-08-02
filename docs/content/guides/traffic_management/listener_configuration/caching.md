---
title: Caching responses
weight: 50
description: Cache responses from upstream services.
---

Cache responses from upstream services by deploying a caching server for your Gloo Edge setup and applying a caching filter to a listener.

{{% notice note %}}
This feature is available only for Gloo Edge Enterprise v1.12.x and later.
{{% /notice %}}

The Gloo Edge Enterprise caching filter is an extension (implementing filter) of the [Envoy cache filter](https://www.envoyproxy.io/docs/envoy/latest/start/sandboxes/cache) and takes advantage of all the cache-ability checks that are applied. However, Gloo Edge also provides the ability to store the cached objects in a Redis instance, including Redis configuration options such as setting a password.

When you enable caching during installation, the caching server deployment is automatically created for you and is managed by Gloo Edge. Then, you can configure an HTTP or HTTPS listener to cache responses for its upstream services. When the listener routes a request comes to an upstream, the response from the upstream is automatically cached for one hour. All subsequent requests receive the cached response.

## Deploy the caching server

Create a caching server during Gloo Edge Enterprise installation time, and specify any Redis overrides. 

1. [Install Gloo Edge Enterprise version 1.12.x or later by using Helm]({{% versioned_link_path fromRoot="/installation/enterprise/#customizing-your-installation-with-helm" %}}). In your `values.yaml` file, specify the following settings:
   * Caching server: Set `global.extensions.caching.enabled: true` to enable the caching server deployment.
   * Redis overrides: By default, the caching server uses the Redis instance that is deployed with Gloo Edge. To use your own Redis instance, such as in production deployments:
     * Set `redis.disabled` to `true` to disable the default Redis instance.
     * Set `redis.service.name` to the name of the Redis service instance. If the instace is an external service, set the endpoint of the external service as the value.
     * For other Redis override settings, see the Redis section of the [Enterprise Helm chart values]({{% versioned_link_path fromRoot="/reference/helm_chart_values/enterprise_helm_chart_values/" %}}).

2. Verify that the caching server is deployed.
   ```sh
   kubectl --namespace gloo-system get all | grep caching
   ```

<!-- future work
## Configure settings for the caching server

should be able to configure general settings for the server in the future, like the default caching time

https://docs.solo.io/gloo-edge/master/reference/api/github.com/solo-io/gloo/projects/gloo/api/v1/enterprise/options/caching/caching.proto.sk/#settings
-->

## Configure caching for a listener

In the Gateway CRD where your listener is defined, specify the caching server in the `httpGateway.options` section. Currently, all paths for all upstreams that are served by a listener are cached.

{{< highlight yaml "hl_lines=11-16" >}}
apiVersion: gateway.solo.io/v1
kind: Gateway
metadata:
  name: gateway-proxy
  namespace: gloo-system
spec:
  bindAddress: ‘::’
  bindPort: 8080
  proxyNames:
  - gateway-proxy
  httpGateway:
    options:
      caching:
        cachingServiceRef:
          name: caching-server
          namespace: gloo-system
    ...
{{< /highlight >}}

<!-- future work: define matchers to specify which paths should be cached -->

## Verify caching

Verify that responses are now cached.

1. Edit your gateway definition to configure the `gateway-proxy` deployment to log the value of the `test-header` request header.
   {{< highlight yaml "hl_lines=18-23" >}}
   kubectl apply -f- <<EOF
   apiVersion: gateway.solo.io/v1
   kind: Gateway
   metadata:
     name: gateway-proxy
     namespace: gloo-system
   spec:
     bindAddress: '::'
     bindPort: 8080
     proxyNames:
     - gateway-proxy
     httpGateway:
       options:
         caching:
           cachingServiceRef:
             name: caching-server
             namespace: gloo-system
     options:
       accessLoggingService:
         accessLog:
         - fileSink:
             stringFormat: "test-header: %REQ(test-header)%\n"
             path: /dev/stdout
   EOF
   {{< /highlight >}}

2. Send a request to an upstream service that the listener routes to. For example, if you followed the [Hello World guide]({{% versioned_link_path fromRoot="/guides/traffic_management/hello_world/" %}}) to deploy a sample app and configure routing to it, send the following curl request. The `test-value` value is specified for the `test-header` request header.
   ```sh
   curl $(glooctl proxy url)/all-pets -H test-header:test-value
   ```
   The response returned by the upstream, such as the following example, should now be cached.
   ```json
   [{"id":1,"name":"Dog","status":"available"},{"id":2,"name":"Cat","status":"pending"}]
   ```

3. Get the access logs from the `gateway-proxy` deployment.
   ```shell
   kubectl logs deployment/gateway-proxy -n gloo-system
   ```
   Verify that you see the following log entry:
   ```
   test-header: test-value
   ```

4. Send another request to the service.
   ```sh
   curl $(glooctl proxy url)/all-pets -H test-header:test-value
   ```

5. Check the access logs again from the `gateway-proxy` deployment. This time, no new log entries are generated, because the gateway returns a cached response instead of routing the request to the upstream and returning the response.
   ```shell
   kubectl logs deployment/gateway-proxy -n gloo-system
   ```