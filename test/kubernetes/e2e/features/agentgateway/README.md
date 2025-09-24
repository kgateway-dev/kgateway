# agentgateway e2e tests 

## Setup

To use the agentgateway control plane with kgateway, you need to enable the integration in the helm chart via 
`--set agentgateway.enabled=true` or in the values.yaml:
```yaml
agentgateway:
  enabled: true # set this to true
```

This is done automatically by the agentgateway e2e suite. 

## Testing with an unreleased agentgateway commit 

Update the go.mod to point to the unreleased agentgateway commit:
```shell
go get github.com/my-fork/agentgateway@my-commit-sha
```

In the agentgateway repo, build the docker image locally with:
```shell
make docker 
```

Then load it into the kind cluster where you are running the e2e tests:
```shell
kind load --name kind docker-image ghcr.io/agentgateway/agentgateway:my-commit-sha
```

You can configure the agentgateway Gateway class to use a specific image by setting the image field on the
GatewayClass:
```yaml
kind: GatewayParameters
apiVersion: gateway.kgateway.dev/v1alpha1
metadata:
  name: kgateway
spec:
  kube:
    agentgateway:
      enabled: true
      logLevel: debug
      image:
        tag: my-commit-sha
---
kind: GatewayClass
apiVersion: gateway.networking.k8s.io/v1
metadata:
  name: agentgateway
spec:
  controllerName: kgateway.dev/kgateway
  parametersRef:
    group: gateway.kgateway.dev
    kind: GatewayParameters
    name: kgateway
    namespace: default
---
kind: Gateway
apiVersion: gateway.networking.k8s.io/v1
metadata:
  name: agent-gateway
spec:
  gatewayClassName: agentgateway
  listeners:
    - protocol: HTTP
      port: 8080
      name: http
      allowedRoutes:
        namespaces:
          from: All
```

This is useful for testing, but the final e2e agentgateway tests should use a released agentgateway image and not a 
local build.