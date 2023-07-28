---
title: HTTP2
weight: 123
description: Enabling HTTP/2 for Upstream communication.
---

You may have services in your Kubernetes cluster that use HTTP/2 for communication. Typically these are gRPC services, but it could apply to any service that uses HTTP/2 in its transport layer. To enable HTTP/2 communication, the `useHttp2` value for the Upstream must be set to true. This signals to the Envoy proxy that HTTP/2 should be used for communication.

In this guide, we will show you two ways to enable HTTP/2 communication by configuring your Kubernetes service properly. We will walk through the following steps:

1. Deploy a Kubernetes example service
1. Update annotations on the service and verify HTTP/2
1. Update port naming on the service and verify HTTP/2

Let's get started!

---

## Prerequisites

To follow along with this guide, you will need to have a [Kubernetes cluster deployed]({{% versioned_link_path fromRoot="/installation/platform_configuration/cluster_setup/" %}}) with [Gloo Edge installed]({{% versioned_link_path fromRoot="/installation/gateway/kubernetes/" %}}) and service discovery enabled. 

---

## Deploy a Kubernetes example service

To demonstrate the configuration for the HTTP/2, we will deploy the Pet Store application to the `default` namespace of our cluster using the following command:

```shell
kubectl apply -f https://raw.githubusercontent.com/solo-io/gloo/v1.14.x/example/petstore/petstore.yaml
```

```console
deployment.apps/petstore created
service/petstore created
```

Let's take a look at the configuration of the service for the Pet Store application and the Upstream Gloo Edge has created for it.

```shell
kubectl get service petstore -oyaml
```

Some output has been truncated for brevity, but you should see something like this.

```yaml
apiVersion: v1
kind: Service
metadata:
  annotations:
    kubectl.kubernetes.io/last-applied-configuration: |
      {"apiVersion":"v1","kind":"Service","metadata":{"annotations":{},"labels":{"service":"petstore"},"name":"petstore","namespace":"default"},"spec":{"ports":[{"port":8080,"protocol":"TCP"}],"selector":{"app":"petstore"}}}
  creationTimestamp: "2020-11-18T14:31:32Z"
  labels:
    service: petstore
  name: petstore
  namespace: default
spec:
  clusterIP: 10.111.4.63
  ports:
  - port: 8080
    protocol: TCP
    targetPort: 8080
  selector:
    app: petstore
  sessionAffinity: None
  type: ClusterIP
status:
  loadBalancer: {}
```

The Upstream can be retrieved with the following:

```shell
kubectl get us -n gloo-system default-petstore-8080 -oyaml
```

Some output has been truncated for brevity, but you should see something like this.

```yaml
apiVersion: gloo.solo.io/v1
kind: Upstream
metadata:
  annotations:
    kubectl.kubernetes.io/last-applied-configuration: |
      {"apiVersion":"v1","kind":"Service","metadata":{"annotations":{},"labels":{"service":"petstore"},"name":"petstore","namespace":"default"},"spec":{"ports":[{"port":8080,"protocol":"TCP"}],"selector":{"app":"petstore"}}}
  creationTimestamp: "2020-11-18T14:31:32Z"
  generation: 2
  labels:
    discovered_by: kubernetesplugin
  name: default-petstore-8080
  namespace: gloo-system
spec:
  discoveryMetadata:
    labels:
      service: petstore
  kube:
    selector:
      app: petstore
    serviceName: petstore
    serviceNamespace: default
    servicePort: 8080
status:
  reportedBy: gloo
  state: 1
```

You'll note that the setting `spec.useHttp2` is not present.

### Enable HTTP/2 through annotation

One of the ways to enable HTTP/2, is to set an annotation value on the Kubernetes service. The field for the annotation is `gloo.solo.io/h2_service` and it can be set to `true` or `false`. Gloo Edge will update the Upstream automatically if the field is present, based on the value set for the field. 

Let's update our Pet Store service with the annotation and check the Upstream.

```shell
kubectl annotate service petstore gloo.solo.io/h2_service=true
```

Now let's take a look at our Upstream settings again using the same command as before. We've included just the `spec` portion of the output.

```yaml
spec:
  discoveryMetadata:
    labels:
      service: petstore
  kube:
    selector:
      app: petstore
    serviceName: petstore
    serviceNamespace: default
    servicePort: 8080
  useHttp2: true
```

The setting `spec.useHttp2` has now been set to `true`. If we update the annotation to the value `false`, the setting should also update to `false`. Let's test that now.

```shell
kubectl annotate service petstore gloo.solo.io/h2_service=false --overwrite
```

```yaml
spec:
  discoveryMetadata:
    labels:
      service: petstore
  kube:
    selector:
      app: petstore
    serviceName: petstore
    serviceNamespace: default
    servicePort: 8080
  useHttp2: false
```

As expected the `spec.useHttp2` setting now has a value of `false`. Removing the annotation will not impact the existing value, but let's remove it for to explore the port naming option.

```shell
kubectl annotate service petstore gloo.solo.io/h2_service- 
```

{{< notice note >}}
In a race condition where both the annotation and port name are set, the annotation value will win as it is evaluated first. If the annotation is set to `false` and the port name is set to `http2`, the `spec.useHttp2` setting on the Upstream will be evaluated to `false`. We recommend using only one of the two options presented.
{{< /notice >}}

### Enable HTTP/2 with port names

The other way to enable HTTP/2 is by setting a specific port name for the service. The name of the port must be one of the following: `grpc`, `http2`, or `h2`. Currently, our Pet Store service does not have a name value for the exposed port `8080`. Let's update the port with the name `http2`.

```shell
kubectl patch service petstore -p '{"spec": { "ports": [ { "name": "http2", "port": 8080, "protocol": "TCP", "targetPort": 8080 } ] } }'
```

Now let's check the Upstream to see if it updated.

```shell
kubectl get us -n gloo-system default-petstore-8080 -oyaml
```

Only the spec at the end of the output has been included.

```yaml
spec:
  discoveryMetadata:
    labels:
      service: petstore
  kube:
    selector:
      app: petstore
    serviceName: petstore
    serviceNamespace: default
    servicePort: 8080
  useHttp2: true
```

You can see that the `spec.Http2` setting has been set back to `true`. As mentioned in the previous section, we recommend using only one of the two methods to enable HTTP/2. During evaluation, the value set on the annotation will override the port naming.

---

## Summary

In this guide you saw how you can use either an annotation or port name to enable HTTP/2 for a Kubernetes service and the accompanying Upstream. The most common application for HTTP/2 is gRPC, so we recommend checking out our guides for working with [gRPC Upstreams]({{% versioned_link_path fromRoot="/guides/traffic_management/destination_types/grpc/" %}}).