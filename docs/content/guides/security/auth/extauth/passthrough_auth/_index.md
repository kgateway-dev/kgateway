---
title: Passthrough Auth
weight: 10
description: Authenticating using an external "passthrough" service. 
---

When using Gloo Gateway's external authentication server, it may be convenient to authorize requests with your own server. Gloo Gateway allows this functionality with two types of passthrough auth:

{{% children description="true" %}}

## Passthrough auth vs. Custom Auth vs. Custom Extauth plugin
You can also implement your own auth with Gloo Gateway with a [Custom Auth server]({{< versioned_link_path fromRoot="/guides/security/auth/custom_auth" >}}) or an [Extauth plugin]({{< versioned_link_path fromRoot="/guides/security/auth/extauth/plugin_auth" >}}). 

**Passthrough vs. Custom Auth server**
With passthrough, you can leverage other Gloo Gateway extauth implementations (e.g. OIDC, API key, etc.) alongside custom logic. A custom auth server is not integrated with Gloo Gateway extauth so it can not do this.

**Passthrough vs. Extauth plugin**
Using Gloo Gateway to passthrough requests to a separate authentication component eliminates the need to recompile extauth plugins with each version of Gloo Gateway Enterprise.

**Passthrough Cons**
While using a Passthrough service does provide additional flexibility and convenience with auth configuration, it does require an additional network hop from Gloo Gateway's external auth service to the gRPC service.