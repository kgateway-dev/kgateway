---
title: Plugin Auth
weight: 90
description: Extend Gloo's built-in auth server with custom Go plugins
---

We have seen that one way of implementing custom authentication logic is by
[providing your own auth server]({{< versioned_link_path fromRoot="/gloo_routing/virtual_services/security/custom_auth" >}}).
While this approach gives you great freedom, it also comes at a cost:

- You have to write and manage an additional service. If your authentication logic is simple, the plumbing needed to get
  it running might constitute a significant overhead.
- If your custom auth server serves only as an adapter for your existing auth service, it will introduce an additional
  network hop with the associated latency to any request that needs to be authenticated; either that, or you will most
  likely need to update your existing service so that it can conform to the request/response format expected by Gloo.
- You likely want to be able to define configuration for your auth server and that configuration should be context-specific,
  e.g. based on which virtual host is serving the request. You could store this configuration outside of Gloo (e.g. in a
  dedicated Kubernetes CRD) and try to derive the context from the request itself, but this would introduce additional
  latency and negate the benefits of the centralized control plane that Gloo provides.

Wouldn't it be nice to be able to **write just the authentication logic you need, plug it into Gloo, and be able to
provide your specific configuration** right on the Virtual Services it applies to? Starting with **Gloo Enterprise**,
release **0.18.11+**, you can do just that!

In this guide we will show you how easy it is to extend Gloo's Ext Auth server via [Go plugins](https://golang.org/pkg/plugin/).

## Development workflow overview

Following are the high-level steps required to use your auth plugins with Gloo. Through the course of this guide we will
see each one of them in greater detail.

1. Write a plugin and publish it as a `docker image` which, when run, copies the compiled plugin file(s) to a
   predefined directory.
2. Configure Gloo to load the plugins by running the image as an `initContainer` on the `extauth` deployment. This can be
   done by rendering the Gloo Helm chart with dedicated value overrides or by modifying the Gloo YAML manifest manually.
3. Reference your plugin in your Virtual Services for it to be invoked for requests matching particular virtual hosts or
   routes.

{{% notice note %}}
For a more in-depth explanation of the Ext Auth Plugin development workflow, please check our dedicated
[**Plugin Developer Guide**]({{< versioned_link_path fromRoot="/dev/writing_auth_plugins" >}}).

{{% /notice %}}

## Building an Ext Auth plugin

{{% notice note %}}
The code used in this section can be found in our [**ext-auth-plugin-examples**](https://github.com/solo-io/ext-auth-plugin-examples) GitHub repository.
{{% /notice %}}

The [official Go docs](https://golang.org/pkg/plugin/) define a plugin as:

> > "_a Go main package with exported functions and variables that has been built with: `go build -buildmode=plugin`_"

In order for Gloo to be able to load your plugin, the plugin's `main` package must export a variable that implements the
[ExtAuthPlugin](https://github.com/solo-io/ext-auth-plugins/blob/master/api/interface.go#L61) interface:

```go
type ExtAuthPlugin interface {
  NewConfigInstance(ctx context.Context) (configInstance interface{}, err error)
  GetAuthService(ctx context.Context, configInstance interface{}) (AuthService, error)
}
```

Check the [plugin developer guide]({{< versioned_link_path fromRoot="/dev/writing_auth_plugins#api-overview" >}}) for a detailed explanation of the API.

For this guide we will use a simple example plugin that has already been built. You can find the source code to build it
yourself [here](https://github.com/solo-io/ext-auth-plugin-examples/tree/master/plugins/required_header).
The plugin authorizes requests if:

1. they contain a certain header, and
2. the value for the header is in a predefined whitelist.

The `main` package for the plugin looks like this:

{{< highlight go "hl_lines=10-11" >}}
package main

import (
"github.com/solo-io/ext-auth-plugins/api"
impl "github.com/solo-io/ext-auth-plugins/plugins/required_header/pkg"
)

func main() {}

// This is the exported variable that Gloo will look for. It implements the ExtAuthPlugin interface.
var Plugin impl.RequiredHeaderPlugin
{{< /highlight >}}

We leave it up to you to inspect the simple `impl.RequiredHeaderPlugin` object. Of interest here is the following
configuration object it uses:

```go
type Config struct {
  RequiredHeader string
  AllowedValues  []string
}
```

The values in this struct will determine the aforementioned header and whitelist.

### Packaging and publishing the plugin

Ext Auth plugins must be made available to Gloo in the form of container images. The images must contain the compiled
plugins and copy these files to the `/auth-plugins` when they are run.

In this guide we will use the image for the `RequiredHeaderPlugin` introduced above. It has been built using
[this Dockerfile](https://github.com/solo-io/ext-auth-plugin-examples/blob/master/Dockerfile) and can be found in
the `quay.io/solo-io/ext-auth-plugins` docker repository. Let's inspect the image:

{{< highlight shell_script "hl_lines=3" >}}
docker run -it --entrypoint ls quay.io/solo-io/ext-auth-plugins:0.18.23 -l compiled-auth-plugins
total 28352
-rw-r--r-- 1 root root 29029288 Sep 3 21:06 RequiredHeader.so
{{< /highlight >}}

You can see that it contains the compiled plugin file `RequiredHeader.so`.

{{% notice warning %}}
Make sure the plugin image tag matches the version of GlooE you are using. Plugins with mismatching versions will most
likely fail to be loaded due to the compatibility constraints imposed by the Go plugin model. See
[this section]({{< versioned_link_path fromRoot="/dev/writing_auth_plugins#build-helper-tools" >}}) of the plugin developer guide for more information.
{{% /notice %}}

## Configuring Gloo

### Installation

Let's start by installing Gloo Enterprise (make sure the version is >= **0.18.11**). We will use the
[Helm install option]({{< ref "/installation/enterprise#installing-on-kubernetes-with-helm" >}}), as it is the easiest
way of configuring Gloo to load your plugin. First we need to fetch the Helm chart:

```bash
helm fetch glooe/gloo-ee --version "0.18.23 --untar"
```

Then we have to create the following `plugin-values.yaml` value overrides file:

{{< highlight bash "hl_lines=7-12" >}}
cat << EOF > plugin-values.yaml
license_key: YOUR_LICENSE_KEY
global:
extensions:
extAuth:
plugins:
my-plugin:
image:
repository: ext-auth-plugins
registry: quay.io/solo-io
pullPolicy: IfNotPresent
tag: 0.18.23
EOF
{{< /highlight >}}

`global.extensions.extAuth.plugins` is a map where:

- each key is a plugin container display name (in this case `my-plugin`)
- the correspondent value is an image spec

Now we can render the helm chart and `apply` it:

```bash
kubectl create namespace gloo-system
helm template gloo-ee --name glooe --namespace gloo-system -f plugin-values.yaml | kubectl apply -n gloo-system -f -
```

If we inspect the `extauth` deployment by running

```bash
kubectl get deployment -n gloo-system extauth -o yaml
```

we should see the following information (non-relevant attributes have been omitted for brevity):

{{< highlight yaml "hl_lines=23-36" >}}
apiVersion: apps/v1
kind: Deployment
metadata:
labels:
app: gloo
gloo: extauth
name: extauth
namespace: gloo-system
spec:
selector:
matchLabels:
gloo: extauth
template:
metadata:
labels:
gloo: extauth
spec:
containers: - image: quay.io/solo-io/extauth-ee:0.18.23
imagePullPolicy: Always
name: extauth
resources: {}
volumeMounts: - mountPath: /auth-plugins
name: auth-plugins
initContainers: - image: quay.io/solo-io/ext-auth-plugins:0.18.23
imagePullPolicy: Always
name: plugin-my-plugin
volumeMounts: - mountPath: /auth-plugins
name: auth-plugins
volumes: - emptyDir: {}
name: auth-plugins
{{< /highlight >}}

Each plugin container image built as described in the _Packaging and publishing the plugin_
[section]({{< versioned_link_path fromRoot="/gloo_routing/virtual_services/security/plugin_auth#packaging-and-publishing-the-plugin" >}})
has been added as an `initContainer` to the `extauth` deployment. A volume named `auth-plugins` is mounted in the
`initContainer`s and the `extauth` container at `/auth-plugins` path: when the `initContainer`s are run, they will copy
the compiled plugin files they contain (in this case `RequiredHeader.so`) to the shared volume, where they become available
to the `extauth` server.

Let's verify that the `extauth` server did successfully start by checking its logs.

```bash
kc logs -n gloo-system deployment/extauth
{"level":"info","ts":1567688585.001545,"logger":"extauth","caller":"runner/run.go:84","msg":"Starting ext-auth server"}
{"level":"info","ts":1567688585.0016475,"logger":"extauth","caller":"runner/run.go:103","msg":"extauth server running in [gRPC] mode, listening at [:8083]"}
```

#### Create a simple Virtual Service

To test our auth plugin, we first need to create an upstream. Let's start by creating a simple service that will return
"Hello World" when receiving HTTP requests:

```bash
kubectl apply -f - <<EOF
apiVersion: apps/v1
kind: Deployment
metadata:
  labels:
    app: http-echo
  name: http-echo
spec:
  selector:
    matchLabels:
      app: http-echo
  replicas: 1
  template:
    metadata:
      labels:
        app: http-echo
    spec:
      containers:
      - image: hashicorp/http-echo:latest
        name: http-echo
        args: ["-text='Hello World!'"]
        ports:
        - containerPort: 5678
          name: http
---
apiVersion: v1
kind: Service
metadata:
  name: http-echo
  labels:
    service: http-echo
spec:
  ports:
  - port: 5678
    protocol: TCP
  selector:
    app: http-echo
EOF
```

Now we can create a Virtual Service that will route any requests with the `/echo` prefix to the `echo` service.

{{< tabs >}}
{{< tab name="yaml" codelang="yaml">}}
{{< readfile file="gloo_routing/virtual_services/security/plugin_auth/vs-echo-no-auth.yaml">}}
{{< /tab >}}
{{< tab name="glooctl" codelang="shell">}}
glooctl create vs --name http-echo --namespace gloo-system
glooctl add route --path-prefix /echo --dest-name default-http-echo-5678
{{< /tab >}}
{{< /tabs >}}

To verify that the Virtual Service works, let's get the URL of the Gloo Gateway and send a request to `/echo`:

```bash
export GATEWAY_URL=$(glooctl proxy url --name gateway-proxy-v2)
```

```bash
curl $GATEWAY_URL/echo
'Hello World!'
```

#### Secure the Virtual Service

Gloo does not yet perform any authentication for the route we just defined. To configure it to authenticate the requests
served by the virtual host that the route belongs to, we need to add the following to the Virtual Service definition:

{{< highlight yaml "hl_lines=20-34" >}}
apiVersion: gateway.solo.io/v1
kind: VirtualService
metadata:
name: echo
namespace: gloo-system
spec:
displayName: echo
virtualHost:
domains: - '\*'
routes: - matcher:
prefix: /echo
routeAction:
single:
upstream:
name: default-http-echo-5678
namespace: gloo-system
virtualHostPlugins:
extensions:
configs:
extauth:
plugin_auth:
plugins: - config:
RequiredHeader: my-header
AllowedValues: - foo - bar - baz
name: RequiredHeader
plugin_file_name: RequiredHeader.so
exported_symbol_name: Plugin
{{< /highlight >}}

This configures the virtual host to authenticate requests using the `plugin_auth` mode. `plugins` is an array of plugins
(in Gloo terms a **plugin chain**), where each element has the following structure:

- `name`: the name of the plugin. This serves mainly for display purposes, but is also used to build the default values
  for the next two fields.
- `plugin_file_name`: the name of the compiled plugin that was copied to the `/auth-plugins` directory. Defaults to `<name>.so`.
- `exported_symbol_name`: the name of the exported symbol Gloo will look for when loading the plugin. Defaults to `<name>`.
- `config`: information that will be used to configure your plugin. Gloo will attempt to parse the value of this
  attribute into the object pointer returned by your plugin's `NewConfigInstance` function implementation. In our case
  this will be an instance of `*Config`, as seen in the
  _Building an Ext Auth plugin_ [section]({{< versioned_link_path fromRoot="/gloo_routing/virtual_services/security/plugin_auth#building-an-ext-auth-plugin" >}}).

{{% notice note %}}
Plugins in a **plugin chain** will be executed in the order they are defined. The first plugin to deny the request will
cause the chain execution to be interrupted. The headers on each plugin response will be merged into the request to the
next one. Check the [plugin developer guide]({{< versioned_link_path fromRoot="/dev/writing_auth_plugins#plugin-chains-and-header-propagation" >}})
for more information about how the headers are merged.
{{% /notice %}}

After `apply`-ing this Virtual Service, let's check the `extauth` logs again:

{{< highlight yaml "hl_lines=4-6" >}}
kc logs -n gloo-system deployment/extauth
{"level":"info","ts":1566316248.9934704,"logger":"extauth","caller":"runner/run.go:78","msg":"Starting ext-auth server"}
{"level":"info","ts":1566316248.9935324,"logger":"extauth","caller":"runner/run.go:93","msg":"extauth server running in grpc mode, listening at :8083"}
{"level":"info","ts":1566316249.0016804,"logger":"extauth","caller":"runner/run.go:150","msg":"got new config","config":[{"vhost":"gloo-system.gateway-proxy-v2-listener-::-8080-gloo-system_echo","AuthConfig":{"PluginAuth":{"plugins":[{"name":"RequiredHeader","plugin_file_name":"RequiredHeader.so","exported_symbol_name":"Plugin","config":{"fields":{"AllowedValues":{"Kind":{"ListValue":{"values":[{"Kind":{"StringValue":"foo"}},{"Kind":{"StringValue":"bar"}},{"Kind":{"StringValue":"baz"}}]}}},"RequiredHeader":{"Kind":{"StringValue":"my-header"}}}}}]}}}]}
{"level":"info","ts":1566316249.0287502,"logger":"extauth.header_value_plugin","caller":"pkg/impl.go:38","msg":"Parsed RequiredHeaderAuthService config","requiredHeader":"my-header","allowedHeaderValues":["foo","bar","baz"]}
{"level":"info","ts":1566316249.0289364,"logger":"extauth","caller":"plugins/loader.go:85","msg":"Successfully loaded plugin. Adding it to the plugin chain.","pluginName":"RequiredHeader"}
{{< /highlight >}}

From the last three lines we can see that the Ext Auth server received the new configuration for our Virtual Service.
If we try to hit our route again, we should see a `403` response:

```bash
curl -v $GATEWAY_URL/echo
*   Trying 192.168.99.100...
* TCP_NODELAY set
* Connected to 192.168.99.100 (192.168.99.100) port 30519 (#0)
> GET /echo HTTP/1.1
> Host: 192.168.99.100:30519
> User-Agent: curl/7.54.0
> Accept: */*
>
< HTTP/1.1 403 Forbidden
< date: Tue, 20 Aug 2019 15:01:57 GMT
< server: envoy
< content-length: 0
<
* Connection #0 to host 192.168.99.100 left intact
```

If you recall the structure of our plugin, it will only allow request with a given header (in this case `my-header`) and
where that header has an expected value (in this case one of `foo`, `bar` or `baz`). If we include a header with these
properties in our request, we will be able to hit our `echo` service:

{{< highlight bash "hl_lines=20" >}}
curl -v -H "my-header: foo" \$GATEWAY_URL/echo

- Trying 192.168.99.100...
- TCP_NODELAY set
- Connected to 192.168.99.100 (192.168.99.100) port 30519 (#0)

- GET /echo HTTP/1.1
- Host: 192.168.99.100:30519
- User-Agent: curl/7.54.0
- Accept: _/_
- my-header: foo

- < HTTP/1.1 200 OK
- < x-app-name: http-echo
- < x-app-version: 0.2.3
- < date: Tue, 20 Aug 2019 16:02:12 GMT
- < content-length: 15
- < content-type: text/plain; charset=utf-8
- < x-envoy-upstream-service-time: 0
- < server: envoy
- <
- 'Hello World!'

- Connection #0 to host 192.168.99.100 left intact
  {{< /highlight >}}

## Summary

In this guide we installed Enterprise Gloo and configured it to load a sample Go plugin that implements custom
auth logic. Then we created a simple Virtual Service to route requests to a test upstream. Finally, we updated the
Virtual Service to use our plugin and saw how requests are allowed or denied based on the custom configuration for our
plugin.

You can cleanup the resources created while following this guide by running:

```bash
glooctl uninstall -n gloo-system
```

## Next steps

As a next step, check out our [plugin developer guide]({{< versioned_link_path fromRoot="/dev/writing_auth_plugins" >}}) for a detailed
tutorial on how to build your own external auth plugins!
