---
title: Additional Authorization servers (Enterprise)
weight: 50
description: Configure multiple External Authorization servers. Decide which one to use at the route level.
---

Gloo Edge Enterprise comes with an External Authorization service, with full-featured security policies, like OpenID Connect, Open Policy Agent, OAuth 2 scopes and a few more. Also, using the so-called **passthrough** system, you can extend the authorization logic and call your own authentication or authorization service.

There are certain cases where it's more relevant to directly plug the `ext_authz` Envoy filter to your own authorization service. This article describes how to configure such connections between the Envoy data-plane and third-party authorization services.

## Calling an external authorization service

Say you are running an internal access management service, that is in charge of returning new claims, like the `role` of the user who is authenticated.
With Gloo Edge Enterprise, there are two - non-exclusive - ways of calling such an external service:
- Option A: use the **Passthrough Auth** [plugin](http://localhost:1313/guides/security/auth/extauth/passthrough_auth/), which comes with the ExtAuth options (see `AuthConfig > passthrough`)
- Option B: define an additional ExtAuth server, using `namedExtAuth` and call it on the relevant routes

![Calling an external authorization service](./two-options-external-authz-service.png)

While there is an extra network hop in the first case, you will be able to leverage the `AuthConfig` CR, and also to easily pass information between the authN/Z steps. See below for more information.

### Option A - Using the passthrough system

With Option A, you can compose your authentication and authorization workflow with the `AuthConfig` Custom Resource.
For instance, you can have a block that leverages the builtin OpenID Connect plugin and another block where you define how to call your external authorization service. 

Below is an common use case where Gloo achieves the following steps:
- use OIDC to authenticate the end-user
- use the passthrough system to populate the request with an additional JWT having authorization-centered claims (roles, etc.)
- use the JWT policy to verify the signature of the newly created JWT

![Compose your AuthN + AuthZ security workflow](./authconfig-oidc-and-passthrough.png)

Also, note you can pass values between the `AuthConfig` blocks using the `State` map. More info in this [section](https://docs.solo.io/gloo-edge/latest/guides/security/auth/extauth/passthrough_auth/grpc/#sharing-state-with-other-auth-steps).

### Option B - Using namedExtAuth

With this new option (available from version 1.9), you can define additional ExtAuthZ servers in the Gloo Edge settings.
For that, you must register your authorization servers in the "default" `Settings` custom resource. See the [API reference](https://docs.solo.io/gloo-edge/latest/reference/api/github.com/solo-io/gloo/projects/gloo/api/v1/settings.proto.sk/#settings).

Below is an example:

{{< highlight bash "hl_lines=9-15" >}}
kubectl -n gloo-system patch st/default --type merge -p "
spec:
  extauth:
    extauthzServerRef:
      name: extauth
      namespace: gloo-system
    transportApiVersion: V3
    userIdHeader: x-user-id
  namedExtauth:
    customHttpAuthz: # custom name
      extauthzServerRef: # define where is running the third-party authorization server
        name: default-http-echo-8080 # this is an Upstream CR. You must create it.
        namespace: gloo-system
      httpService: # this option enables communication over HTTP instead of gRPC (which is the default)
        pathPrefix: /
"
{{< /highlight >}}

A more advanced use case is shown in the schema below. There are two additional ExtAuth services added to Gloo's configuration.

<figure><img src="{{% versioned_link_path fromRoot="/guides/security/auth/multi_authz/namedextauth-use-case.png" %}}">
<figcaption style="text-align:center;font-style:italic">NamedExtAuth - click and zoom in to see the details</figcaption></figure>

Gloo users can decide which ExtAuth service they want to call on a per-route level, using `customAuth`.
Example:

{{< highlight yaml "hl_lines=18-21" >}}
apiVersion: gateway.solo.io/v1
kind: VirtualService
metadata:
  name: httpbin
  namespace: gloo-system
spec:
  virtualHost:
    domains:
    - '*'
    routes:
    - matchers:
      - prefix: /
      routeAction:
        single:
          upstream:
            name: default-httpbin-8000
            namespace: gloo-system
      options:
        extauth:
          customAuth: # set the authorization system at the route level
            name: customHttpAuthz # one of the `namedExtAuth` 
{{< /highlight >}}

## The gRPC specification

Be it with the **Passthrough Auth** option or with the **namedExtAuth** option, you must conform to the Envoy specification for the external Authorization service: https://github.com/envoyproxy/envoy/blob/main/api/envoy/service/auth/v3/external_auth.proto

There are examples in this GitHub repository: https://github.com/solo-io/gloo/tree/master/docs/examples/grpc-passthrough-auth/pkg/auth/v3

## Managing headers

If you want to add new headers to the request from your external authorization service, this is possible.

### gRPC mode

In the case of gRPC, you will rely on the [protobuf specification](https://github.com/envoyproxy/envoy/blob/main/api/envoy/service/auth/v3/external_auth.proto#L76) and you need to populate the `OkHttpResponse` with the new headers. They will be visible to the rest of the filter chain and they will be passed to the upstream service.

### HTTP mode

With the **Passthrough Auth** option, say you have several config blocks defined in your `AuthConfig` CR and you want the step(s) after `passthrough` to be passed values, then you must use the `State` map. 

With both the **HTTP passthrough** option and the **namedExtAuth** with HTTP option, if you want to read new headers in the rest of the filter chain or in the upstream service, then just add them to the authorization request and they will be merged into the original request before being forwarded upstream.

You also decide which headers are allowed to go upstream and which are not. Under the `httpService` option, you can define some rules about headers you want to forward to the external authorization service, and also rules to sanitize headers before forwarding the request upstream.

