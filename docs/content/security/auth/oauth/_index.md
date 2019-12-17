---
title: OAuth
weight: 20
description: External Auth with Oauth
---

{{% notice note %}}
{{< readfile file="static/content/enterprise_only_feature_disclaimer" markdown="true">}}
{{% /notice %}}

Gloo supports authentication via **OpenID Connect (OIDC)**. OIDC is an identity layer on top of the **OAuth 2.0** protocol. 
In OAuth 2.0 flows, authentication is performed by an external **Identity Provider** (IdP) which, in case of success, 
returns an **Access Token** representing the user identity. The protocol does not define the contents and 
structure of the Access Token, which greatly reduces the portability of OAuth 2.0 implementations.

The goal of OIDC is to address this ambiguity by additionally requiring Identity Providers to return a well-defined 
**ID Token**. OIDC ID tokens follow the [JSON Web Token (JWT)]({{< ref "security/auth/jwt" >}}) 
standard and contain specific fields that your applications can expect and handle. This standardization allows you to
switch between Identity Providers - or support multiple ones at the same time - with minimal, if any, changes to your 
downstream services; it also allows you to consistently apply additional security measures like _Role-based Access Control (RBAC)_ 
based on the identity of your users, i.e. the contents of their ID token 
(check out [this guide]({{< ref "security/auth/jwt/access_control" >}}) for an example of how to 
use Gloo to apply RBAC policies to JWTs). 

In this guide, we will focus on the format of the Gloo API for OIDC authentication.

## Configuration format
{{% notice warning %}}
{{% extauth_version_info_note %}}
{{% /notice %}}

Following is an example of an `AuthConfig` with an OIDC configuration (for more information on `AuthConfig` CRDs, see 
the [main page]({{< versioned_link_path fromRoot="/security/auth/#auth-configuration-overview" >}}) 
of the authentication docs):

{{< highlight yaml "hl_lines=8-15" >}}
apiVersion: enterprise.gloo.solo.io/v1
kind: AuthConfig
metadata:
  name: oidc
  namespace: gloo-system
spec:
  configs:
  - oauth:
      issuer_url: theissuer.com
      app_url: myapp.com
      callback_path: /my/callback/path/
      client_id: myclientid
      client_secret_ref:
        name: my-oauth-secret
        namespace: gloo-system
      scopes:
      - email
{{< /highlight >}}

The `AuthConfig` consists of a single `config` of type `oauth`. Let's go through each of its attributes:

- `issuer_url`: The url of the OpenID Connect identity provider. Gloo will automatically discover OpenID Connect 
configuration by querying the `.well-known/open-configuration` endpoint on the `issuer_url`. For example, if you are 
using Google as an identity provider, Gloo will expect to find OIDC discovery information at 
`https://accounts.google.com/.well-known/openid-configuration`.
- `app_url`: This is the public URL of your application. It is used in combination with the `callback_path` attribute.
- `callback_path`: The callback path relative to the `app_url`. Once a user has been authenticated, the identity provider 
will redirect them to this URL. Gloo will intercept requests with this path, exchange the authorization code received from 
Dex for an ID token, place the ID token in a cookie on the request, and forward the request to its original destination.
- `client_id`: This is the **client id** that you obtained when you registered your application with the identity provider.
- `client_secret_ref`: This is a reference to a Kubernetes secret containing the **client secret** that you obtained 
when you registered your application with the identity provider. The easiest way to create the Kubernetes secret in the 
expected format is to use `glooctl`, but you can also provide it by `kubectl apply`ing YAML to your cluster:
{{< tabs >}}
{{< tab name="glooctl" codelang="shell">}}
glooctl create secret oauth --namespace gloo-system --name oidc --client-secret secretvalue
{{< /tab >}}
{{< tab name="kubectl" codelang="yaml">}}
apiVersion: v1
kind: Secret
type: Opaque
metadata:
  annotations:
    resource_kind: '*v1.Secret'
  name: oidc
  namespace: gloo-system
data:
  # The value is a base64 encoding of the following YAML:
  # client_secret: secretvalue
  # Gloo expects OAuth client secrets in this format.
  oauth: Y2xpZW50U2VjcmV0OiBzZWNyZXR2YWx1ZQo=
{{< /tab >}}
{{< /tabs >}} 
- `scopes`: scopes to request in addition to the `openid` scope.

## Examples
We have seen how a sample OIDC `AuthConfig` is structured. For complete examples of how to set up an OIDC flow with 
Gloo, check out the following guides:

{{% children description="true" %}}