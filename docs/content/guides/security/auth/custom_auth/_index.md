---
title: Custom Auth server
weight: 30
description: External Authentication with your own auth server
---

{{% notice note %}}
The custom auth feature was introduced with **Gloo Edge**, release 0.20.7, and **Gloo Edge Enterprise**, release 0.13.5. 
If you are using an earlier version, this tutorial will not work.
{{% /notice %}}

Gloo Edge Enterprise ships with an external auth server that implements a wide array of authentication and authorization models. 
Even though these features are not available in the open source version of Gloo Edge, you can deploy your own 
service and configure Gloo Edge to use it to secure your Virtual Services.

In this guide we will see how to create such a custom external auth service. For simplicity, we will implement an HTTP 
service. With minor adjustments, you should be able to use the contents of this guide to deploy a gRPC server that implements
the Envoy spec for an [external authorization server](https://github.com/envoyproxy/envoy/blob/main/api/envoy/service/auth/v3/external_auth.proto).

## Setup
{{< readfile file="/static/content/setup_notes" markdown="true">}}

Let's start by creating the sample `petstore` application:

```shell
kubectl apply -f https://raw.githubusercontent.com/solo-io/gloo/v1.14.x/example/petstore/petstore.yaml
```

We can now add a route to the sample application by running the following command:

{{< tabs >}}
{{< tab name="kubectl" codelang="shell" >}}
kubectl apply -f - <<EOF
apiVersion: gateway.solo.io/v1
kind: VirtualService
metadata:
  name: default
  namespace: gloo-system
spec:
  virtualHost:
    domains:
    - '*'
    routes:
    - matchers:
      - prefix: /
      routeAction:
        single:
          upstream:
            name: default-petstore-8080
            namespace: gloo-system
EOF
{{< /tab >}}
{{< tab name="glooctl" codelang="shell" >}}
glooctl add route --name default --namespace gloo-system --path-prefix / --dest-name default-petstore-8080 --dest-namespace gloo-system
{{< /tab >}}
{{< /tabs >}}

Let's verify that everything so far works by querying the virtual service.

```shell script
curl $(glooctl proxy url)/api/pets/
```

You should see the following output:

```json
[{"id":1,"name":"Dog","status":"available"},{"id":2,"name":"Cat","status":"pending"}]
```

## Creating a simple HTTP Authentication service

When a request matches a route that defines an `extauth` configuration, Gloo Edge will forward the request to the external 
auth service. If the HTTP service returns a `200 OK` response, the request will be considered authorized and sent to 
its original destination. Otherwise the request will be denied.
You can fine tune which headers are sent to the the auth service, and whether or not the body is forwarded as well, 
by editing the {{< protobuf name="enterprise.gloo.solo.io.Settings" display="extauth settings" >}} 
in the Gloo Edge settings (see the example [below](#configure-gloo-edge-settings)).

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

### Deploy auth service

To deploy this service to your cluster, copy the content of 
[this file](https://github.com/solo-io/gloo/blob/v1.3.2/docs/content/security/auth/custom_auth/auth-service.yaml) 
to a file named `auth-service.yaml` and apply it:

```shell
kubectl apply --filename auth-service.yaml
```

This file contains the deployment, service and upstream definitions.

{{% notice tip %}}
When running in `minikube` you can easily update this sample auth service. Just download the [Dockerfile](Dockerfile) and 
the [server.py file](server.py) to a local directory, apply your changes to the server code, and run the following commands:

```shell
eval $(minikube docker-env)
docker build -t quay.io/solo-io/sample-auth .
kubectl --namespace gloo-system delete pod -l app=sample-auth
```
{{% /notice %}}

## Configure Gloo Edge to use your server

### Configure Gloo Edge settings

To use our custom auth server, we need to edit the Gloo Edge Settings resource. Run the following command to edit the settings:

```shell script
kubectl --namespace gloo-system edit settings default
```


We need to add the following `extauth` attribute:

{{< highlight yaml "hl_lines=18-25" >}}
apiVersion: gloo.solo.io/v1
kind: Settings
metadata:
  name: default
  namespace: gloo-system
spec:
  discoveryNamespace: gloo-system
  gateway:
    validation:
      alwaysAccept: true
      proxyValidationServerAddr: gloo:9988
  gloo:
    xdsBindAddr: 0.0.0.0:9977
  kubernetesArtifactSource: {}
  kubernetesConfigSource: {}
  kubernetesSecretSource: {}
  refreshRate: 60s
  extauth:
   extauthzServerRef:
     name: auth-server
     namespace: gloo-system
   httpService: {}
   requestBody:
     maxRequestBytes: 10240
   requestTimeout: 0.5s
{{< /highlight >}}

More details about the `httpService` object are available 
{{<
protobuf display="here"
name="enterprise.gloo.solo.io.HttpService"
>}}.
For example, if you want to copy some of the original request headers to the request that gets sent to the custom auth 
server, you would need to configure the `extauth` attribute in the following way:

{{< highlight yaml "hl_lines=22-25" >}}
apiVersion: gloo.solo.io/v1
kind: Settings
metadata:
  name: default
  namespace: gloo-system
spec:
  discoveryNamespace: gloo-system
  gateway:
    validation:
      alwaysAccept: true
      proxyValidationServerAddr: gloo:9988
  gloo:
    xdsBindAddr: 0.0.0.0:9977
  kubernetesArtifactSource: {}
  kubernetesConfigSource: {}
  kubernetesSecretSource: {}
  refreshRate: 60s
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
{{< /highlight >}}

{{% notice tip %}}
When using a gRPC auth service, remove the `httpService` attribute from the configuration above.
{{% /notice %}}

This configuration also sets other configuration parameters:

- `requestBody` - When this attribute is set, the request body will also be sent to the auth service. With the above configuration, 
a body up to 10KB will be buffered and sent to the service. This is useful in use cases where the request body is relevant 
to the authentication logic, e.g. when it is used to compute an HMAC.
- `requestTimeout` - A timeout for the auth service response. If the service takes longer to respond, the request will be denied.

### Securing the Virtual Service

Edit the VirtualService and mark it with custom auth to turn authentication on:

{{< highlight yaml "hl_lines=11-13" >}}
kubectl apply -f - <<EOF
apiVersion: gateway.solo.io/v1
kind: VirtualService
metadata:
  name: default
  namespace: gloo-system
spec:
  virtualHost:
    domains:
    - '*'
    options:
      extauth:
        customAuth: {}
    routes:
    - matchers:
      - prefix: /
      routeAction:
        single:
          upstream:
            name: default-petstore-8080
            namespace: gloo-system
EOF
{{< /highlight >}}

If you have followed this guide verbatim, you can just download and apply 
[this manifest](https://github.com/solo-io/gloo/blob/v1.3.2/docs/content/security/auth/custom_auth/gloo-vs.yaml) 
to update both the `Settings` and the `Virtual Service`.

## Test

Let's verify that our configuration has been accepted by Gloo Edge. Requests to `/api/pets/1` should be allowed:

```shell
curl --write-out "%{http_code}\n" $(glooctl proxy url)/api/pets/1
```

```noop
{"id":1,"name":"Dog","status":"available"}
200
```

Any request with a path that is not `/api/pets/1` should be denied.

```shell
curl --write-out "%{http_code}\n" $(glooctl proxy url)/api/pets/2
```

```noop
401
```

## Conclusion

Gloo Edge's extendable architecture allows follows the 'batteries included but replaceable' approach.
while you can use Gloo Edge's built in auth services for OpenID Connect and Basic Auth, you can also
extend Gloo Edge with your own custom auth logic.
