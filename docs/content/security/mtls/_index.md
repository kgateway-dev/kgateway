---
title: Gloo mTLS mode
weight: 25
description: Gloo mTLS is a way to ensure that communications between Gloo and Envoy is secure. This is useful if your control-plane is in a different environment than your envoy instance.
---

{{% notice note %}}
This feature was introduced in version 1.3.6 of Gloo and version 1.3.0-beta3 of Gloo Enterprise.
If you are using earlier versions of Gloo, this feature will not be available.
{{% /notice %}}

### Motivation

Gloo and Envoy communicate through the [xDS protocol](https://www.envoyproxy.io/docs/envoy/latest/api-docs/xds_protocol#streaming-grpc-subscriptions).
Since the Envoy configuration can contain secret data, plaintext communication between Gloo and Envoy may be too insecure.
This is especially true if your setup has the Gloo control plane and Envoy instances running in separate clusters.

Turning on mTLS will encrypt the xDS communication between Gloo and Envoy.

### Enabling mTLS

It is possible to skip the manual installation phase by passing in the following helm-override.yaml file.

```yaml
global:
  glooMtls:
    enabled: true
```

Then, we run:
`glooctl install gateway --values helm-override.yaml`

This will ensure that Envoy initializes the connection to Gloo using mTLS. Gloo will now answer through a TCP proxy
that communicates with the TLS protocol. We do this by attaching an envoy sidecar to the gloo pod to do TLS termination.

For Gloo Enterprise users, the extauth and rate-limiting servers also need to communicate with Gloo
in order to get configuration. These pods will now start up a gRPC connection with additional TLS credentials.

### Detailed Explanation

This is a step by step guide to what the `global.glooMtls.enabled=true` helm value does to
the Gloo installation.

#### Secret Creation

The first step is to create a kubernetes Secret object of type 'kubernetes.io/tls'. If Gloo is installed with the Helm
override flag, a Job called 'gloo-mtls-certgen' is created to automatically generate the 'gloo-mtls-certs' Secret for you.
The secret object has the following structure:

```yaml
apiVersion: v1
data:
  ca.crt: ...
  tls.crt: ...
  tls.key: ...
kind: Secret
metadata:
  name: gloo-mtls-certs
  namespace: gloo-system
type: kubernetes.io/tls
```

#### Gloo Deployment (the xDS server)

In the gloo deployment, two sidecars are added: the envoy sidecar and the SDS sidecar.

The purpose of the envoy sidecar is to do TLS termination on the default gloo xdsBindAddr (0.0.0.0:9977) with something
that accepts and validates a TLS connection.

In the gloo deployment, this sidecar is added as:
 
```yaml
      - env:
        - name: ENVOY_SIDECAR
          value: "true"
        name: envoy-sidecar
        image: "quay.io/solo-io/gloo-envoy-wrapper:1.3.6"
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

Note that the 'containerPort: 9977' stanza and the 'readinessProbe' stanza move away from the gloo container, so
those sections need to be deleted from the gloo container.

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

Finally, the 'gloo-mtls-certs' secret is added to the volumes to make it accessible:

```yaml
      volumes:
      - name: gloo-mtls-certs
        secret:
          defaultMode: 420
          secretName: gloo-mtls-certs
```

#### Gloo Settings

The default settings CRD changes such that the gloo.xdsBindAddr will only listen to incoming requests
from localhost.

`k edit settings.gloo.solo.io -n gloo-system default -oyaml`

{{< highlight yaml "hl_lines=2" >}}
  gloo:
    xdsBindAddr: 127.0.0.1:9999
{{< /highlight >}}

The address 127.0.0.1 binds all incoming connections to Gloo to localhost. This ensures that only the envoy
sidecar can connect to the Gloo, but not any other malicious sources.

The Gloo Settings CR gets picked up automatically within ~5 seconds, so there’s no need to restart the Gloo pod.


#### Changes to the xDS clients

##### Gateway-Proxy

The gateway-proxy pod is changed so that Envoy will initialize the connection to Gloo using TLS.

The configmap has the following change:

{{< highlight yaml "hl_lines=2-13" >}}
    clusters:
      - name: gloo.gloo-system.svc.cluster.local:9977
        transport_socket:
          name: envoy.transport_sockets.tls
          typed_config:
            "@type": type.googleapis.com/envoy.api.v2.auth.UpstreamTlsContext
            common_tls_context:
              tls_certificate_sds_secret_configs:
                - name: server_cert
                  sds_config:
                    api_config_source:
                      api_type: GRPC
                      grpc_services:
                      - envoy_grpc:
                          cluster_name: gateway_proxy_sds
              validation_context_sds_secret_config:
                name: validation_context
                sds_config:
                  api_config_source:
                    api_type: GRPC
                    grpc_services:
                    - envoy_grpc:
                        cluster_name: gateway_proxy_sds
      - name: gateway_proxy_sds
        connect_timeout: 0.25s
        http2_protocol_options: {}
        load_assignment:
          cluster_name: sds_server_mtls
          endpoints:
            - lb_endpoints:
                - endpoint:
                    address:
                      socket_address:
                        address: 127.0.0.1
                        port_value: 8234
{{< /highlight >}}

The gateway-proxy deployment is changed to provide the certs to the pod.

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

A SDS sidecar is also added to the gateway-proxy deployment:

```yaml
      - name: sds
        image: "quay.io/solo-io/sds:1.3.4"
        imagePullPolicy: IfNotPresent
        volumeMounts:
        - mountPath: /etc/envoy/ssl
          name: gloo-mtls-certs
          readOnly: true
```

#### Extauth Server

To make the default extauth server work with mTLS, the extauth deployment takes in an additional environment
variable:

{{< highlight yaml "hl_lines=2-3" >}}
        env:
        - name: GLOO_MTLS
          value: "true"
{{< /highlight >}}

The gloo-mtls-certs are added to the volumes section and mounted in the extauth container:

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

To make the default rate-limiting server work with mTLS, the rate-limit deployment takes in an additional environment
variable as well:

{{< highlight yaml "hl_lines=2-3" >}}
        env:
        - name: GLOO_MTLS
          value: "true"
{{< /highlight >}}

The gloo-mtls-certs are added to the volumes section and mounted in the rate-limit container:

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

### Logging

#### SDS sidecar
The gloo, gateway-proxy, extauth and rate-limiting pods will have SDS sidecars
when Gloo is running in mTLS mode. To see the logs for the sds server, run:
```
kubectl logs -n gloo-system deploy/gloo sds
kubectl logs -n gloo-system deploy/gateway-proxy sds
kubectl logs -n gloo-system deploy/extauth sds
kubectl logs -n gloo-system deploy/rate-limit sds
```

You should see logs like:
```
"caller":"server/server.go:57","msg":"sds server listening on 127.0.0.1:8234"
"logger":"sds_server","caller":"server/server.go:97","msg":"Updating SDS config. Snapshot version is xxxx"
```

#### Envoy sidecar
The gloo, extauth, and rate-limiting pods will have Envoy sidecar containers.
To see the logs for the Envoy sidecar containers, run:
```
kubectl logs -n gloo-system deploy/gloo envoy-sidecar
kubectl logs -n gloo-system deploy/extauth envoy-sidecar
kubectl logs -n gloo-system deploy/rate-limit envoy-sidecar
```

If the SDS server hasn't started up yet, the Envoy sidecar will contain log lines like:
```
StreamSecrets gRPC config stream closed: 14, upstream connect error or disconnect/reset before headers. reset reason: connection failure
```
Once the SDS server starts up and provides certs to the Envoy sidecar, these messages will stop.

Each Envoy sidecar also has an administration interface available on port 8001. To access
this page (e.g. for the gloo pod's Envoy sidecar), run:
```
kubectl port-forward -n gloo-system deploy/gloo 8001
```

To check that the SDS server has successfully delivered certs, check [localhost:8001/certs](http://localhost:8001/certs).