---
title: gRPC Passthrough Auth
weight: 10
description: Authenticating using an external grpc service that implements [Envoy's Authorization Service API](https://www.envoyproxy.io/docs/envoy/latest/intro/arch_overview/security/ext_authz_filter.html). 
---

{{% notice note %}}
The gRPC Passthrough feature was introduced with **Gloo Gateway Enterprise**, release 1.6.0. If you are using an earlier version, this tutorial will not work.
{{% /notice %}}

When using Gloo Gateway's external authentication server, it may be convenient to integrate authentication with a component that implements [Envoy's authorization service API](https://www.envoyproxy.io/docs/envoy/latest/intro/arch_overview/security/ext_authz_filter.html?highlight=authorization%20service#service-definition). This guide will walk through the process of setting up Gloo Gateway's external authentication server to pass through requests to the provided component for authenticating requests. 

Make sure to check out the pros and cons of using passthrough auth [here]({{< versioned_link_path fromRoot="/guides/security/auth/extauth/passthrough_auth" >}})

## Setup
{{< readfile file="/static/content/setup_notes" markdown="true">}}

Let's start by creating a [Static Upstream]({{< versioned_link_path fromRoot="/guides/traffic_management/destination_types/static_upstream/" >}}) 
that routes to a website; we will send requests to it during this tutorial.

{{< tabs >}}
{{< tab name="kubectl" codelang="yaml">}}
{{< readfile file="/static/content/upstream.yaml">}}
{{< /tab >}}
{{< tab name="glooctl" codelang="shell" >}}
glooctl create upstream static --static-hosts jsonplaceholder.typicode.com:80 --name json-upstream
{{< /tab >}}
{{< /tabs >}}

### Creating an authentication service
This authentication service will be a gRPC authentication service. For more information, view the service spec in the [official docs](https://github.com/envoyproxy/envoy/blob/main/api/envoy/service/auth/v3/external_auth.proto).

To use an example gRPC authentication service provided for this guide, run the following command to deploy the provided image. This will run a docker image that contains a simple gRPC service running on port 9001.

{{< highlight shell >}}
kubectl apply -f - <<EOF
apiVersion: apps/v1
kind: Deployment
metadata:
  name: extauth-grpcservice
spec:
  selector:
    matchLabels:
      app: grpc-extauth
  replicas: 1
  template:
    metadata:
      labels:
        app: grpc-extauth
    spec:
      containers:
        - name: grpc-extauth
          image: quay.io/solo-io/passthrough-grpc-service-example
          imagePullPolicy: IfNotPresent
          ports:
            - containerPort: 9001
EOF
{{< /highlight >}}

The source code for the gRPC service can be found in the Gloo Gateway repository [here](https://github.com/solo-io/gloo/tree/main/docs/examples/grpc-passthrough-auth).

Once we create the authentication service, we also want to apply the following Service to assign it a static cluster IP.
{{< highlight shell >}}
kubectl apply -f - <<EOF
apiVersion: v1
kind: Service
metadata:
  name: example-grpc-auth-service
  labels:
      app: grpc-extauth
spec:
  ports:
  - port: 9001
    protocol: TCP
  selector:
      app: grpc-extauth
EOF
{{< /highlight >}}

## Creating a Virtual Service
Now let's configure Gloo Gateway to route requests to the upstream we just created. To do that, we define a simple Virtual 
Service to match all requests that:

- contain a `Host` header with value `foo` and
- have a path that starts with `/` (this will match all requests).

Apply the following virtual service:
{{< readfile file="guides/security/auth/extauth/basic_auth/test-no-auth-vs.yaml" markdown="true">}}

Let's send a request that matches the above route to the gateway proxy and make sure it works:

```shell
curl -H "Host: foo" $(glooctl proxy url)/posts/1
```

The above command should produce the following output:

```json
{
  "userId": 1,
  "id": 1,
  "title": "sunt aut facere repellat provident occaecati excepturi optio reprehenderit",
  "body": "quia et suscipit\nsuscipit recusandae consequuntur expedita et cum\nreprehenderit molestiae ut ut quas totam\nnostrum rerum est autem sunt rem eveniet architecto"
}
```

If you are getting a connection error, make sure you are port-forwarding the `glooctl proxy url` port to port 8080.

# Securing the Virtual Service 
As we just saw, we were able to reach the upstream without having to provide any credentials. This is because by default 
Gloo Gateway allows any request on routes that do not specify authentication configuration. Let's change this behavior. 
We will update the Virtual Service so that all requests will be authenticated by our own auth service.

{{< highlight shell "hl_lines=9-14" >}}
kubectl apply -f - <<EOF
apiVersion: enterprise.gloo.solo.io/v1
kind: AuthConfig
metadata:
  name: passthrough-auth
  namespace: gloo-system
spec:
  configs:
  - passThroughAuth:
      grpc:
        # Address of the grpc auth server to query
        address: example-grpc-auth-service.default.svc.cluster.local:9001
        # Set a connection timeout to external service, default is 5 seconds
        connectionTimeout: 3s
EOF
{{< /highlight >}}

{{% notice note %}}
Passthrough services also allow for failing "open" through the [`failureModeAllow`]({{< versioned_link_path fromRoot="/reference/api/github.com/solo-io/gloo/projects/gloo/api/v1/enterprise/options/extauth/v1/extauth.proto.sk/#settings" >}}) field. 
By setting this field to `true`, the auth service responds with an `OK` if either your server returns a `5XX`-equivalent response or the request times out.
{{% /notice %}}

Once the `AuthConfig` has been created, we can use it to secure our Virtual Service:

{{< highlight shell "hl_lines=21-25" >}}
kubectl apply -f - <<EOF
apiVersion: gateway.solo.io/v1
kind: VirtualService
metadata:
  name: auth-tutorial
  namespace: gloo-system
spec:
  virtualHost:
    domains:
      - 'foo'
    routes:
      - matchers:
        - prefix: /
        routeAction:
          single:
            upstream:
              name: json-upstream
              namespace: gloo-system
        options:
          autoHostRewrite: true      
    options:
      extauth:
        configRef:
          name: passthrough-auth
          namespace: gloo-system
EOF
{{< /highlight >}}

In the above example we have added the configuration to the Virtual Host. Each route belonging to a Virtual Host will 
inherit its `AuthConfig`, unless it [overwrites or disables]({{< versioned_link_path fromRoot="/guides/security/auth/extauth/#inheritance-rules" >}}) it.

### Logging

If Gloo Gateway is running on kubernetes, the extauth server logs can be viewed with:
```
kubectl logs -n gloo-system deploy/extauth -f
```
If the auth config has been received successfully, you should see the log line:
```
"logger":"extauth","caller":"runner/run.go:179","msg":"got new config"
```

### Metrics

{{% notice note %}}
For more information on how Gloo Gateway handles observability and metrics, view our [observability introduction]({{< versioned_link_path fromRoot="/introduction/observability/" >}}).
{{% /notice %}}

* Failure Mode Allow
  * Metric Name: `extauth.solo.io/passthrough_failure_bypass`
  * Description: The number of times a server error or timeout occurred and was bypassed through the `failure_mode_allow=true` setting

## Testing the secured Virtual Service
The virtual service that we have created should now be secured using our external authentication service. To test this, we can try our original command, and the request should not be allowed through because of missing authentication.

```shell
curl -H "Host: foo" $(glooctl proxy url)/posts/1
```

The output should be empty. In fact, we can see the 403 (Unauthorized) HTTP status code if we run the same curl, but with a modification to print the http code to the console.

```shell
curl -s -o /dev/null -w "%{http_code}" -H "Host: foo" $(glooctl proxy url)/posts/1
```

The sample gRPC authentication service has been implemented such that any request with the header `authorization: authorize me` will be authorized. We can easily add this header to our curl request as follows:

```shell
curl -H "Host: foo" -H "authorization: authorize me" $(glooctl proxy url)/posts/1
```

The request should now be authorized!

## Configuring retries for unresponsive passthrough services

You can configure the Gloo ExtAuth server to retry the connection to the passthrough service in the case that the passthrough service becomes unavailable. Consider the following two scenarios to learn more about how retries to the passthrough service are executed: 


* **Passthrough service becomes unavailable after initial connection**: In this scenario, the Gloo ExtAuth server successfully established an initial connection to the passthrough service. However later on, the passthrough service becomes unavailable. You can add a retry policy to your AuthConfig to retry the connection to the passthrough service if the service becomes unavailable. In the following AuthConfig, the Gloo ExtAuth server is configured to retry the connection to the passthrough service 10 times. To not overload the passthrough service, an optional exponential backoff strategy is defined. The backoff strategy configures the ExtAuth server to start retries after 1 second (`baseInterval`). Retries are then executed exponentially, such as after 2 seconds, 4 seconds, 8 seconds, etc up to the `maxInterval` that defaults to 10 times the `baseInterval`. In this example, the `maxInterval` configures a maximum delay of 2 seconds between retries. Note that the global settings `global.extensions.extAuth.requestTimeout` must be greater than the `retryPolicy.numRetries` * `retryPolicy.retryBackOff.baseInterval` to ensure that the failed gRPC call has sufficient time to retry.

  For more information, see the [API docs]({{< versioned_link_path fromRoot="/reference/api/github.com/solo-io/gloo/projects/gloo/api/v1/enterprise/options/extauth/v1/extauth.proto.sk/#retrypolicy" >}}).

  {{< highlight yaml "hl_lines=13-18" >}}
apiVersion: enterprise.gloo.solo.io/v1
kind: AuthConfig
metadata:
  name: passthrough-auth
  namespace: gloo-system
spec:
  configs:
  - passThroughAuth:
      grpc:
        # Address of the grpc auth server to query
        address: example-grpc-auth-service.default.svc.cluster.local:9001
        # Set a connection timeout to external service, default is 5 seconds
        connectionTimeout: 3s
        retryPolicy: 
          numRetries: 10
          retryBackOff:
            baseInterval: 1s
            maxInterval: 2s
  {{< /highlight >}}


* **Passthrough service is unavailable during the initial connection**: When you configure your AuthConfig with a retry policy, auth requests are retried only after the initial connection between the Gloo auth server and passthrough service is established successfully. If establishing the initial connection fails, the ExtAuth server retries the connection up to the defined `connectionTimeout` in the AuthConfig. The settings in the retry policy are ignored and any auth requests that are sent to the ExtAuth server during that time fail immediately. Auth requests continue to fail when the `connectionTimeout` is reached, even if the passthrough service becomes available afterwards. To mitigate this issue, you can try increasing the `connectionTimeout` setting if you think that your passthrough service can recover and become available within the specified connection timeout.


## Sharing state with other auth steps

{{% notice note %}}
The sharing state feature was introduced with **Gloo Gateway Enterprise**, release 1.6.10. If you are using an earlier version, this will not work.
{{% /notice %}}

A common requirement is to be able to share state between the passthrough service, and other auth steps (either custom plugins, or our built-in authentication) . When writing a custom auth plugin, this is possible, and the steps to achieve it are [outlined here]({{< versioned_link_path fromRoot="/guides/dev/writing_auth_plugins#sharing-state-between-steps" >}}). We support this requirement by leveraging request and response metadata.

We provide some example implementations in the Gloo Gateway repository at `docs/examples/grpc-passthrough-auth/pkg/auth/v3/auth-with-state.go`.

### Reading state from other auth steps

State from other auth steps is sent to the passthrough service via [CheckRequest FilterMetadata](https://github.com/envoyproxy/envoy/blob/50e722cbb0486268c128b0f1d0ef76217387799f/api/envoy/service/auth/v3/external_auth.proto#L36) under a unique key: `solo.auth.passthrough`.

### Writing state to be used by other auth steps

State from the passthrough service can be sent to other auth steps via [CheckResponse DynamicMetadata](https://github.com/envoyproxy/envoy/blob/50e722cbb0486268c128b0f1d0ef76217387799f/api/envoy/service/auth/v3/external_auth.proto#L126) under a unique key: `solo.auth.passthrough`.

### Passing in custom configuration to Passthrough Auth Service from AuthConfigs
{{% notice note %}}
This feature was introduced with **Gloo Gateway Enterprise**, release 1.6.15. If you are using an earlier version, this will not work.
{{% /notice %}}

Custom config can be passed from gloo to the passthrough authentication service. This can be achieved using the `config` field under Passthrough Auth in the AuthConfig:

{{< highlight shell "hl_lines=15-17" >}}
kubectl apply -f - <<EOF
apiVersion: enterprise.gloo.solo.io/v1
kind: AuthConfig
  metadata:
    name: passthrough-auth
    namespace: gloo-system
  spec:
    configs:
    - passThroughAuth:
        grpc:
          # Address of the grpc auth server to query
          address: example-grpc-auth-service.default.svc.cluster.local:9001
          # Set a connection timeout to external service, default is 5 seconds
          connectionTimeout: 3s
      config:
        customKey1: "customConfigStringValue"
        customKey2: false
EOF
{{< /highlight >}}

This config is accessible via the [CheckRequest FilterMetadata](https://github.com/envoyproxy/envoy/blob/50e722cbb0486268c128b0f1d0ef76217387799f/api/envoy/service/auth/v3/external_auth.proto#L36) under a unique key: `solo.auth.passthrough.config`.

## Summary

In this guide, we installed Gloo Gateway Enterprise and created an unauthenticated Virtual Service that routes requests to a static upstream. We spun up an example gRPC authentication service that uses a simple header for authentication. We then created an `AuthConfig` and configured it to use Passthrough Auth, pointing it to the IP of our example gRPC service. In doing so, we instructed gloo to pass through requests from the external authentication server to the grpc authentication service provided by the user.
