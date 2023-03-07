---
title: Rate limiting setup
weight: 5
description: Set up and verify your rate limiting environment
---

Set up and verify rate limiting in Gloo Edge. For more information about how rate limiting works, see [Rate limiting]({{< versioned_link_path fromRoot="/guides/security/rate_limiting/" >}}).

## Before you begin

1. [Create your environment]({{< versioned_link_path fromRoot="/installation/platform_configuration/" >}}), such as a Kubernetes cluster in a cloud provider.
2. [Install Gloo Edge Open Source]({{< versioned_link_path fromRoot="/installation/gateway/" >}}) (Envoy API rate limiting only) or [Gloo Edge Enterprise]({{< versioned_link_path fromRoot="/installation/enterprise/" >}}) (all supported rate limiting) in your environment.
3. Install a test app such as Pet Store from the [Hello World tutorial]({{< versioned_link_path fromRoot="/guides/traffic_management/hello_world/" >}}).
4. Optional: [Configure your rate limiting server]({{< versioned_link_path fromRoot="/guides/security/rate_limiting/enterprise/" >}}) to change the defaults, such as to update the query behavior or to use a different backing database.

## Step 1: Decide which rate limiting API to use {#api}

Depending on the type of Gloo Edge that you installed, you have multiple options for rate limiting.

| Rate limiting API | Supported Product | Description |
| ----------------- | ----------------- | ----------- |
| [Envoy API]({{< versioned_link_path fromRoot="/guides/security/rate_limiting/envoy/" >}}) | Gloo Edge Open Source or Enterprise | To use the Envoy rate limiting API, you configure descriptors (required key and optional value pairs to attach to a request) and actions (counters to use for the request). The order of descriptors matter, and requests are only limited if the ordered descriptors match a rule exactly. For example, say that you want to have two different rate limiting behaviors for requests:<ul><li>Limit requests with an `x-type` header.</li><li>Limit requests with both `x-type` and `x-number: 5` headers.</li></ul>To set this rate limiting up, you must have two corresponding actions:<ul><li>Get only the value of `x-type` header.</li><li>Get both the values of `x-type` and `x-number` headers.</li></ul> This approach lets you set up many rate limiting use cases. As such, the Envoy API is well-suited for any of your rate limiting needs, but you might choose a different API style if you have more complex (set-style) or simpler (Gloo Edge) use cases.|
| [Set-Style API]({{< versioned_link_path fromRoot="/guides/security/rate_limiting/set/" >}}) | Gloo Edge Enterprise | Like the Envoy API, the set-style API is based on descriptors (`setDescriptors`) and actions (`setActions`). Unlike the Envoy API, the set-style descriptors are unordered and can be used in combination with other descriptors. For example, you might set up a wildcard matching rule to rate limit requests with:<ul><li>An `x-type: a` header.</li><li>An `x-number: 1` header.</li><li>Any `x-color` header (`x-color: *`).</li></ul>At scale, this approach is more flexible than the Envoy API approach. You can also use Envoy and set-style APIs together. |
| [Gloo Edge API]({{< versioned_link_path fromRoot="/guides/security/rate_limiting/simple/" >}}) | Gloo Edge Enterprise | For simple rate limiting per route or host, you can use the Gloo Edge rate limiting API. In this approach, you do not have to set up complicated descriptors and actions. Instead, you simply specify the requests per unit and time unit for each route or host directly within the virtual service resource. You can also have different rate limiting behavior for authorized versus anonymous requests.|

## Step 2: Implement rate limiting {#implement}

Depending on the rate limiting API that you chose to use, you have several options on how to implement rate limiting in your Gloo Edge routing, host, and settings resources.

### Envoy or Set-Style API {#implement-envoy-set}

Choose between two implementation approaches:
* **Enterprise-only**: [In the `RateLimitConfig` resource](#implement-rlc). You can configure descriptors and actions together in a `RateLimitConfig` resource. This approach is more flexible at scale, and less likely to cause errors in configuring your resources.
* **Open Source or Enterprise**: [In separate Gloo Edge resources](#implement-separate). You configure descriptors in the Gloo Edge `Settings` for the entire cluster. Then, you configure the actions directly in each `VirtualService` that you want to rate limit. This approach is not as flexible at scale. Also, because you have to edit the `Settings` and `VirtualServices` resources more extensively, you might be likelier to make a configuration error or encounter merge conflicts if multiple people edit a configuration at once. If you use Gloo Edge Open Source, you must take this implementation approach.

### In the `RateLimitConfig` resource {#implement-rlc}

1. Define the descriptors and actions in the `RateLimitConfig` resource. For more information, see [RateLimitConfigs (Enterprise)]({{< versioned_link_path fromRoot="/guides/security/rate_limiting/crds/" >}}).
   ```yaml
   kubectl apply -f - <<EOF
   apiVersion: ratelimit.solo.io/v1alpha1
   kind: RateLimitConfig
   metadata:
     name: my-rate-limit-policy
     namespace: gloo-system
   spec:
     raw:
       descriptors:
       - key: generic_key
         value: counter
         rateLimit:
           requestsPerUnit: 10
           unit: MINUTE
       rateLimits:
       - actions:
         - genericKey:
             descriptorValue: counter
   EOF
   ```
2. Refer to the `RateLimitConfig` in each `VirtualService` that you want to rate limit. The following example updates the `default` virtual service that you created when you installed the sample Pet Store app.
   1. Get the virtual service configuration file.
      ```sh
      kubectl get vs default -n gloo-system -o yaml > vs.yaml
      ```
   2. Refer the rate limit config that you previously created for all the routes in the virtual host or on a per-route basis.
      {{< tabs >}} 
{{% tab name="All routes in the virtual host" %}}
```yaml
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
      - exact: /all-pets
      options:
        prefixRewrite: /api/pets
      routeAction:
        single:
          upstream:
            name: default-petstore-8080
            namespace: gloo-system
    options:
      rateLimitConfigs:
        refs:
        - name: my-rate-limit-policy
          namespace: gloo-system
```
{{% /tab %}} 
{{% tab name="Per route" %}}
```yaml
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
      - exact: /all-pets
      options:
        prefixRewrite: /api/pets
        rateLimitConfigs:
          refs:
          - name: my-rate-limit-policy
            namespace: gloo-system
      routeAction:
        single:
          upstream:
            name: default-petstore-8080
            namespace: gloo-system
```
{{% /tab %}} 
      {{< /tabs >}}
3. To fill in the descriptor and action values, continue to [Configure rate limiting behavior](#configure).

### In separate resources {#implement-separate}

1. Define the rate limit descriptors in the `Settings` resource.
2. Define the actions in each `VirtualService` resource.
3. To fill in the descriptor and action values, continue to [Configure rate limiting behavior](#configure).


### Gloo Edge API {#implement-gloo-edge}



## Step 3: Configure rate limiting behavior {#configure}

## Step 4: Verify rate limting with a sample app {#verify}

## Next steps

Now that you know the basic steps to set up rate limiting, you might explore the following options.

* Use rate limiting [metrics]({{< versioned_link_path fromRoot="/guides/security/rate_limiting/metrics/" >}}) or [access logs]({{< versioned_link_path fromRoot="/guides/security/rate_limiting/access_logs/" >}}) to improve rate limiting.
* Try out more examples of the [Envoy]({{< versioned_link_path fromRoot="/guides/security/rate_limiting/envoy/" >}}), [Set-Style]({{< versioned_link_path fromRoot="/guides/security/rate_limiting/set/" >}}), and [Gloo Edge]({{< versioned_link_path fromRoot="/guides/security/rate_limiting/simple/" >}}) rate limiting API.
* Explore other ways to [secure network traffic with Gloo Edge]({{< versioned_link_path fromRoot="/guides/security/rate_limiting/" >}}).