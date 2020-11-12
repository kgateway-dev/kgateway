---
title: Setting up Upstream TLS with Service Annotations
menuTitle: TLS with Service Annotations
weight: 30
description: Set up Gloo Edge to route to TLS-encrypted services using Kubernetes Service object annotations
---

## Motivation

Gloo Edge can auto-discover SSL configuration for [upstream TLS connections]({{< versioned_link_path fromRoot="/guides/security/tls/client_tls/">}}) using annotations on the Kubernetes Service.

This can be used as a convenient alternative to the {{% protobuf display="Upstream" name="gloo.solo.io.Upstream" %}} to configure Upstream or Client SSL.

This document explains the options for configuring SSL using service annotations. For a step-by-step guide illustrating Upstream SSL in Gloo Edge, see [the Upstream SSL Guide]({{< versioned_link_path fromRoot="/guides/security/tls/client_tls/">}})

## Configuring Upstream SSL Using Kubernetes Secrets

To use a Kubernetes TLS secret for Upstream TLS, set the annotations of your service like so:

{{< highlight yaml "hl_lines=4-5" >}}
apiVersion: v1
kind: Service
metadata:
  annotations:
    gloo.solo.io/sslService.secret: upstream-tls
  name: example-tls-server
  namespace: default
spec:
  clusterIP: 10.7.244.103
  ports:
  - port: 8080
    protocol: TCP
    targetPort: 8080
  selector:
    app: example-tls-server
  type: ClusterIP
{{< /highlight >}}


{{% notice note %}}
Note: The secret must live in the same namespace as the service.
{{% /notice %}}

## Configuring Upstream SSL Using Files Mounted to the Proxy

To certs mounted to the proxy pod (named `gateway-proxy` by default) for Upstream TLS, set the annotations of your service like so:

{{< highlight yaml "hl_lines=4-7" >}}
apiVersion: v1
kind: Service
metadata:
  annotations:
    gloo.solo.io/sslService.tlsCert: /tls.crt
    gloo.solo.io/sslService.tlsKey: /tls.key
    gloo.solo.io/sslService.rootCa: /ca.crt
  name: example-tls-server
  namespace: default
spec:
  clusterIP: 10.7.244.103
  ports:
  - port: 8080
    protocol: TCP
    targetPort: 8080
  selector:
    app: example-tls-server
  type: ClusterIP
{{< /highlight >}}


{{% notice note %}}
Note: The certificates must be mounted to the proxy pod (named `gateway-proxy` by default) with the paths specified in the annotations.
{{% /notice %}}


## Configuring Upstream SSL for a Specific Port on a Service

A service may have more than one port, where only a specific port is serving SSL.

In this case, it's necessary to include the SSL port in the annotation value, like so:


{{< highlight yaml "hl_lines=5-8" >}}
apiVersion: v1
kind: Service
metadata:
  annotations:
    # configure Upstream SSL for routes to port 443 of our service
    gloo.solo.io/sslService.tlsCert: /443:tls.crt
    gloo.solo.io/sslService.tlsKey: /443:tls.key
    gloo.solo.io/sslService.rootCa: /443:ca.crt
  name: web
  namespace: default
spec:
  clusterIP: 10.7.244.103
  ports:
  - port: 80
    protocol: TCP
    targetPort: 80
  - port: 443
    protocol: TCP
    targetPort: 443
  selector:
    app: web
  type: ClusterIP
{{< /highlight >}}


{{% notice note %}}
Note: You can also specify `<port>:<secret>` for the `gloo.solo.io/sslService.secret` annotation.
{{% /notice %}}
