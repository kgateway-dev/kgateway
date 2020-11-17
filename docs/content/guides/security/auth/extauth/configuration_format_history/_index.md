---
title: Configuration format history
weight: 100
description: Overview of the external auth configuration formats supported by each Gloo Edge Enterprise version.
---

#### Gloo Edge Enterprise versions >=0.20.1

**Gloo Edge Enterprise**, release [**0.20.1**]({{< versioned_link_path fromRoot="/reference/changelog/enterprise" >}}), simplified the
external auth configuration format. You can now specify the `extauth` configuration directly on the `Options`/`Plugins`
(Gloo Edge 1.0+ vs Gloo Edge 0.x respectively) attribute of the relevant resource:

```yaml
options: # Pre Gloo Edge 1.0, this was virtualHostPlugins, routePlugins, or weightedDestinationPlugins
  extauth:
    configRef:
      name: basic-auth
      namespace: gloo-system
```

Compare this to the old format (not supported in Gloo Edge 1.0+):

```yaml
virtualHostPlugins:
  extensions:
    configs:
      extauth:
        configRef:
          name: basic-auth
          namespace: gloo-system
```

For more information on the latest configuration format see the [main page]({{< versioned_link_path fromRoot="/guides/security/auth/extauth/#auth-configuration-overview" >}}) 
of the authentication section of the docs.

#### Gloo Edge Enterprise versions >=0.19.0

{{% notice info %}}
As of now, this configuration format is still supported by **Gloo Edge Enterprise**.
{{% /notice %}}

**Gloo Edge Enterprise**, release [**0.19.0**]({{< versioned_link_path fromRoot="/reference/changelog/enterprise" >}}), introduced the possibility to 
configure authentication on **Routes** and **WeightedDestinations**. As part of this change, authentication configurations 
have been promoted to top-level resources, i.e. they are stored in a dedicated `AuthConfig` resource. 
**The new features require this new configuration format**.

Here is an example `AuthConfig` resource:

```yaml
apiVersion: enterprise.gloo.solo.io/v1
kind: AuthConfig
metadata:
  name: basic-auth
  namespace: gloo-system
spec:
  configs:
  - basicAuth:
      realm: "test"
      apr:
        users:
          user:
            salt: "TYiryv0/"
            hashedPassword: "8BvzLUO9IfGPGGsPnAgSu1"
```

The format of the configuration for the different external auth implementations **has not changed** from previous versions, 
i.e. the `spec.configs` attribute has the same format as the `extensions.configs.extauth.configs` attribute that we used 
to define directly on virtual services.

Once you have defined your `AuthConfigs` you can reference them in your virtual services like this:

{{< highlight yaml "hl_lines=10-16 25-30" >}}
apiVersion: gateway.solo.io/v1
kind: VirtualService
metadata:
  name: my-vs
  namespace: gloo-system
spec:
  virtualHost:
    domains:
    - 'example.com'
    virtualHostPlugins:
      extensions:
        configs:
          extauth:
            configRef:
              name: basic-auth # Default auth config for this virtual host and all its child resources
              namespace: gloo-system
    routes:
    - matcher:
        prefix: /super-secret
      routeAction:
        single:
          upstream:
            name: some-secret-upstream-1234
            namespace: gloo-system
      routePlugins:
        extensions:
          configs:
            extauth:
              name: admin-auth # More specific config overwrites the parent default
              namespace: gloo-system
    - matcher:
        prefix: /public
      routeAction:
        single:
          upstream:
            name: some-public-upstream-1234
            namespace: gloo-system
      routePlugins:
        extensions:
          configs:
            extauth:
              disable: true # Disable auth for this route
    - matcher:
        prefix: /
      routeAction:
        single:
          upstream:
            name: some-upstream-1234
            namespace: gloo-system
{{< /highlight >}}

#### Gloo Edge Enterprise versions >=0.18.21

{{% notice info %}}
As of now, this configuration format is still supported by **Gloo Edge Enterprise**.
{{% /notice %}}

**Gloo Edge Enterprise**, release [**0.18.21**]({{< versioned_link_path fromRoot="/reference/changelog/enterprise" >}}), introduced a change in the 
authentication configuration format. It turned the `extauth` attribute from being an object into an array. This allows us 
to define multiple configuration steps that are executed in the order in which they are specified. If any one of these 
steps fails, the request will be denied without executing any subsequent steps. Authentication can still be configured 
only on virtual hosts, with the possibility for child routes to opt out.

Here is an example of this configuration format:

{{< highlight yaml "hl_lines=21-30" >}}
apiVersion: gateway.solo.io/v1
kind: VirtualService
metadata:
  name: test-auth
  namespace: gloo-system
spec:
  virtualHost:
    domains:
      - 'foo'
    routes:
      - matcher:
          prefix: /authenticated
        routeAction:
          single:
            upstream:
              name: my-upstream
              namespace: gloo-system
    virtualHostPlugins:
      extensions:
        configs:
          extauth:
            configs:
            - basicAuth:
                realm: "test"
                apr:
                  users:
                    user:
                      salt: "TYiryv0/"
                      hashedPassword: "8BvzLUO9IfGPGGsPnAgSu1"
{{< /highlight >}}

#### Gloo Edge Enterprise versions <0.18.21

{{% notice info %}}
As of now, this configuration format is still supported by **Gloo Edge Enterprise**.
{{% /notice %}}

This is the original configuration format that was first introduced in the early days of **Gloo Edge Enterprise** 
(it was originally released with version **v0.0.10**). This configuration format supports authentication only on **Virtual Hosts**. 
The configuration has to be specified directly on the Virtual Service CRD:

{{< highlight yaml "hl_lines=18-30" >}}
apiVersion: gateway.solo.io/v1
kind: VirtualService
metadata:
  name: test-auth
  namespace: gloo-system
spec:
  virtualHost:
    domains:
      - 'foo'
    routes:
      - matcher:
          prefix: /authenticated
        routeAction:
          single:
            upstream:
              name: my-upstream
              namespace: gloo-system
    virtualHostPlugins:
      extensions:
        configs:
          extauth:
            basicAuth:
              realm: "test"
              apr:
                users:
                  user:
                    salt: "TYiryv0/"
                    hashedPassword: "8BvzLUO9IfGPGGsPnAgSu1"
{{< /highlight >}}

On a **Route** level, it is only possible to opt out of auth configurations specified on parent Virtual Hosts:

{{< highlight yaml "hl_lines=25-29" >}}
apiVersion: gateway.solo.io/v1
kind: VirtualService
metadata:
  name: test-auth
  namespace: gloo-system
spec:
  virtualHost:
    domains:
      - 'foo'
    routes:
      - matcher:
          prefix: /authenticated
        routeAction:
          single:
            upstream:
              name: my-upstream
              namespace: gloo-system
      - matcher:
          prefix: /skip-auth
        routeAction:
          single:
            upstream:
              name: my-insecure-upstream
              namespace: gloo-system
        routePlugins:
          extensions:
            configs:
              extauth:
                disable: true
    virtualHostPlugins:
      extensions:
        configs:
          extauth:
            basicAuth:
              realm: "test"
              apr:
                users:
                  user:
                    salt: "TYiryv0/"
                    hashedPassword: "8BvzLUO9IfGPGGsPnAgSu1"
{{< /highlight >}}
