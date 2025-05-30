# Run the controller locally

To run the controller locally on your host outside Kubernetes, set the following environment variables:

- `KUBECONFIG`: Path to your kubeconfig file.
- `KGW_LOG_LEVEL`: Log level for the controller
- `KGW_XDS_SERVICE_HOST`: Host for the xDS service the Gateway will connect to.
- `KGW_DEFAULT_IMAGE_REGISTRY`: Default image registry for the Gateway.
- `KGW_DEFAULT_IMAGE_TAG`: Default image tag for the Gateway.
- `KGW_DEFAULT_IMAGE_PULL_POLICY`: Default image pull policy for the Gateway.

```bash
export KUBECONFIG=/home/user/.kube/config
export KGW_LOG_LEVEL=info
export KGW_XDS_SERVICE_HOST=172.17.0.1
export KGW_DEFAULT_IMAGE_REGISTRY=ghcr.io/kgateway-dev
export KGW_DEFAULT_IMAGE_TAG=2.0.0-dev
export KGW_DEFAULT_IMAGE_PULL_POLICY=IfNotPresent

go run cmd/kgateway/main.go
```

> Note: `172.17.0.1` is IP address of the `docker0` bridge interface that allows pods running in `Kind` to access the host network, thereby allowing the Gateway proxy to connect to the xDS service running on the host.

> Additional note: Replacing `172.17.0.1` with `192.168.65.254`, as the value for `KGW_XDS_SERVICE_HOST` may help, if the gateway proxy pods in a local kind cluster are unable to connect to the xDS service running on the local machine. `192.168.65.254` is the IP address that `host.docker.internal` resolves to on gateway proxy pods when running in a local kind cluster.

## Vscode Debugger

Use the following `launch.json` configuration to run the controller in the debugger:
```json
{
  "name": "app",
  "type": "go",
  "request": "launch",
  "mode": "auto",
  "program": "${workspaceFolder}/cmd/kgateway/main.go",
  "env": {
    "KUBECONFIG": "/home/user/.kube/config",
    "KGW_LOG_LEVEL": "info",
    "KGW_XDS_SERVICE_HOST": "172.17.0.1",
    "KGW_DEFAULT_IMAGE_REGISTRY": "ghcr.io/kgateway-dev",
    "KGW_DEFAULT_IMAGE_TAG": "2.0.0-dev",
    "KGW_DEFAULT_IMAGE_PULL_POLICY": "IfNotPresent",
  },
}
```

## Steps to run and inspect local builds:

Setup a local kind cluster:
```sh
VERSION=2.0.0-dev ./hack/kind/setup-kind.sh
```

Install the required CRDs:
```sh
helm install kgateway-crds install/helm/kgateway-crds
```

Create a namespace for testing:
```sh
kubectl create ns kgateway-system
```

Run kgateway locally using one of the methods described above: either `go run` or the vscode debugger.

Create an example gateway:
```sh
kubectl -n kgateway-system apply -f examples/example-gw.yaml
```

Create an example route:
```sh
kubectl -n kgateway-system apply -f examples/example-http-route.yaml
```

Using a local web browser:
- GET http://localhost:9097/snapshots/krt to inspect the KRT snapshot.
- GET http://localhost:9097/snapshots/xds to inspect the XDS snapshot.

When finished testing:

```sh
kubectl -n kgateway-system delete -f examples/example-gw.yaml
kind delete cluster
```
