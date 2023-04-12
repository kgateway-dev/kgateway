---
title: "Building External Auth Plugins"
weight: 7
description: Guidelines and best practices for developing and configuring Go plugins to extend Gloo Edge's ext auth server
---

In the [**Plugin Auth** guide]({{% versioned_link_path fromRoot="/guides/security/auth/extauth/plugin_auth" %}}) 
we showed how easy it is to extend Gloo Edge with custom authentication logic using Go plugins. That guide uses a 
[plugin](https://github.com/solo-io/ext-auth-plugin-examples/tree/master/plugins/required_header) that has already been 
built and published, and primarily focuses on giving an overview of the plugin development workflow.

In this guide, we will get our hands dirty and dig into the nitty-gritty details of how to write, test, build, and 
publish your external auth plugins.

## Table of Contents
- [Before you start](#before-you-start)
- [Building and publishing and auth plugin](#building-and-publishing-and-auth-plugin)
    - [API Overview](#api-overview)
        - [`ExtAuthPlugin`](#extauthplugin)
        - [`AuthService`](#authservice)
    - [`AuthService` lifecycle](#about-the-authservice-lifecycle)
    - [How to make your plugin implement `ExtAuthPlugin`](#about-the-authservice-lifecycle)
    - [Build helper tools](#build-helper-tools)
        - [Compare dependencies](#compare-dependencies)
        - [Verify compatibility script](#verify-compatibility-script)
        - [Dockerfile](#dockerfile)
- [Configuring Gloo Edge to load your plugins](#configuring-gloo-edge-to-load-your-plugins)
    - [Configuring Virtual Services to use your plugins](#configuring-virtual-services-to-use-your-plugins)
- [Multi-step AuthConfigs](#multi-step-authconfigs)
    - [Header propagation](#header-propagation)
    - [Sharing state between steps](#sharing-state-between-steps)

## Before you start
This guide will make frequent references to the code contained in our 
[Ext Auth Plugin examples](https://github.com/solo-io/ext-auth-plugin-examples) GitHub repository. In addition to the sample 
plugin implementation, the repository contains useful tools to verify whether your plugin is compatible with a certain 
version of Gloo Edge Enterprise. Given the [constraints imposed by Go plugins](#build-helper-tools), these utilities will 
significantly improve the experience of developing external auth plugins.

{{% notice note %}}
We recommend that you fork the example repository and use it as a starting point to develop your plugins.
{{% /notice %}}

### Development workflow overview
In the [**Plugin Auth** guide]({{% versioned_link_path fromRoot="/guides/security/auth/extauth/plugin_auth#development-workflow-overview" %}}) 
we gave a high-level description of the steps required to extend Gloo Edge with your own plugins:

1. Write a plugin and publish it as a `docker image` which, when run, copies the compiled plugin file to a 
predefined directory.
2. Configure Gloo Edge to load the plugin by running the image as an `initContainer` on the `extauth` deployment. This can be 
done by installing Gloo Edge with [dedicated value overrides]({{% versioned_link_path fromRoot="/guides/security/auth/extauth/plugin_auth#installation" %}}) 
or by modifying the Gloo Edge installation manifest manually.
3. Reference your plugin in your Virtual Services for it to be invoked for requests matching particular virtual hosts or 
routes.

In the following sections we will see each one of them in greater detail.

## Building and publishing and auth plugin
In this section we will see how to develop an auth plugin and distribute it the format that Gloo Edge expects.

### API overview
When developing external auth plugins, there are two interfaces we need to be familiar with. They are both defined 
[here](https://github.com/solo-io/ext-auth-plugins/blob/master/api/interface.go).

##### ExtAuthPlugin
Gloo Edge expects auth plugins to implement the 
[ExtAuthPlugin](https://github.com/solo-io/ext-auth-plugins/blob/master/api/interface.go#L41) interface.

```go
type ExtAuthPlugin interface {
	NewConfigInstance(ctx context.Context) (configInstance interface{}, err error)
	GetAuthService(ctx context.Context, configInstance interface{}) (AuthService, error)
}
```

Objects that implement this interface are used as factories for authentication service instances. After Gloo Edge detects a 
reference to your plugin on a Virtual Service and loads it, it will call the `NewConfigInstance` function to get an 
object to deserialize the plugin configuration into. 

{{% notice warning %}}
The object returned by the `NewConfigInstance` function **MUST** be a pointer type.
{{% /notice %}}

Let's see an example to understand this better. If the `AuthConfig` for your plugin looks like this 
(see [this section]({{% versioned_link_path fromRoot="/guides/security/auth/extauth/plugin_auth#create-an-authconfig-resource" %}}) 
of the documentation for an explanation of the fields below):

{{< highlight shell "hl_lines=8-16" >}}
apiVersion: enterprise.gloo.solo.io/v1
kind: AuthConfig
metadata:
  name: plugin-auth
  namespace: gloo-system
spec:
  configs:
  - pluginAuth:
      name: my-plugin
      pluginFileName: MyPlugin.so
      exportedSymbolName: Plugin
      config:
        someKey: value-1
        someStruct:
          another_key: value-2
{{< /highlight >}}

then `NewConfigInstance` function of your `ExtAuthPlugin` implementation should return a pointer to the following Go struct:

```go
type MyPluginConfig struct {
    SomeKey string
    SomeStruct NestedConfig
}

type NestedConfig struct {
    AnotherKey string
 }
```

Gloo Edge will populate the struct fields with the values found on the correspondent YAML attributes. 

{{% notice note %}}
You might have noticed that the `configs` attribute in the configuration example above is an array. It is in fact 
possible to define multiple configuration in the same `AuthConfig`. We'll see how this works 
[later](#multi-step-authconfigs).
{{% /notice %}}

The `GetAuthService` function will be invoked by Gloo Edge right after this step. As its `configInstance` argument, Gloo Edge will 
pass the object that it just populated with the values from the plugin configuration. This function must return an 
instance of the [AuthService](https://github.com/solo-io/ext-auth-plugins/blob/master/api/interface.go#L49) interface.

##### AuthService
`AuthService` instances are responsible for authorizing individual requests. This is the interface that all of Gloo Edge's 
out-of-the box auth implementations (basic auth, OIDC, etc.) implement as well. Your plugin is responsible for providing 
Gloo Edge with a valid `AuthService` implementation.

```go
type AuthService interface {
	Start(ctx context.Context) error
	Authorize(ctx context.Context, request *AuthorizationRequest) (*AuthorizationResponse, error)
}
```

The `Start` function will be called once by Gloo Edge, when the auth service is started. It is intended as a hook to perform 
initialization logic or to start auxiliary processes that span the whole lifecycle of the service.

All the functions we have just described (`NewConfigInstance`, `GetAuthService`, and `Start`) will be invoked when Gloo Edge 
detects a new auth configuration on your Virtual Services. The `Authorize` function, on the other hand, will be invoked 
each time a request hits Gloo Edge and matches the virtual host on which the your plugin is defined. 
The `AuthorizationResponse` that it returns will determine whether the request will be allowed or denied. 
We provide minimal responses of both types via the `AuthorizedResponse()` and `UnauthorizedResponse()` 
functions in [the same package](https://github.com/solo-io/ext-auth-plugins/blob/master/api/interface.go#L114-L134). 
You can use them as a basis for your own responses.

#### About the AuthService lifecycle
We mentioned how `ExtAuthPlugin` implementations function as factories for `AuthService` instances. It's worth spending 
a few words on the lifecycle of `AuthService`s. You might have noticed that Gloo Edge passes a `context.Context` to each of 
the functions we just saw. The context will live as long as the plugin configuration that generated it is valid. 
Whenever the auth configuration changes, Gloo Edge will start new `AuthService` instances and signal the termination of the 
previous ones by cancelling the context it provided them with.

Assuming we start with a clean sheet, i.e. no `AuthConfig` resources are referenced on any of your Virtual Services, 
the following is the sequence of actions that the Gloo Edge external auth service performs when it detects a change in an 
auth configuration. The service:

1. starts a new cancellable `context.Context`
1. loops over all detected `configs` in the `AuthConfig` and for each one, if it is a plugin:
    1. loads the correspondent plugin `.so` file from the `auth-plugins` directory (more info about this [later](#configuring-gloo-edge-to-load-your-plugins))
    2. invokes `NewConfigInstance` **passing in the context**
    3. deserializes the detected plugin config into the provided object
    4. invokes `GetAuthService` **passing in the context** and the configuration object
1. if an error occurred, it returns and does not update the `extauth` server configuration, else it continues
1. cancels the previous `context.Context`
1. invokes the `Start` functions on all plugins **passing in the context**
1. applies the plugin configurations to the `extauth` server state.

We recommend that you tie all the goroutines that you may spawn to the provided context by watching its `Done` channel. 
This will prevent your plugin from leaking memory. You can find a great overview of `Context` and how to best use it 
in [this Go Blog post](https://blog.golang.org/context).

#### How to make your plugin implement `ExtAuthPlugin`
Earlier in this guide we mentioned that Gloo Edge expects auth plugins to implement the 
[ExtAuthPlugin](https://github.com/solo-io/ext-auth-plugins/blob/master/api/interface.go#L62) interface. To understand 
what we mean by that, let's take a closer look at how Go plugins work.

The [official Go docs](https://golang.org/pkg/plugin/) describe a plugin as:

>> "*a Go main package with exported functions and variables that has been built with: `go build -buildmode=plugin`*"

In order for Gloo Edge to be able to load your plugin, the `main` package of your plugin must export a variable that 
implements the [ExtAuthPlugin](https://github.com/solo-io/ext-auth-plugins/blob/master/api/interface.go#L62) interface. 
This is usually a struct or a pointer to a struct (Gloo Edge is smart enough to handle both cases). 
Gloo Edge will use the [Lookup function](https://golang.org/pkg/plugin/#Plugin.Lookup) to find the exported variable and 
assert that it in fact implements the expected interface.

You can specify the name of the variable Gloo Edge looks for when you reference your plugin in your Virtual Services:

{{< highlight yaml "hl_lines=5" >}}
plugin_auth:
  plugins:
  - name: my-plugin
    plugin_file_name: MyPlugin.so
    exported_symbol_name: Plugin
    config: {}
{{< /highlight >}}

See the [*Plugin Auth* guide]({{% versioned_link_path fromRoot="/guides/security/auth/extauth/plugin_auth#secure-the-virtual-service" %}}) 
for more information about the structure of this piece of configuration. 

### Build helper tools
Now that we saw how to write your plugin, it's time to look at how to build it. When working with plugins, building your 
code is not as straightforward as when working with regular Go programs. Go plugins impose a set of **pretty harsh 
constraints on your build environment** for plugins work with the program that is supposed to load them:

1. The plugin Go compiler version must exactly match the program's compiler version. For example, loading a plugin 
compiled with Go 1.12.9 will not work with a program compiled with 1.12.8. The flags provided during compilation must match 
as well.
2. Any libraries that are shared by both the plugin and the program must have their versions match *exactly*. Moreover, 
these libraries must be compiled with the same `GOPATH` value when building both the plugin and the program.
3. Both the plugins and the program need to be built with CGO_ENABLED=1. Cross compiling isn't as easy as 
`GOARCH=amd64 GOOS=linux go build` anymore.

If any of the above conditions is not met, loading the plugin will fail.

All these constraints can make plugin development pretty frustrating. This is why we provide a set of tools that are 
meant make your plugin development experience as smooth as possible. You can find these tools in our 
[Ext Auth Plugin examples](https://github.com/solo-io/ext-auth-plugin-examples) GitHub repository.

{{% notice note %}}
Gloo Edge publishes information about the environment it was built with to a Google Storage bucket. The tools in this section 
will make use of this information. You can find the information for a specific Gloo Edge version in the following files located 
under `http://storage.googleapis.com/gloo-ee-dependencies/[GLOOE_VERSION]`:

- `dependencies`: contains the versions of all dependencies used by Gloo Edge (generated by running `go list -m all` on the Gloo Edge Enterprise module)
- `build_env`: values that can be used to replicate the environment the given Gloo Edge version was built in.
- `verify-plugins-linux-amd64`: a script to verify that the plugin can be loaded by the given Gloo Edge version.

You can get all these files by running `GLOOE_VERSION=desired_version make get-glooe-info` in our example repository.
{{% /notice %}}

#### Compare dependencies
We manage Gloo Edge dependencies using [Go Modules](https://github.com/golang/go/wiki/Modules). The `go.mod` file contains 
constraints that you impose on your module dependencies, but it does not provide complete information about all the 
dependencies. This is why we also publish the output of `go list -m all` in the `dependencies` file mentioned in the 
previous section.
When you develop your plugins, we suggest that you use Go Modules for dependency management. This way you will be able 
to take advantage of a script we provide for comparing the dependencies of your plugin with the Gloo Edge ones. 
It is located at `scripts/compare_dependencies.go` and can be invoked via the following `make` command:

```bash
GLOOE_VERSION=desired_version make resolve-deps
```

If all dependencies match, the command will exit with a zero code, else it will output the discrepancies to both stdout 
and to a file (`mismatched_dependencies.json`) and exit with code 1. Here's an example of the output in case of failure:

```json
[
  {
    "message": "Please pin your dependency to the same version as the Gloo Edge one using a [require] clause",
    "pluginDependencies": {
      "name": "go.uber.org/zap",
      "version": "v1.12.0",
      "replacement": false
    },
    "glooDependencies": {
      "name": "go.uber.org/zap",
      "version": "v1.13.0",
      "replacement": false
    }
  }
]
```

If you get an error message like this, you have to manually update the dependencies in your `go.mod` file. In this 
case we would need to add the following entry to the `replace` section of the `go.mod` file:

```
go.uber.org/zap v1.13.0
```

For convenience, in case of failure the script also outputs a file named `suggestions`, which contains an entry for 
every mismatched dependency; you can use these entries to update your `go.mod` file. Given the example above, the 
`suggestion` file would look like this:

```
require (
	// Add the following entries to the 'require' section of your go.mod file:
	go.uber.org/zap v1.13.0
)
```

If you are using a different dependency management tool (e.g. [dep](https://github.com/golang/dep)), you should still 
be able to use the information in the Gloo Edge `dependencies` file to verify that the dependencies match.

{{% notice note %}}
Please see [this section](https://github.com/solo-io/ext-auth-plugin-examples#compare-deps) of the README in the examples 
repository for more information about the dependency comparison script and a description of the different kinds of 
mismatches that can occur.
{{% /notice %}}
  
#### Verify compatibility script
As part of each Gloo Edge Enterprise release, we ship a script to verify whether your plugin can be loaded by that version of 
Gloo Edge Enterprise. You can find it in the aforementioned Google Cloud bucket at 
`http://storage.googleapis.com/gloo-ee-dependencies/[GLOOE_VERSION]/verify-plugins-linux-amd64`. The script accepts 
three arguments:

| Arg Name  | Description                                                     | Optional |
| --------- | --------------------------------------------------------------- | -------- |
| pluginDir | Path to a directory containing the plugin `.so` files to verify | No       |
| manifest  | A .yaml file containing information required to load the plugin | No       |
| debug     | Set debug log level                                             | Yes      |

The `manifest` file is needed to instruct the script on how to load the plugins. It intentionally has a very similar 
format as the configuration defined on the `AuthConfig` resource:

```yaml
name: MyPlugin
pluginFileName: Plugin.so
exportedSymbolName: MyPlugin
config: {} # plugin-specific config
```

Here is the sample output of a successful run of the script:

```text
{"level":"info","ts":"2019-08-21T17:02:22.803Z","logger":"verify-plugins.header_value_plugin","caller":"pkg/impl.go:39","msg":"Parsed RequiredHeaderAuthService config","requiredHeader":"my-auth-header","allowedHeaderValues":["foo","bar","baz"]}
{"level":"info","ts":"2019-08-21T17:02:22.803Z","logger":"verify-plugins","caller":"plugins/loader.go:85","msg":"Successfully loaded plugin. Adding it to the plugin chain.","pluginName":"RequiredHeader"}
{"level":"info","ts":"2019-08-21T17:02:22.803Z","logger":"verify-plugins","caller":"scripts/verify_plugins.go:62","msg":"Successfully verified that plugins can be loaded by Gloo!"}
```

{{% notice note %}}
The script is compiled to run on `linux` with `amd64` architectures. We will explain how it is supposed to be used in the next section.
{{% /notice %}}
 
#### Dockerfile
We mentioned that the plugin must be compiled with the same `GOPATH` as Gloo Edge. We also cannot easily cross-compile with 
`go build` because we need to run with CGO enabled. The best way to get around these constraints is to compile inside a 
container.

The example repository contains a [Dockerfile](https://github.com/solo-io/ext-auth-plugin-examples/blob/master/Dockerfile),
which we include here since all of it is relevant. You can use it as a starting point for your builds. We recommend that 
you use [multi-stage builds](https://docs.docker.com/develop/develop-images/multistage-build/) to keep the size of 
your final image to a minimum. See the comments for an explanation of each build layer:

{{< highlight yaml >}}
# This stage is parametrized to replicate the same environment Gloo Edge Enterprise was built in.
# All ARGs need to be set via the docker `--build-arg` flags.
ARG GO_BUILD_IMAGE
FROM $GO_BUILD_IMAGE AS build-env

# This must contain the value of the `gcflag` build flag that Gloo Edge was built with
ARG GC_FLAGS
# This must contain the path to the plugin verification script
ARG VERIFY_SCRIPT

# Fail if VERIFY_SCRIPT not set
# We don't have the same check GC_FLAGS as empty values are allowed
RUN if [[ ! $VERIFY_SCRIPT ]]; then echo "Required VERIFY_SCRIPT build argument not set" && exit 1; fi

# Install packaes needed for compilation
RUN apk add --no-cache gcc musl-dev

# Copy the repository to the image and set it as the working directory. The GOPATH her is `/go`.
#
# You have to update the path here to the one corresponding to your repository. This is usually in the form:
# /go/src/github.com/YOUR_ORGANIZATION/PLUGIN_REPO_NAME
ADD . /go/src/github.com/solo-io/ext-auth-plugin-examples/
WORKDIR /go/src/github.com/solo-io/ext-auth-plugin-examples

# De-vendor all the dependencies and move them to the GOPATH, so they will be loaded from there.
# We need this so that the import paths for any library shared between the plugins and Gloo Edge are the same.
#
# For example, if we were to vendor the ext-auth-plugin dependency, the ext-auth-server would load the plugin interface
# as `GLOOE_REPO/vendor/github.com/solo-io/ext-auth-plugins/api.ExtAuthPlugin`, while the plugin
# would instead implement `THIS_REPO/vendor/github.com/solo-io/ext-auth-plugins/api.ExtAuthPlugin`. These would be seen 
# by the go runtime as two different types, causing Gloo Edge to fail.
# Also, some packages cause problems if loaded more than once. For example, loading `golang.org/x/net/trace` twice
# causes a panic (see here: https://github.com/golang/go/issues/24137). By flattening the dependencies this way we
# prevent these sorts of problems.
RUN cp -a vendor/. /go/src/ && rm -rf vendor

# Build plugins with CGO enabled, passing the GC_FLAGS flags
RUN CGO_ENABLED=1 GOARCH=amd64 GOOS=linux go build -buildmode=plugin -gcflags="$GC_FLAGS" -o plugins/RequiredHeader.so plugins/required_header/plugin.go

# Run the script to verify that the plugin(s) can be loaded by Gloo Edge
RUN chmod +x $VERIFY_SCRIPT
RUN $VERIFY_SCRIPT -pluginDir plugins -manifest plugins/plugin_manifest.yaml

# This stage builds the final image containing just the plugin .so files. It can really be any linux/amd64 image.
FROM alpine:3.17.3

# Copy compiled plugin file from previous stage
RUN mkdir /compiled-auth-plugins
COPY --from=build-env /go/src/github.com/solo-io/ext-auth-plugin-examples/plugins/RequiredHeader.so /compiled-auth-plugins/

# This is the command that will be executed when the container is run. 
# It has to copy the compiled plugin file(s) to a directory.
CMD cp /compiled-auth-plugins/* /auth-plugins/
{{< /highlight >}}

The two `GO_BUILD_IMAGE`, `VERIFY_SCRIPT`, and `GC_FLAGS` arguments have to be passed to docker via the `--build-arg` flag(s):

```bash
docker build -t your_repo:your_tag \
    --build-arg GO_BUILD_IMAGE=<value-from-build_env-file> \
	--build-arg GC_FLAGS=<value-from-build_env-file> \
    --build-arg VERIFY_SCRIPT=<value-from-build_env-file> \
	.
```

You have to get the values for these arguments from the Google Cloud bucket. 

{{% notice note %}}
Our example repository contains a [Makefile](https://github.com/solo-io/ext-auth-plugin-examples/blob/master/Makefile) 
with targets that automate these steps and can be easily modified to fit your needs. Be sure to check it out!
{{% /notice %}}

#### Wrapping it up
If you followed the guide to this point, you should have an image that is guaranteed to be compatible with Gloo Edge.

The next step is to see how to set up Gloo Edge to use your plugins.

## Configuring Gloo Edge to load your plugins
The Gloo Edge `extauth` server loads plugins from a directory in the file system it has access to. It is possible to 
accomplish this in different ways, but the preferred one (and the reason why we packaged the plugins as docker images 
with a `copy` entry point) is by running the plugin container(s) as `initContainer`(s) and mounting a volume shared with 
the `extauth` deployment.

In the [**Plugin Auth** guide]({{% versioned_link_path fromRoot="/guides/security/auth/extauth/plugin_auth" %}}) we saw how to do 
this [using glooctl]({{% versioned_link_path fromRoot="/guides/security/auth/plugin_auth#installation" %}}). 
Here we will see how to accomplish the same result by editing the raw Gloo Edge Enterprise YAML manifest.

Let's start with a basic version of the `extauth` deployment. Note that we are omitting many attributes for brevity.

```yaml
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
      - image: quay.io/solo-io/extauth-ee:0.20.6
        imagePullPolicy: IfNotPresent
        name: extauth
```

In order for the `extauth` server to load the plugin files from your images, we apply the following changes:

{{< highlight yaml "hl_lines=22-34" >}}
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
      - image: quay.io/solo-io/extauth-ee:0.20.6
        imagePullPolicy: IfNotPresent
        name: extauth
        volumeMounts:
        - mountPath: /auth-plugins
          name: auth-plugins
      initContainers:
      - image: quay.io/solo-io/ext-auth-plugins:0.20.6
        imagePullPolicy: IfNotPresent
        name: plugin-my-plugin
        volumeMounts:
        - mountPath: /auth-plugins
          name: auth-plugins
      volumes:
      - emptyDir: {}
        name: auth-plugins
{{< /highlight >}}

A [emptyDir](https://kubernetes.io/docs/concepts/storage/volumes/#emptydir) volume has been added and it is mounted on 
both the `extauth` container and the plugin `initContainers` at the `/auth-plugins` path. This is the same path our 
plugin container is configured to copy the plugin files to when it is run.

{{% notice note %}}
Currently, Gloo Edge expects to find the plugin files in the `/auth-plugins` directory. We plan to make this location configurable soon.
{{% /notice %}}

### Configuring Virtual Services to use your plugins
The [*Plugin Auth* guide]({{% versioned_link_path fromRoot="/guides/security/auth/extauth/plugin_auth#secure-the-virtual-service" %}}) 
contains a thorough explanation of how to update Virtual Service to use your plugins to authenticate requests. 

## Multi-step AuthConfigs
Earlier in this guide we called your attention to the fact that the `configs` attribute in the `AuthConfig` CRD is an 
array. When the `configs` array contains more then one element, we refer to it as a **multi-step** `AuthConfig`:

{{< highlight yaml "hl_lines=7-15" >}}
apiVersion: enterprise.gloo.solo.io/v1
kind: AuthConfig
metadata:
  name: plugin-auth
  namespace: gloo-system
spec:
  configs:
  - pluginAuth:
      name: my-plugin
      pluginFileName: MyPlugin.so
      exportedSymbolName: Plugin
      config:
        some_key: value-1
        some_struct:
          another_key: value-2
{{< /highlight >}}

Elements in the `configs` array (from now on **steps**) will be executed in the order they are defined. The first step 
to deny the request will cause execution to be interrupted and Gloo Edge to return the response generated by the step that 
denied it. No steps after the failing one will be executed.

{{% notice note %}}
Some of the external auth schemes that Gloo Edge provides out-of-the-box take advantage of this feature. 
To see an example of this, see the appendix to the 
[**Open Policy Agent** Authorization guide]({{% versioned_link_path fromRoot="/guides/security/auth/extauth/opa" %}}), where we 
use an OPA step to enforce a policy on a 
[**JSON Web Token**]({{% versioned_link_path fromRoot="/guides/security/auth/jwt" %}}) produced by a 
previous [**OIDC**]({{% versioned_link_path fromRoot="/guides/security/auth/extauth/oauth" %}}) step.
{{% /notice %}}

### Header propagation
Each step in a multi-step `AuthConfig` can append, add or override headers from the request it received before 
forwarding it to the upstream or the next step. It is important to understand how header modifications are handled when 
more than one step is defined.

Let's look at the [response object](https://github.com/solo-io/ext-auth-plugins/blob/master/api/interface.go#L16) 
returned by our `AuthService` instance:

{{< highlight go "hl_lines=12" >}}
package api

import (
	envoyauthv2 "github.com/envoyproxy/go-control-plane/envoy/service/auth/v2"
)

// Response returned by authorization services to the Gloo Edge ext-auth server
type AuthorizationResponse struct {
	// Additional user information
	UserInfo UserInfo
	// The result of the authorization process that will be sent back to Envoy
	CheckResponse envoyauthv2.CheckResponse
}
{{< /highlight >}}

You can see that it wraps the Envoy [CheckResponse](https://www.envoyproxy.io/docs/envoy/latest/api-v2/service/auth/v2/external_auth.proto#service-auth-v2-checkresponse) 
type. Gloo Edge conforms to the Envoy API semantics when merging headers in plugin chains. If you go to the linked Envoy docs 
page and inspect the [OkHttpResponse](https://www.envoyproxy.io/docs/envoy/latest/api-v2/service/auth/v2/external_auth.proto#service-auth-v2-okhttpresponse) 
object, you will see that it consists of just an array of [HeadersValueOption objects](https://www.envoyproxy.io/docs/envoy/latest/api-v3/config/core/v3/base.proto#envoy-api-msg-core-headervalueoption).
A `HeadersValueOption` associates a header value with an `Append` flag. The flag determines whether the header will 
overridden or appended to the one in the request.

To give a concrete example, suppose Gloo Edge receives a request that with the following header:

```text
Headers:
  - one: foo
```

and that the request matches a Virtual Service that references an `AuthConfig` configured with the following sequence of
auth configurations:

```     
config_1:
  HeadersValueOptions:
  - Header:
      one: bar
    Append: false
config_2:
  HeadersValueOptions:
  - Header:
      one: baz
    Append: true
  - Header:
      two: asd
```

If the request is authenticated by both configurations, then the headers on the final request that will be sent 
to the upstream will be:

```text
Headers:
  - one: bar, baz
  - two: asd
```

### Sharing state between steps
If you are writing your own external auth plugins and chaining them together in a multi-step `AuthConfig`, a common 
requirement is to be able to share state between a step and the ones that are following. You could achieve this via 
header manipulation (see the preceding section), but this more of a workaround than a maintainable approach.

Luckily, the `AuthService` API provides you with a more effective way to share state between plugins.
Let's look at the [request object](https://github.com/solo-io/ext-auth-plugins/blob/master/api/interface.go#L23) 
accepted by our `AuthService` instances:

{{< highlight go "hl_lines=4-5" >}}
type AuthorizationRequest struct {
	// The request that needs to be authorized
	CheckRequest *envoyauthv2.CheckRequest
	// Use this attribute to share state between `AuthService`s
	State        map[string]interface{}
}
{{< /highlight >}}

Each request contains a generic `State` map. Whichever values are stored in that field when an `AuthService` step 
(in this case your plugin) returns a response, those values will be available in the `AuthorizationRequest` that will be 
passed to the next step.

## Conclusion
If you got to this point, we hope that you have a good understanding of how the Gloo Edge Ext Auth plugin framework works and 
that you are ready to start hacking away! If you have any questions or ideas about how to improve this guide, please 
contact us on our [**Slack**](https://slack.solo.io) or open an issue in the [Gloo Edge repository](https://github.com/solo-io/gloo), 
adding the "**Area: Docs**" label.
