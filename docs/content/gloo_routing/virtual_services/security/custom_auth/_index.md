---
title: Custom Auth server
weight: 80
description: External Authentication with your own auth server
---

While Gloo provides an auth server that covers your OpenID Connect, ApiKey, and Basic Auth use cases, it
also allows your to use your own authentication server, to implement custom auth logic.

In this guide we will demonstrate your to create and configure Gloo to use your own auth service.
For simplicity we will use an http service. Though this guide will work (with minor adjustments) also work with a gRPC server that implements
the Envoy spec for an [external authorization server](https://github.com/envoyproxy/envoy/blob/master/api/envoy/service/auth/v2/external_auth.proto).

Let's get right to it!

## Deploy Gloo and the petstore demo app

Install Gloo-enterprise (version v0.13.5 or above) and the petstore demo:

```shell
glooctl install gateway enterprise --license-key <YOUR KEY>
kubectl apply --filename https://raw.githubusercontent.com/solo-io/gloo/master/example/petstore/petstore.yaml
```

Add a route and test that everything so far works:

```shell
glooctl add route --name default --namespace gloo-system --path-prefix / --dest-name default-petstore-8080 --dest-namespace gloo-system
curl "$(glooctl proxy url)/api/pets/"
```

```json
[{"id":1,"name":"Dog","status":"available"},{"id":2,"name":"Cat","status":"pending"}]
```

## HTTP Authentication service intro

When using an HTTP auth service, the request will be forwarded to the authentication service. If the
auth service returns `200 OK` it is considered authorized. Otherwise the request is denied.
You can fine tune which headers are sent to the the auth service, and wether or not the body is forwarded as well, by editing the [extauth extension](/v1/github.com/solo-io/solo-projects/projects/gloo/api/v1/plugins/extauth/extauth.proto.sk/#settings) settings in the Gloo settings (see [below](#configure-gloo-settings) for an example of the Gloo settings with the extension settings).

For reference, here's the code for the authorization server used in this tutorial:

```python
import http.server
import socketserver

class Server(http.server.SimpleHTTPRequestHandler):
    def do_GET(self):
        path = self.path
        print("path", path)
        if path.startswith("/api/pets/1"):
            self.send_response(200, 'OK')
        else:
            self.send_response(401, 'Not authorized')
        self.send_header('x-server', 'pythonauth')
        self.end_headers()

def serve_forever(port):
    socketserver.TCPServer(('', port), Server).serve_forever()

if __name__ == "__main__":
    serve_forever(8000)
```

As you can see, this service will allow requests to `/api/pets/1` and will deny everything else.

{{% notice tip %}}
You can easily change the sample auth server. When using minikube, download the [Dockerfile](Dockerfile) and the [server code](server.py) and just run:

```shell
eval $(minikube docker-env)
docker build -t quay.io/solo-io/sample-auth .
kubectl --namespace gloo-system delete pod -l app=sample-auth
```
{{% /notice %}}

### Deploy auth service

To add this service to your cluster, download the [auth-service yaml](auth-service.yaml) and apply it:

```shell
kubectl apply --filename auth-service.yaml
```

This file contains the deployment, service and upstream definitions.

## Configure Gloo to use your server

### Configure Gloo settings

Edit the gloo settings (`kubectl --namespace gloo-system edit settings default`) to point to your auth server.
The Settings custom resource should look like this:

{{< highlight yaml "hl_lines=9-18" >}}
apiVersion: gloo.solo.io/v1
kind: Settings
metadata:
  name: default
  namespace: gloo-system
spec:
  bindAddr: 0.0.0.0:9977
  discoveryNamespace: gloo-system
  extensions:
    configs:
      extauth:
        extauthzServerRef:
          name: auth-server
          namespace: gloo-system
        httpService: {}
        requestBody:
          maxRequestBytes: 10240
        requestTimeout: 0.5s
      rate-limit:
        ratelimit_server_ref:
          name: rate-limit
          namespace: gloo-system
  kubernetesArtifactSource: {}
  kubernetesConfigSource: {}
  kubernetesSecretSource: {}
  refreshRate: 60s
{{< /highlight >}}

More details about the `httpService` object are available [here](/v1/github.com/solo-io/solo-projects/projects/gloo/api/v1/plugins/extauth/extauth.proto.sk#httpservice). For example, if you want to copy some of the request headers to your custom auth server
you would do something like the following example that will pass the `X-foo` request header to the auth server.

{{< highlight yaml "hl_lines=15-18" >}}
apiVersion: gloo.solo.io/v1
kind: Settings
metadata:
  name: default
  namespace: gloo-system
spec:
  bindAddr: 0.0.0.0:9977
  discoveryNamespace: gloo-system
  extensions:
    configs:
      extauth:
        extauthzServerRef:
          name: auth-server
          namespace: gloo-system
        httpService:
          request:
            allowedHeaders:
            - "X-foo"
        requestBody:
          maxRequestBytes: 10240
        requestTimeout: 0.5s
      rate-limit:
        ratelimit_server_ref:
          name: rate-limit
          namespace: gloo-system
  kubernetesArtifactSource: {}
  kubernetesConfigSource: {}
  kubernetesSecretSource: {}
  refreshRate: 60s
{{< /highlight >}}

{{% notice tip %}}
When using a gRPC auth service, remove the `httpService: {}` line from the config above.
{{% /notice %}}

This configuration also sets other configuration parameters:

- requestBody - When set to, the request body will also be sent to the auth service. with this configuration, a body up to 10KB will be buffered and sent to the auth-service. This is useful in use cases where the auth service needs to compute an HMAC on the body.
- requestTimeout - A timeout for the auth service response. If the service takes longer to response, the request will be denied.

### Configure the VirtualService

Edit the VirtualService (`kubectl --namespace gloo-system edit virtualservice default`), and mark it with custom auth to turn authentication on. The VirtualService should look like this:

{{< highlight yaml "hl_lines=10-14" >}}
apiVersion: gateway.solo.io/v1
kind: VirtualService
metadata:
  name: default
  namespace: gloo-system
spec:
  virtualHost:
    domains:
    - '*'
    virtualHostPlugins:
      extensions:
        configs:
          extauth:
            customAuth: {}
    routes:
    - matcher:
        prefix: /
      routeAction:
        single:
          upstream:
            name: default-petstore-8080
            namespace: gloo-system
{{< /highlight >}}

To make it easy, if you have followed this guide verbatim, you can just download and apply [this](gloo-vs.yaml) manifest to update both Settings and VirtualService.

## Test

We are all set to test!

```shell
curl --write-out "%{http_code}\n" "$(glooctl proxy url)/api/pets/1"
```

```noop
{"id":1,"name":"Dog","status":"available"}
200
```

```shell
curl --write-out "%{http_code}\n" "$(glooctl proxy url)/api/pets/2"
```

```noop
401
```

## Conclusion

Gloo's extendable architecture allows follows the 'batteries included but replaceable' approach.
while you can use Gloo's built in auth services for OpenID Connect and Basic Auth, you can also
extend Gloo with your own custom auth logic.
