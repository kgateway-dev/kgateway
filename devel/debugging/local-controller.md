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
