---
title: Plugin Auth
weight: 90
description: Extend Gloo Edge's built-in auth server with custom Go plugins
---

{{% notice note %}}
This feature was introduced with **Gloo Edge Enterprise**, release 0.18.11. If you are using an earlier version, this tutorial will not work.
{{% /notice %}}

We have seen that one way of implementing custom authentication logic is by [providing your own auth server]({{< versioned_link_path fromRoot="/guides/security/auth/custom_auth" >}}). While this approach gives you great freedom, it also comes at a cost: 

- You have to write and manage an additional service. If your authentication logic is simple, the plumbing needed to get it running might constitute a significant overhead.
- If your custom auth server serves only as an adapter for your existing auth service, it will introduce an additional network hop with the associated latency to any request that needs to be authenticated; either that, or you will most likely need to update your existing service so that it can conform to the request/response format expected by Gloo Edge.
- You likely want to be able to define configuration for your auth server and that configuration should be context-specific, e.g. based on which virtual host is serving the request. You could store this configuration outside of Gloo Edge (e.g. in a dedicated Kubernetes CRD) and try to derive the context from the request itself, but this would introduce additional latency and negate the benefits of the centralized control plane that Gloo Edge provides.

Wouldn't it be nice to be able to **write just the authentication logic you need, plug it into Gloo Edge, and be able to provide your specific configuration** right on the Virtual Services it applies to? Starting with **Gloo Edge Enterprise**, release **0.18.11+**, you can do just that! 

In this guide we will show you how easy it is to extend Gloo Edge's Ext Auth server via [Go plugins](https://golang.org/pkg/plugin/).

{{% notice warning %}}
Beware of encountering potential bugs in the go plugin runtime if you're running on an old version of linux (e.g., the linux installed by kops defaults in kops 1.15.1). If so, you may see go fail to execute the `dlopen` C function call, manifesting as:
```shell script
{"level":"error","ts":1582578689.0415232,"logger":"extauth.ext-auth-service","caller":"service/extauth.go:58","msg":"Error while authorizing request","error":"empty config set","stacktrace":...}
```
Even when your auth service has properly been configured, seeing a similar log line to this earlier in your logs:
```shell script
{"level":"info","ts":1582578688.078688,"logger":"extauth","caller":"runner/run.go:158","msg":"got new config","config":[{"auth_config_ref_name":"gloo-system.plugin-auth","configs":[{"AuthConfig":{"plugin_auth":{"name":"RequiredHeader","plugin_file_name":"RequiredHeader.so","exported_symbol_name":"Plugin","config":{"fields":{"AllowedValues":{"Kind":{"list_value":{"values":[{"Kind":{"string_value":"foo"}},{"Kind":{"string_value":"bar"}}]}}},"RequiredHeader":{"Kind":{"string_value":"my-auth-header"}}}}}}}]}]}
```
To resolve this error, we suggest upgrading your host linux OS.
{{% /notice %}}

## Development workflow overview

Following are the high-level steps required to use your auth plugins with Gloo Edge. Through the course of this guide we will see each one of them in greater detail.

1. Write a plugin and publish it as a `docker image` which, when run, copies the compiled plugin file(s) to a predefined directory.
1. Configure Gloo Edge to load the plugins by running the image as an `initContainer` on the `extauth` deployment. This can be done by installing Gloo Edge with [dedicated value overrides](#installation) or by modifying the Gloo Edge YAML manifest manually.
1. Reference your plugin in your Virtual Services for it to be invoked for requests matching particular virtual hosts or routes.

{{% notice note %}}
For a more in-depth explanation of the Ext Auth Plugin development workflow, please check our dedicated [**Plugin Developer Guide**]({{< versioned_link_path fromRoot="/guides/dev/writing_auth_plugins" >}}).
{{% /notice %}}

## Building an Ext Auth plugin

{{% notice note %}}
The code used in this section can be found in our [**ext-auth-plugin-examples**](https://github.com/solo-io/ext-auth-plugin-examples) GitHub repository.
{{% /notice %}}

The [official Go docs](https://golang.org/pkg/plugin/) define a plugin as:

>> "*a Go main package with exported functions and variables that has been built with: `go build -buildmode=plugin`*"

In order for Gloo Edge to be able to load your plugin, the plugin's `main` package must export a variable that implements the [ExtAuthPlugin](https://github.com/solo-io/ext-auth-plugins/blob/master/api/interface.go#L61) interface:

```go
type ExtAuthPlugin interface {
	NewConfigInstance(ctx context.Context) (configInstance interface{}, err error)
	GetAuthService(ctx context.Context, configInstance interface{}) (AuthService, error)
}
```

Check the [plugin developer guide]({{< versioned_link_path fromRoot="/guides/dev/writing_auth_plugins#api-overview" >}}) for a detailed explanation of the API.

For this guide we will use a simple example plugin that has already been built. You can find the source code to build it yourself [here](https://github.com/solo-io/ext-auth-plugin-examples/tree/master/plugins/required_header).

The plugin authorizes requests if:
 
1. they contain a certain header, and
1. the value for the header is in a predefined whitelist.

The `main` package for the plugin looks like this:

{{< highlight go "hl_lines=10-11" >}}
package main

import (
	"github.com/solo-io/ext-auth-plugins/api"
	impl "github.com/solo-io/ext-auth-plugins/plugins/required_header/pkg"
)

func main() {}

// This is the exported variable that Gloo Edge will look for. It implements the ExtAuthPlugin interface.
var Plugin impl.RequiredHeaderPlugin
{{< /highlight >}}

We leave it up to you to inspect the simple `impl.RequiredHeaderPlugin` object. Of interest here is the following configuration object it uses:

```go
type Config struct {
	RequiredHeader string
	AllowedValues  []string
}
```

The values in this struct will determine the aforementioned header and whitelist.

#### Packaging and publishing the plugin

Ext Auth plugins must be made available to Gloo Edge in the form of container images. The images must contain the compiled plugins and copy these files to the `/auth-plugins` when they are run.

In this guide we will use the image for the `RequiredHeaderPlugin` introduced above. It has been built using [this Dockerfile](https://github.com/solo-io/ext-auth-plugin-examples/blob/master/Dockerfile) and can be found in the `quay.io/solo-io/ext-auth-plugins` docker repository. We publish a new version of this plugin with each Gloo Edge Enterprise release to ensure compatibility with that release. Let's inspect the plugin image for version `0.20.6`:

{{< highlight shell_script "hl_lines=3" >}}
docker run -it --entrypoint ls quay.io/solo-io/ext-auth-plugins:0.20.6 -l compiled-auth-plugins
total 28356
-rw-r--r--    1 root     root      29033320 Oct 25 21:41 RequiredHeader.so
{{< /highlight >}}

You can see that it contains the compiled plugin file `RequiredHeader.so`.

{{% notice warning %}}
Make sure the plugin image tag matches the version of Gloo Edge Enterprise you are using. Plugins with mismatching versions will fail to be loaded due to the compatibility constraints imposed by the Go plugin model. See [this section]({{< versioned_link_path fromRoot="/guides/dev/writing_auth_plugins#build-helper-tools" >}}) of the plugin developer guide for more information.
{{% /notice %}}

## Configuring Gloo Edge

#### Installation
Let's start by installing Gloo Edge Enterprise. We need to customize the standard installation to configure Gloo Edge to use our plugin; we can do that by defining the following `plugin-values.yaml` value file:

{{< highlight bash "hl_lines=6-11" >}}
cat << EOF > plugin-values.yaml
global:
  extensions:
    extAuth:
      plugins:
        my-plugin:
          image:
            repository: ext-auth-plugins
            registry: quay.io/solo-io
            pullPolicy: IfNotPresent
            tag: 1.3.0 # change this to your Gloo Edge Enterprise installation version
EOF
{{< /highlight >}}

In the above file, `global.extensions.extAuth.plugins` is a map where:

* each key is an arbitrary string that will be used to identify the plugin container (in this case `my-plugin`)
* the correspondent value is a reference to a container image that contains the plugin file

{{% notice note %}}
We will install using `glooctl`, but you can use this same value file to install the [Gloo Edge Helm chart]({{% versioned_link_path fromRoot="/installation/enterprise#installing-on-kubernetes-with-helm" %}}).
{{% /notice %}}

Now we can use this file to install Gloo Edge:

```bash
glooctl install gateway enterprise --license-key $GLOO_LICENSE_KEY --values plugin-values.yaml
```

If we now inspect the `extauth` deployment by running

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
      containers:
      - image: quay.io/solo-io/extauth-ee:1.3.0
        imagePullPolicy: IfNotPresent
        name: extauth
        resources: {}
        volumeMounts:
        - mountPath: /auth-plugins
          name: auth-plugins
      initContainers:
      - image: quay.io/solo-io/ext-auth-plugins:1.3.0
        imagePullPolicy: IfNotPresent
        name: plugin-my-plugin
        volumeMounts:
        - mountPath: /auth-plugins
          name: auth-plugins
      volumes:
      - emptyDir: {}
        name: auth-plugins
{{< /highlight >}}

Each plugin container image built as described in the *Packaging and publishing the plugin* [section]({{< versioned_link_path fromRoot="/guides/security/auth/extauth/plugin_auth#packaging-and-publishing-the-plugin" >}}) has been added as an `initContainer` to the `extauth` deployment. A volume named `auth-plugins` is mounted in the `initContainer`s and the `extauth` container at `/auth-plugins` path: when the `initContainer`s are run, they will copy the compiled plugin files they contain (in this case `RequiredHeader.so`) to the shared volume, where they become available to the `extauth` server.

Let's verify that the `extauth` server did successfully start by checking its logs.

```bash
kubectl logs -n gloo-system deployment/extauth
```
returns
```
{"level":"info","ts":1573567317.1261566,"logger":"extauth","caller":"runner/run.go:86","msg":"Starting ext-auth server"}
{"level":"info","ts":1573567317.1262844,"logger":"extauth","caller":"runner/run.go:105","msg":"extauth server running in [gRPC] mode, listening at [:8083]"}
```

#### Create a simple Virtual Service
Let's deploy a sample application that we will route requests to when testing our auth plugin:

```shell
kubectl apply -f https://raw.githubusercontent.com/solo-io/gloo/v1.14.x/example/petstore/petstore.yaml
```

### Create a Virtual Service
Now we can create a Virtual Service that routes all requests (note the `/` prefix) to the `petstore` service.

```yaml
apiVersion: gateway.solo.io/v1
kind: VirtualService
metadata:
  name: petstore
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
          kube:
            ref:
              name: petstore
              namespace: default
            port: 8080
```

To verify that the Virtual Service works, let's send a request to `/api/pets`:

```shell
curl $(glooctl proxy url)/api/pets
```

You should see the following output:

```json
[{"id":1,"name":"Dog","status":"available"},{"id":2,"name":"Cat","status":"pending"}]
```

#### Secure the Virtual Service
{{% notice warning %}}
{{% extauth_version_info_note %}}
{{% /notice %}}

As we just saw, we were able to reach the upstream without having to provide any credentials. This is because by default Gloo Edge allows any request on routes that do not specify authentication configuration. Let's change this behavior. We will update the Virtual Service so that only requests containing a header named `my-header` whose value is either `foo` or `bar` are allowed.

##### Create an AuthConfig resource
Let's start by creating and `AuthConfig` CRD with that will use our plugin:

{{< highlight shell "hl_lines=8-16" >}}
apiVersion: enterprise.gloo.solo.io/v1
kind: AuthConfig
metadata:
  name: plugin-auth
  namespace: gloo-system
spec:
  configs:
  - pluginAuth:
      name: RequiredHeader
      pluginFileName: RequiredHeader.so
      exportedSymbolName: Plugin
      config:
        RequiredHeader: my-header
        AllowedValues:
        - foo
        - bar
{{< /highlight >}}

When referenced on a Virtual Service, this configuration will instruct Gloo Edge to authenticate requests using our plugin. Let's examine the structure of the `pluginAuth` object:

- `name`: the name of the plugin. This serves mainly for display purposes, but is also used to build the default values 
for the next two fields.
- `pluginFileName`: the name of the compiled plugin that was copied to the `/auth-plugins` directory. Defaults to `<name>.so`.
- `exportedSymbolName`: the name of the exported symbol Gloo Edge will look for when loading the plugin. Defaults to `<name>`.
- `config`: information that will be used to configure your plugin. Gloo Edge will attempt to parse the value of this attribute into the object pointer returned by your plugin's `NewConfigInstance` function implementation. In our case this will be an instance of `*Config`, as seen in the *Building an Ext Auth plugin* [section]({{< versioned_link_path fromRoot="/guides/security/auth/extauth/plugin_auth#building-an-ext-auth-plugin" >}}).

{{% notice note %}}
You may have noticed that the `configs` attribute in the above `AuthConfig` is an array. It is possible to define multiple steps in an `AuthConfig`. Steps will be executed in the order they are defined. The first config to deny a request will cause the execution to be interrupted and a response to be returned to the downstream client. The headers produced by each step will be merged into the request to the next one. Check the [plugin developer guide]({{< versioned_link_path fromRoot="/guides/dev/writing_auth_plugins#multi-step-authconfigs" >}}) for more information about how the headers are merged.
{{% /notice %}}

##### Update the Virtual Service
Once the `AuthConfig` has been created, we can use it to secure our Virtual Service:

{{< highlight yaml "hl_lines=20-34" >}}
apiVersion: gateway.solo.io/v1
kind: VirtualService
metadata:
  name: petstore
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
          kube:
            ref:
              name: petstore
              namespace: default
            port: 8080
    options:
      extauth:
        configRef:
          name: plugin-auth
          namespace: gloo-system
{{< /highlight >}}

After `apply`-ing this Virtual Service, let's check the `extauth` logs again:

```shell script
kubectl logs -n gloo-system deployment/extauth
```

returns

{{< highlight yaml "hl_lines=3-4" >}}
{"level":"info","ts":1573567317.1261566,"logger":"extauth","caller":"runner/run.go:86","msg":"Starting ext-auth server"}
{"level":"info","ts":1573567317.1262844,"logger":"extauth","caller":"runner/run.go:105","msg":"extauth server running in [gRPC] mode, listening at [:8083]"}
{"level":"info","ts":1573574476.3094413,"logger":"extauth","caller":"runner/run.go:160","msg":"got new config","config":[{"auth_config_ref_name":"gloo-system.plugin-auth","AuthConfig":null,"configs":[{"AuthConfig":{"plugin_auth":{"name":"RequiredHeader","plugin_file_name":"RequiredHeader.so","exported_symbol_name":"Plugin","config":{"fields":{"AllowedValues":{"Kind":{"ListValue":{"values":[{"Kind":{"StringValue":"foo"}},{"Kind":{"StringValue":"bar"}}]}}},"RequiredHeader":{"Kind":{"StringValue":"my-header"}}}}}}}]}]}
{"level":"info","ts":1573574476.3579354,"logger":"extauth.header_value_plugin","caller":"pkg/impl.go:39","msg":"Parsed RequiredHeaderAuthService config","requiredHeader":"my-header","allowedHeaderValues":["foo","bar"]}
{{< /highlight >}}

From the last two lines we can see that the Ext Auth server received the new configuration for our Virtual Service. 

## Test our configuration
If we try to hit our route again, we should see a `403` response:

```shell script
curl -v $(glooctl proxy url)/api/pets
```

returns

{{< highlight shell "hl_lines=6" >}}
> GET /api/pets HTTP/1.1
> Host: 192.168.99.100:30834
> User-Agent: curl/7.54.0
> Accept: */*
>
< HTTP/1.1 403 Forbidden
< date: Tue, 12 Nov 2019 16:03:42 GMT
< server: envoy
< content-length: 0
<
{{< /highlight >}}

If you recall the structure of our plugin, it will only allow request with a given header (in this case `my-header`) and where that header has an expected value (in this case one of `foo` or `bar`). If we include a header with these properties in our request, we will be able to hit our sample service:

```shell script
curl -v -H "my-header: foo" $(glooctl proxy url)/api/pets
```

{{< highlight shell "hl_lines=7 14" >}}
> GET /api/pets HTTP/1.1
> Host: 192.168.99.100:30834
> User-Agent: curl/7.54.0
> Accept: */*
> my-header: foo
>
< HTTP/1.1 200 OK
< content-type: application/xml
< date: Tue, 12 Nov 2019 16:05:05 GMT
< content-length: 86
< x-envoy-upstream-service-time: 3
< server: envoy
<
[{"id":1,"name":"Dog","status":"available"},{"id":2,"name":"Cat","status":"pending"}]
{{< /highlight >}}

## Summary
In this guide we installed Gloo Edge Enterprise and configured it to load a sample Go plugin that implements custom auth logic. Then we created a simple Virtual Service to route requests to a test upstream. Finally, we updated the Virtual Service to use our plugin and saw how requests are allowed or denied based on the custom configuration for our 
plugin.

You can cleanup the resources created while following this guide by running:
```bash
glooctl uninstall --all
kubectl delete -f https://raw.githubusercontent.com/solo-io/gloo/v1.14.x/example/petstore/petstore.yaml
```

## Next steps
As a next step, check out our [plugin developer guide]({{< versioned_link_path fromRoot="/guides/dev/writing_auth_plugins" >}}) for a detailed tutorial on how to build your own external auth plugins!
