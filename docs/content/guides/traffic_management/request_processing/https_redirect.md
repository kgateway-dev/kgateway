---
title: HTTPS Redirect
weight: 20
description: Route HTTP traffic to HTTPS services
---

To help users or old services find your https endpoints, it is a common practice to redirect http traffic to https endpoints.
With Gloo Edge, this is a simple matter of creating an auxiliary http virtual service that routes to your full
{{% protobuf
display="https virtual service" 
name="gateway.solo.io.VirtualService"
%}}

Here is an example redirect config (which is used to host these docs). Please take note of a few details which enable this behavior:

- There are two virtual services, an "auxiliary" and a "main" virtual service.
  - The main VS is an https VS because it specifies an
{{% protobuf
name="gloo.solo.io.SslConfig"
display="sslConfig"
%}}
  - The auxiliary VS is an http VS because it does not specify an
{{% protobuf
name="gloo.solo.io.SslConfig"
display="sslConfig"
%}}
- The domain listed in the auxiliary VS matches that of the main VS.
- The auxiliary VS has a single route which matches all traffic and applies a
{{% protobuf
display="redirectAction" 
name="gloo.solo.io.RedirectAction"
%}} with `httpsRedirect: true` specified.


{{< highlight yaml "hl_lines=9-10 14-16 25-30 32-33" >}}
# auxiliary virtual service to redirect http traffic to the full https virtual service
apiVersion: gateway.solo.io/v1
kind: VirtualService
metadata:
  name: docsmgmt-http
  namespace: docs
spec:
  virtualHost:
    domains:
    - docs.solo.io
    routes:
    - matchers:
      - prefix: /
      redirectAction:
        hostRedirect: docs.solo.io
        httpsRedirect: true
---
# main virtual service
apiVersion: gateway.solo.io/v1
kind: VirtualService
metadata:
  name: docsmgmt
  namespace: docs
spec:
  sslConfig:
    secretRef:
      name: docs.solo.io
      namespace: gloo-system
    sniDomains:
    - docs.solo.io
  virtualHost:
    domains:
    - 'docs.solo.io'
    routes:
    - matchers:
# (put the https routes here)
{{< /highlight >}}
