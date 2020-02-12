---
title: Gloo MTLS mode
weight: 25
description: Gloo MTLS is a way to ensure that communications between Gloo and Envoy is secure. This is useful if your control-plane is in a different environment than your envoy instance.
---

{{% notice note %}}
This feature was introduced in version 1.3.4 of Gloo and version 1.3.1 of Gloo Enterprise.
If you are using earlier versions of Gloo, this feature will not be available.
{{% /notice %}}

### Architecture

Gloo and Envoy communicate through the [xDS protocol](https://www.envoyproxy.io/docs/envoy/latest/api-docs/xds_protocol#streaming-grpc-subscriptions)
Essentially, Envoy reaches out to Gloo and sets up an open line of communication where it gets the configuration.
As long as this communication is done with TLS, the config secrets will not be in plaintext.

Essentially, we’re implementing a poor man’s service mesh here.

We can set this up by telling Envoy to initialize the connection using TLS. Then, we need to tell Gloo to answer that
communication with the TLS protocol. We do this by attaching an envoy sidecar to the gloo pod to do TLS termination.

For Gloo Enterprise users, the extauth and rate-limiting servers also need to communicate with Gloo
in order to get configuration. These pods will now start up a gRPC connection with additional TLS credentials. 
 
### Helm values

It is possible to skip the manual installation phase by passing in the following helm-override.yaml file.

`glooctl install gateway --values helm-override.yaml`

```yaml
global:
  glooMtls:
    enabled: true
```

#### Gloo MTLS Cert generation

The first step is to create a kubernetes Secret object of type 'kubernetes.io/tls'. If you installed with the Helm
override flag, then a Job is created to automatically generate the 'gloo-mtls-certs' Secret for you. The secret object
should look like:

```yaml
apiVersion: v1
data:
  ca.crt: <secret>
  tls.crt: <secret>
  tls.key: <secret>
kind: Secret
metadata:
  name: gloo-mtls-certs
  namespace: gloo-system
type: kubernetes.io/tls
```

#### Gloo Deployment

In our Gloo Deployment, we add two sidecars: the envoy sidecar and the SDS sidecar.

The purpose of the envoy sidecar is to do TLS termination on the default gloo xdsBindAddr (0.0.0.0:9977) with something
that accepts and validates a TLS connection.

In the gloo deployment, this sidecar is added as:
 
```yaml
      - env:
        - name: ENVOY_SIDECAR
          value: "true"
        name: envoy-sidecar
        image: "quay.io/solo-io/gloo-envoy-wrapper:1.3.4"
        imagePullPolicy: IfNotPresent
        ports:
        - containerPort: 9977
          name: grpc-xds
          protocol: TCP
        readinessProbe:
          tcpSocket:
            port: 9977
          initialDelaySeconds: 1
          periodSeconds: 2
          failureThreshold: 10
        volumeMounts:
        - mountPath: /etc/envoy/ssl
          name: gloo-mtls-certs
          readOnly: true
```

Note that we move the 'containerPort: 9977' stanza and the 'readinessProbe' stanza away from the gloo container, so we
need to delete those sections as well.

SDS stands for [secret discovery service](https://www.envoyproxy.io/docs/envoy/latest/configuration/security/secret), a
new feature in Envoy that allows you to rotate certs without needing to restart envoy.

In the gloo deployment, this sidecar is added as:
 
```yaml
      - name: sds
        image: "quay.io/solo-io/sds:1.3.4"
        imagePullPolicy: IfNotPresent
        volumeMounts:
        - mountPath: /etc/envoy/ssl
          name: gloo-mtls-certs
          readOnly: true
```

And finally, we add the 'gloo-mtls-certs' secret to the volumes, so that it is accessible:

```yaml
      volumes:
      - name: gloo-mtls-certs
        secret:
          defaultMode: 420
          secretName: gloo-mtls-certs
```

#### Gloo Settings

We will also need to edit the default settings CRD and change the gloo.xdsBindAddr to only listen to incoming requests
from localhost.

`k edit settings.gloo.solo.io -n gloo-system default -oyaml`

{{< highlight yaml "hl_lines=2" >}}
  gloo:
    xdsBindAddr: 127.0.0.1:9999
{{< /highlight >}}

The address 127.0.0.1 binds all incoming connections to Gloo to localhost. This ensures that only the envoy
sidecar can connect to the Gloo, but not any other malicious sources.

The Gloo Settings CR gets picked up automatically within ~5 seconds, so there’s no need to restart the Gloo pod.


#### Gateway Proxy
We need to edit the gateway-proxy pod and tell Envoy to initialize the connection to Gloo using TLS.

First we edit the configmap:

`k edit cm -n gloo-system gateway-proxy-envoy-config`

{{< highlight yaml "hl_lines=2-13" >}}
    clusters:
      - name: gloo.gloo-system.svc.cluster.local:9977
        transport_socket:
          name: envoy.transport_sockets.tls
          typed_config:
            "@type": type.googleapis.com/envoy.api.v2.auth.UpstreamTlsContext
            common_tls_context:
              tls_certificates:
              - certificate_chain: { "filename": "/etc/envoy/ssl/tls.crt" }
                private_key: { "filename": "/etc/envoy/ssl/tls.key" }
              validation_context:
                trusted_ca:
                  filename: /etc/envoy/ssl/tls.crt
{{< /highlight >}}

Then we edit the gateway-proxy deployment to provide the certs to the pod.

{{< highlight yaml "hl_lines=4-6 13-16" >}}
        volumeMounts:
        - mountPath: /etc/envoy
          name: envoy-config
        - mountPath: /etc/envoy/ssl
          name: gloo-mtls-certs
          readOnly: true
...
      volumes:
      - configMap:
          defaultMode: 420
          name: gateway-proxy-envoy-config
        name: envoy-config
      - name: gloo-mtls-certs
        secret:
          defaultMode: 420
          secretName: gloo-mtls-certs
{{< /highlight >}}

#### Extauth Server

To make our default extauth server work with MTLS, we need to edit the extauth deployment:

`k edit -n gloo-system deploy/extauth`

Add the following environment variable:

{{< highlight yaml "hl_lines=2-3" >}}
        env:
        - name: GLOO_MTLS
          value: "true"
{{< /highlight >}}

Then, add the certs to the volumes section and mount it in the extauth container

{{< highlight yaml "hl_lines=2-4 7-10" >}}
        volumeMounts:
        - mountPath: /etc/envoy/ssl
          name: gloo-mtls-certs
          readOnly: true
...
      volumes:
      - name: gloo-mtls
        secret:
          defaultMode: 420
          secretName: gloo-mtls
{{< /highlight >}}

#### Rate-limiting Server

To make our default extauth server work with MTLS, we need to edit the extauth deployment:

`k edit -n gloo-system deploy/extauth`

Add the following environment variable:

{{< highlight yaml "hl_lines=2-3" >}}
        env:
        - name: GLOO_MTLS
          value: "true"
{{< /highlight >}}

Then, add the certs to the volumes section and mount it in the extauth container

{{< highlight yaml "hl_lines=2-4 7-10" >}}
        volumeMounts:
        - mountPath: /etc/envoy/ssl
          name: gloo-mtls-certs
          readOnly: true
...
      volumes:
      - name: gloo-mtls
        secret:
          defaultMode: 420
          secretName: gloo-mtls
{{< /highlight >}}


### Cert Rotation

Cert rotation can be done by updating the gloo-mtls-certs secret.
