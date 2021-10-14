---
title: Discovered Upstream Configuration via Annotations
weight: 101
---

Gloo will look for discovered Upstream configuration in the annotations of any Services it identifies. Creating a Service which has an annotation with `"gloo.solo.io/UpstreamConfig"` as its key, and Upstream configuration as JSON as its value will apply the Upstream configuration to the discovered Upstream.

For example, we can set the initial stream window size on the discovered upstream using the a modified version of the pet store manifest provided in the parent document:

{{< tabs >}}
{{< tab name="kubectl" codelang="yaml">}}
kubectl apply -f - <<EOF
# petstore service
apiVersion: apps/v1
kind: Deployment
metadata:
  labels:
    app: petstore
  name: petstore
  namespace: default
spec:
  selector:
    matchLabels:
      app: petstore
  replicas: 1
  template:
    metadata:
      labels:
        app: petstore
    spec:
      containers:
      - image: soloio/petstore-example:latest
        name: petstore
        ports:
        - containerPort: 8080
          name: http
---
apiVersion: v1
kind: Service
metadata:
  annotations:
    gloo.solo.io/upstream_config: '{"spec": {"initial_stream_window_size": 2048}}'
  name: petstore
  namespace: default
  labels:
    service: petstore
spec:
  ports:
  - port: 8080
    protocol: TCP
  selector:
    app: petstore
EOF
{{< /tabs >}}

Let's look at the yaml output for this upstream from Kubernetes:

```shell
kubectl get upstream -n gloo-system default-petstore-8080 -oyaml
```

```yaml
apiVersion: gloo.solo.io/v1
kind: Upstream
metadata:
  annotations:
    gloo.solo.io/upstream_config: '{"spec": {"initial_stream_window_size": 2048}}'
    kubectl.kubernetes.io/last-applied-configuration: |
      {"apiVersion":"v1","kind":"Service","metadata":{"annotations":{"gloo.solo.io/upstream_config":"{\"spec\": {\"initial_stream_window_size\": 2048}}"},"labels":{"service":"petstore"},"name":"petstore","namespace":"default"},"spec":{"ports":[{"port":8080,"protocol":"TCP"}],"selector":{"app":"petstore"}}}
  creationTimestamp: "2021-10-14T13:22:12Z"
  generation: 2
  labels:
    discovered_by: kubernetesplugin
  name: default-petstore-8080
  namespace: default
  resourceVersion: "5679"
  uid: 0ab14ba5-6377-40c5-a781-ce33b7755cdc
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
  initialStreamWindowSize: 2048
status:
  statuses:
    default:
      reportedBy: gloo
      state: 1
```

As you can see, our configuration has set `spec.initialStreamWindowSize` on the discovered upstream! 