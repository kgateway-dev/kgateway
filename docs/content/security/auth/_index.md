---
title: External auth
weight: 10
description: Authenticate and authorize requests to your services using Gloo's external auth service.
---

## Why Authenticate in API Gateway Environments

API Gateways act as a control point for the outside world to access the various application services (monoliths, microservices, serverless functions) running in your environment. In microservices or hybrid application architecture, any number of these workloads need to accept incoming requests from external end users (clients). Incoming requests are treated as anonymous or authenticated and depending on the service. You may want to establish and validate who the client is, the service they are requesting, and define any access or traffic control policies.

## Authentication in Gloo

{{% notice note %}}
This section refers specifically to the **Gloo Enterprise** external auth server. If you are using the open source version of Gloo, please refer to the [Custom Auth section]({{< versioned_link_path fromRoot="/security/auth/custom_auth" >}}) of the security docs.
{{% /notice %}}

Gloo Enterprise provides a variety of authentication options to meet the needs of your environment. They range from supporting basic use cases to complex and fine grained secure access control. Architecturally, Gloo uses a dedicated auth server to verify the user credentials and determine their permissions. Gloo provides an auth server that can support several authN/Z implementations and also allows you to provide your auth server to implement custom logic.

{{% notice info %}}
If you are seeing authentication errors for `OPTIONS` requests, and your application is doing CORS, please refer to the [Understanding CORS]({{< versioned_link_path fromRoot="/security/cors" >}}) docs.
{{% /notice %}}

### Switching Between Ext Auth Deployment Modes

By default, Gloo's built-in Auth Server deploys as a Kubernetes pod. To authenticate a request, Gloo needs to communicate with the authentication service over the network. In case you deem this overhead not to be acceptable for your use case, you can deploy the server in **sidecar mode**.

In this configuration, the Ext Auth server runs as an additional container inside the `gateway-proxy` pod(s) that run Gloo's Envoy instance(s), and communication with Envoy occurs via Unix Domain Sockets instead of TCP. This cuts out the overhead associated with the TCP protocol and can provide substantial performance benefits (40%+ in some benchmarks).

You can activate this mode by [installing Gloo with Helm]({{< versioned_link_path fromRoot="/installation/enterprise#installing-on-kubernetes-with-helm" >}}) and providing the following value overrides:

| option                                         | type | description                                                                                                                       |
| ---------------------------------------------- | ---- | --------------------------------------------------------------------------------------------------------------------------------- |
| global.extensions.extAuth.envoySidecar         | bool | Deploy ext-auth as a sidecar to Envoy instances. Communication occurs over Unix Domain Sockets instead of TCP. Default is `false` |
| global.extensions.extAuth.standaloneDeployment | bool | Deploy ext-auth as a standalone deployment. Communication occurs over TCP. Default is `true`                                      |

Note that you can provide any combination of values for these two overrides, including setting both to `true` if you'd like to more easily test the latency improvement gained by using the sidecar mode. Regardless of the deployment mode used, the values set in `global.extensions.extAuth` applies to both instances of Ext Auth.

You can set which auth server Envoy queries by updating your settings to use the proper Ext Auth upstream. On install, Gloo manually creates an upstream for each instance of Ext Auth that you have deployed; standalone deployments get an upstream named `extauth`, and sidecar deployments get an upstream named `extauth-sidecar`. If you are deploying both standalone and sidecar or just standalone, the default upstream name in your settings is `extauth`. If you are deploying just the sidecar, the default upstream name in your settings is `extauth-sidecar`.

Here is a look at a snippet of the `default` settings after a fresh install of Gloo, using the default Helm configuration; note the Ext Auth server ref specifically.

```shell
kubectl --namespace gloo-system get settings default --output yaml
```

{{< highlight yaml "hl_lines=13-16" >}}
apiVersion: gloo.solo.io/v1
kind: Settings
metadata:
  ...
  labels:
    app: gloo
    gloo: settings
  name: default
  namespace: gloo-system
  ...
spec:
  discoveryNamespace: gloo-system
  extauth:
    extauthzServerRef:
      name: extauth
      namespace: gloo-system
  gateway:
    validation:
      alwaysAccept: true
      proxyValidationServerAddr: gloo:9988
  gloo:
    xdsBindAddr: 0.0.0.0:9977
  kubernetesArtifactSource: {}
  kubernetesConfigSource: {}
  kubernetesSecretSource: {}
  ratelimitServer:
    ratelimitServerRef:
      name: rate-limit
      namespace: gloo-system
  refreshRate: 60s
  ...
{{< /highlight >}}

### Auth Configuration Overview

{{% notice info %}}
{{% extauth_version_info_note %}}
{{% /notice %}}

Authentication configuration is defined in {{< protobuf display="AuthConfig" name="enterprise.gloo.solo.io.AuthConfig" >}} resources. `AuthConfig`s are top-level resources, which means that if you are running in Kubernetes, they will be stored in a dedicated CRD. Here is an example of a simple `AuthConfig` CRD:

```yaml
apiVersion: enterprise.gloo.solo.io/v1
kind: AuthConfig
metadata:
  name: basic-auth
  namespace: gloo-system
spec:
  configs:
  - basicAuth:
      apr:
        users:
          user:
            hashedPassword: 14o4fMw/Pm2L34SvyyA2r.
            salt: 0adzfifo
      realm: gloo
```

The resource `spec` consists of an array of `configs` that will be executed in sequence when a request matches the `AuthConfig` (more on how requests are matched to `AuthConfigs` shortly). If any one of these "authentication steps" fails, the request will be denied. In most cases an `AuthConfig` will contain a single `config`.

#### Configuration Format

Once an `AuthConfig` is created, it can be used to authenticate your `Virtual Services` (or on the lower-level `Proxy` resources). You can define authentication configuration your Virtual Services at three different levels:

- on **VirtualHosts**,
- on **Routes**, and
- on **WeightedDestinations**.

The configuration format is the same in all three cases. It must be specified under the relevant `options` attribute (on Virtual Hosts, Routes, or Weighted Destinations) and can take one of two forms. The first is used to enable authentication and requires you to reference an existing `AuthConfig`. An example configuration of this kind follows:

```yaml
options:
  extauth:
    config_ref:
      # references the example AuthConfig we defined earlier
      name: basic-auth
      namespace: gloo-system
```

In the case of a route or weighted destination, the top attribute would be named `options` as well.

The second form is used to disable authentication explicitly:

```yaml
options:
  extauth:
    config_ref:
      disable: true
```

#### Inheritance Rules

By default, an `AuthConfig` defined on a `Virtual Service` attribute is inherited by all the child attribute:

- `AuthConfig`s defined on `VirtualHosts` are inherited by `Route`s and `WeightedDestination`s.
- `AuthConfig`s defined on `Route`s are inherited by `WeightedDestination`s.

There are two exceptions to this rule:

- if the child attribute defines an `AuthConfig`, or
- if the child explicitly disables authentication via the `disable: true` configuration.

### Implementations

We have seen how `AuthConfigs` can be used to define granular authentication configurations for `Virtual Services`. For a detailed overview of the authN/authZ models implemented by Gloo, check out the other guides:

{{% children description="true" %}}
