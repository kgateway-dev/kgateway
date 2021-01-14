---
title: Passthrough Auth
weight: 10
description: Authenticating using an external grpc service that implements [Envoy's Authorization Service API](https://www.envoyproxy.io/docs/envoy/latest/intro/arch_overview/security/ext_authz_filter.html?highlight=authorization%20service#service-definition). 
---

{{% notice note %}}
The Passthrough feature was introduced with **Gloo Edge Enterprise**, release 1.6.0. If you are using an earlier version, this tutorial will not work.
{{% /notice %}}

When using Gloo Edge's external authentication server, it may be convenient to integrate authentication with a component that implements [Envoy's authorization service API](https://www.envoyproxy.io/docs/envoy/latest/intro/arch_overview/security/ext_authz_filter.html?highlight=authorization%20service#service-definition). This guide will walk through the process of setting up Gloo Edge's external authentication server to pass through requests to the provided component for authenticating requests. 
If you do not wish to use the external authentication server provided in the enterprise version of Gloo Edge, you can also configure gloo to work with your own [Custom Auth server]({{< versioned_link_path fromRoot="/guides/security/auth/custom_auth" >}}).

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
Currently, Gloo Edge's external authentication server only supports passthrough requests to a gRPC server. The server must implement the Envoy authorization service API. For more information, view the service spec in the [official docs](https://github.com/envoyproxy/envoy/blob/master/api/envoy/service/auth/v2/external_auth.proto).



## Creating a Virtual Service
Now let's configure Gloo Edge to route requests to the upstream we just created. To do that, we define a simple Virtual 
Service to match all requests that:

- contain a `Host` header with value `foo` and
- have a path that starts with `/` (this will match all requests).

Apply the following virtual service:
{{< readfile file="guides/security/auth/extauth/basic_auth/test-no-auth-vs.yaml" markdown="true">}}

Let's send a request that matches the above route to the Gloo Edge gateway and make sure it works:

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

# Securing the Virtual Service 
As we just saw, we were able to reach the upstream without having to provide any credentials. This is because by default 
Gloo Edge allows any request on routes that do not specify authentication configuration. Let's change this behavior. 
We will update the Virtual Service so that all requests will be authenticated by our own auth service.
We can do that as follows:

{{< highlight shell "hl_lines=11-13" >}}
kubectl apply -f - <<EOF
apiVersion: enterprise.gloo.solo.io/v1
kind: AuthConfig
metadata:
  name: passthrough-auth
  namespace: gloo-system
spec:
  configs:
  - passThroughAuth:
      # As of Gloo Edge v1.6.1, grpc is the only pass through auth method supported
      grpc:
        # Address of the grpc auth server to query
        address: <<server address here>>
EOF
{{< /highlight >}}

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
inherit its `AuthConfig`, unless it [overwrites or disables]({{< versioned_link_path fromRoot="/guides/security/auth#inheritance-rules" >}}) it.

### Logging

If Gloo Edge is running on kubernetes, the extauth server logs can be viewed with:
```
kubectl logs -n gloo-system deploy/extauth -f
```
If the auth config has been received successfully, you should see the log line:
```
"logger":"extauth","caller":"runner/run.go:179","msg":"got new config"
```

## Summary

In this guide, we installed Gloo Edge Enterprise and created an unauthenticated Virtual Service that routes requests to a static upstream. We then created an `AuthConfig` and configured it to use Passthrough Auth. In doing so, we instructed gloo to pass through requests from the external authentication server to the grpc authentication service provided by the user.