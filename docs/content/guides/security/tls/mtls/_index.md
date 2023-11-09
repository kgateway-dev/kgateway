---
title: Gloo Edge mTLS mode
weight: 40
description: Ensure that communications between Gloo Edge and Envoy is secure with mTLS
---

{{% notice note %}}
This feature was introduced in version 1.3.6 of Gloo Edge and version 1.3.0-beta3 of Gloo Edge Enterprise. If you are using earlier versions of Gloo Edge, this feature will not be available.
{{% /notice %}}

Gloo Edge and Envoy communicate through the [xDS protocol](https://www.envoyproxy.io/docs/envoy/latest/api-docs/xds_protocol#streaming-grpc-subscriptions). Since the Envoy configuration can contain secret data, plaintext communication between Gloo Edge and Envoy may be too insecure. This is especially true if your setup has the Gloo Edge control plane and Envoy instances running in separate clusters.

Mutual TLS authentication (mTLS) ensures that both the client and server in a session are presenting valid certificates to each other. Turning on mTLS will encrypt the xDS communication between Gloo Edge and Envoy and validate the identity of both parties in the session.

---

## Enabling mTLS

It is possible to skip the manual installation phase by passing in the following helm-override.yaml file.

```yaml
global:
  glooMtls:
    enabled: true
```

Then, we run:

`glooctl install gateway --values helm-override.yaml`

This will ensure that Envoy initializes the connection to Gloo Edge using mTLS. Gloo Edge will now answer through a TCP proxy that communicates with the TLS protocol. We do this by attaching an envoy sidecar to the gloo pod to do TLS termination.

For Gloo Edge Enterprise users, the extauth and rate-limiting servers also need to communicate with Gloo Edge in order to get configuration. These pods will now start up a gRPC connection with additional TLS credentials.

---

## Detailed Explanation

This is a step-by step-guide to what the `global.glooMtls.enabled=true` Helm value does to the Gloo Edge installation.

### Secret Creation

The first step is to create a Kubernetes secret object of type 'kubernetes.io/tls'. If Gloo Edge is installed with the Helm override flag, a Job called 'gloo-mtls-certgen' is created to automatically generate the 'gloo-mtls-certs' secret for you. The secret object has the following structure:

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

### Gloo Edge Deployment (the xDS server)

In the gloo deployment, two sidecars are added: the envoy sidecar and the SDS sidecar.

The purpose of the envoy sidecar is to do TLS termination on the default gloo xdsBindAddr (0.0.0.0:9977) with something that accepts and validates a TLS connection.

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

The 'containerPort: 9977' stanza and the 'readinessProbe' stanza move away from the gloo container, so those sections need to be deleted from the gloo container.

SDS stands for [secret discovery service](https://www.envoyproxy.io/docs/envoy/latest/configuration/security/secret), a new feature in Envoy that allows you to rotate certificates without needing to restart envoy.

In the gloo deployment, this sidecar is added as:
 
```yaml
      - name: sds
        image: "quay.io/solo-io/sds:1.3.4"
        imagePullPolicy: IfNotPresent
        env:
        - name: GLOO_MTLS_SDS_ENABLED
          value: "true"
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

### Gloo Edge Settings

The default Settings custom resource (CR) changes such that the gloo.xdsBindAddr will only listen to incoming requests from localhost.

`kubectl edit settings.gloo.solo.io -n gloo-system default -oyaml`

{{< highlight yaml "hl_lines=2" >}}
  gloo:
    xdsBindAddr: 127.0.0.1:9999
{{< /highlight >}}

The address 127.0.0.1 binds all incoming connections to Gloo Edge to localhost. This ensures that only the envoy sidecar can connect to the Gloo Edge, but not any other malicious sources.

The Gloo Edge Settings CR gets picked up automatically within ~5 seconds, so there’s no need to restart the Gloo Edge pod.

### Changes to the xDS clients

#### Gateway-Proxy

The gateway-proxy pod is changed so that Envoy will initialize the connection to Gloo Edge using TLS.

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

An SDS sidecar is also added to the gateway-proxy deployment:

```yaml
      - name: sds
        image: "quay.io/solo-io/sds:1.5.0-beta23"
        imagePullPolicy: IfNotPresent
        env:
        - name: GLOO_MTLS_SDS_ENABLED
          value: "true"
        volumeMounts:
        - mountPath: /etc/envoy/ssl
          name: gloo-mtls-certs
          readOnly: true
```

### Extauth Server

To make the default extauth server work with mTLS, the extauth deployment adds in an Envoy sidecar and SDS sidecar.

The envoy sidecar is responsible for TLS termination and outgoing encryption, and uses the SDS sidecar to handle cert rotation.
The SDS sidecar watches the gloo-mtls-certs kube secret and provides those certs when the envoy sidecar is sending the request for Gloo Edge configuration.

The configuration for the extauth envoy sidecar can be found in the extauth-sidecar-config confimap in the gloo-system
namespace. It:

1) listens to 127.0.0.1:9955 and routes to Gloo Edge's XDS port.
2) listens to 0.0.0.0:8083 and routes to 127.0.0.1:8084, extauth's Server Port.

### Rate-limiting Server

To make the default rate-limiting server work with mTLS, the rate-limit deployment also adds in an Envoy sidecar
and SDS sidecar.

The envoy sidecar is responsible for TLS termination and outgoing encryption, and uses the SDS sidecar to handle cert rotation.
The SDS sidecar watches the gloo-mtls-certs kube secret and provides those certs when the envoy sidecar is sending the request for Gloo Edge configuration.

The configuration for the extauth envoy sidecar can be found in the rate-limit-sidecar-config confimap. It:

1) listens to 127.0.0.1:9955 and routes to Gloo Edge's XDS port.

---

## Cert Rotation

Cert rotation can be done by updating the `gloo-mtls-certs` secret. The SDS sidecar automatically picks up the change.

If you want to automatically rotate certificates based on a schedule, you can use the Gloo Edge `certGen` CronJob. The job is configured to rotate certificates in stages to minimize the downtime for your apps. You have the option to instruct Gloo Edge to wait between stages to ensure that your workloads have enough time to pick up certificate changes. The job follows the following steps: 

1. The cert rotation job creates a new Certificate Authority (CA) bundle. 
2. The new CA bundle is added to the `gloo-mtls-certs` secret alongside the old CA bundle that is about to expire.
3. Gloo Edge waits for the duration that is set in `gateway.certGenJob.rotationDuration` before continuing to the next step so that workloads in the cluster can pick up this change.
4. Set the server certificate and private key of the new CA bundle to be the current CA. 
5. Gloo Edge waits for the duration that is set in `gateway.certGenJob.rotationDuration` before continuing to the next step.
6. Remove the old CA bundle from the `gloo-mtls-certs` secret. All workloads now use the new CA bundle.

To enable the `certGen` CronJob, set the `gateway.certGenJob.cron.enabled` option to `true` and specify a rotation schedule in your Gloo Edge Helm values file as shown in the following example. If you also want to configure the wait time between stages, use the `gateway.certGenJob.rotationDuration` option. The default wait time is 65s. 

```yaml
global:
  glooMtls:
    enabled: true
gateway:
  certGenJob:
    cron:
      enabled: true
      schedule: "* * * * *" # enter cron schedule here
    rotationDuration: 120s
```

---

## Logging

### SDS sidecar

The gloo, gateway-proxy, extauth and rate-limiting pods will have SDS sidecars when Gloo Edge is running in mTLS mode. To see the logs for the sds server, run:

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

### Envoy sidecar

The gloo, extauth, and rate-limiting pods will have Envoy sidecar containers. To see the logs for the Envoy sidecar containers, run:

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

Each Envoy sidecar also has an administration interface available on port 8001. To access this page (e.g. for the gloo pod's Envoy sidecar), run:

```
kubectl port-forward -n gloo-system deploy/gloo 8001
```

To check that the SDS server has successfully delivered certs, check [localhost:8001/certs](http://localhost:8001/certs).

---

## Next Steps

In addition to mutual TLS, you can also configure client TLS to Upstreams and server TLS to downstream clients. Check out these guides to learn more:

* **[Setting up Upstream TLS]({{% versioned_link_path fromRoot="/guides/security/tls/client_tls//" %}})**
* **[Setting up Upstream TLS with Service Annotations]({{% versioned_link_path fromRoot="/guides/security/tls/client_tls_service_annotations//" %}})**
* **[Setting up Server TLS]({{% versioned_link_path fromRoot="/guides/security/tls/server_tls//" %}})**
