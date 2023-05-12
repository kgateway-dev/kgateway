---
title: API Keys
weight: 40
description: How to setup ApiKey authentication. 
---

{{% notice note %}}
The API keys authentication feature was introduced with **Gloo Edge Enterprise**, release 0.18.5. If you are using an earlier version, this tutorial will not work.
{{% /notice %}}

{{% notice note %}}
The API key secret format shown in this guide was introduced with **Gloo Edge Enterprise**, release v1.5.0-beta8. 
If you are using an earlier version, please refer to the [previous version](https://docs.solo.io/gloo-edge/1.3.0/security/auth/apikey_auth/) 
of this guide.
{{% /notice %}}

Sometimes when you need to protect a service, the set of users that will need to access it is known in advance and does 
not change frequently. For example, these users might be other services or specific persons or teams in your organization. 

You might also want to retain direct control over how credentials are generated and when they expire. If one of these 
conditions applies to your use case, you should consider securing your service using 
[API keys](https://en.wikipedia.org/wiki/Application_programming_interface_key). API keys are secure, long-lived UUIDs 
that clients must provide when sending requests to a service that is protected using this method. 

{{% notice warning %}}
It is important to note that **your services are only as secure as your API keys**; securing API keys and proper API key 
rotation is up to the user, thus the security of the routes is up to the user.
{{% /notice %}}

To secure your services using API keys, you first need to provide Gloo Edge with your secret API keys in the form of `Secrets`. After your API key secrets are in place, you can configure authentication on your Virtual Services by referencing the secrets in one of two ways:

1. You can specify a **label selector** that matches one or more labelled API key secrets (this is the preferred option), or
1. You can **explicitly reference** a set of secrets by their identifier (namespace and name).

When Gloo Edge matches a request to a route secured with API keys, it looks for a valid API key in the request headers. 
The name of the header that is expected to contain the API key is configurable. If the header is not present, 
or if the API key it contains does not match one of the API keys in the secrets referenced on the Virtual Service, 
Gloo Edge will deny the request and return a 401 response to the downstream client.

Internally, Gloo Edge will generate a mapping of API keys to _user identities_ for all API keys present in the system. The _user identity_ for a given API key is the name of the `Secret` which contains the API key. The _user identity_ will be added to the request as a header, `x-user-id` by default, which can be utilized in subsequent filters. You can see the [default order of the filters in the Gloo Edge source](https://github.com/solo-io/gloo/blob/main/projects/gloo/pkg/plugins/plugin_interface.go#L187-L198). In this specific case, the extauth plugin (which handles the api key flow) is part of the `AuthNStage` stage, so filters after this stage will have access to the `user identity` header. For example, this functionality is used in [Gloo Edge's rate limiting API to provide different rate limits for anonymous vs. authorized users]({{< versioned_link_path fromRoot="/guides/security/rate_limiting/simple" >}}). For security reasons, this header will be sanitized from the response before it leaves the proxy.

Be sure to check the external auth [configuration overview]({{< versioned_link_path fromRoot="/guides/security/auth/extauth/#auth-configuration-overview" >}}) for detailed information about how authentication is configured on Virtual Services.

## Setup
{{< readfile file="/static/content/setup_notes" markdown="true">}}

Let's create a [Static Upstream]({{< versioned_link_path fromRoot="/guides/traffic_management/destination_types/static_upstream/" >}}) named `json-upstream` that routes to a static website; we will send requests to it during this tutorial.

{{< tabs >}}
{{< tab name="kubectl" codelang="yaml">}}
{{< readfile file="/static/content/upstream.yaml">}}
{{< /tab >}}
{{< tab name="glooctl" codelang="shell" >}}
glooctl create upstream static --static-hosts jsonplaceholder.typicode.com:80 --name json-upstream
{{< /tab >}}
{{< /tabs >}}

## Creating a Virtual Service
Now let's configure Gloo Edge to route requests to the upstream we just created. To do that, we define a simple Virtual Service to match all requests that:

- Contain a `Host` header with value `foo` and
- Have a path that starts with `/` (this will match all requests).

{{< tabs >}}
{{< tab name="kubectl" codelang="yaml">}}
{{< readfile file="guides/security/auth/extauth/apikey_auth/test-no-auth-vs.yaml">}}
{{< /tab >}}
{{< /tabs >}} 

Let's send a request that matches the above route to the Gloo Edge gateway and make sure it works:

```shell
curl -H "Host: foo" $(glooctl proxy url)/posts/1
```

The above command should return:

```json
{
  "userId": 1,
  "id": 1,
  "title": "sunt aut facere repellat provident occaecati excepturi optio reprehenderit",
  "body": "quia et suscipit\nsuscipit recusandae consequuntur expedita et cum\nreprehenderit molestiae ut ut quas totam\nnostrum rerum est autem sunt rem eveniet architecto"
}
```

## Securing the Virtual Service
{{% notice warning %}}
{{% extauth_version_info_note %}}
{{% /notice %}}

As we just saw, we were able to reach the upstream without having to provide any credentials. 
This is because by default Gloo Edge allows any request on routes that do not specify authentication configuration. 
Let's change this behavior. We will update the Virtual Service so that only requests containing 
a valid API key are allowed.

We start by creating an API key secret using `glooctl`:

```shell
glooctl create secret apikey infra-apikey \
    --apikey N2YwMDIxZTEtNGUzNS1jNzgzLTRkYjAtYjE2YzRkZGVmNjcy \
    --apikey-labels team=infrastructure
```

The above command creates a secret that:

- is named `infra-apikey`,
- is placed in the `gloo-system` namespace (this  is the default if no namespace is provided via the `--namespace` flag),
- is of kind `apikey` (this is just a Kubernetes secret which contains the API key in the `data` entry with the key expected by Gloo Edge),
- is marked with a label named `team` with value `infrastructure`, and 
- contains an API key with value `N2YwMDIxZTEtNGUzNS1jNzgzLTRkYjAtYjE2YzRkZGVmNjcy`.

Instead of providing a value for the API key we could also have asked `glooctl` to generate one for us using the 
`--apikey-generate` flag.
 
Let's verify that we have indeed created a valid secret.

```shell
kubectl get secret infra-apikey -n gloo-system -oyaml
```

You secret should look similar to this:

```yaml
apiVersion: v1
kind: Secret
type: extauth.solo.io/apikey
metadata:
  labels:
    team: infrastructure
  name: infra-apikey
  namespace: gloo-system
data:
  api-key: TjJZd01ESXhaVEV0TkdVek5TMWpOemd6TFRSa1lqQXRZakUyWXpSa1pHVm1OamN5
```

The value of `data.api-key` is base64-encoded. Let's decode it to verify that it is indeed our API key:

```shell
echo TjJZd01ESXhaVEV0TkdVek5TMWpOemd6TFRSa1lqQXRZakUyWXpSa1pHVm1OamN5 | base64 -D
```

The command should return `N2YwMDIxZTEtNGUzNS1jNzgzLTRkYjAtYjE2YzRkZGVmNjcy`, which is indeed our API key.

Now that we have a valid API key secret, let's go ahead and create an `AuthConfig` Custom Resource (CR) with our API key authentication configuration:

{{< highlight shell "hl_lines=9-14" >}}
kubectl apply -f - <<EOF
apiVersion: enterprise.gloo.solo.io/v1
kind: AuthConfig
metadata:
  name: apikey-auth
  namespace: gloo-system
spec:
  configs:
  - apiKeyAuth:
      # This is the name of the header that is expected to contain the API key.
      # This field is optional and defaults to `api-key` if not present.
      headerName: api-key
      labelSelector:
        team: infrastructure
EOF
{{< /highlight >}}

Once the `AuthConfig` has been created, we can use it to secure our Virtual Service:

{{< highlight shell "hl_lines=21-25" >}}
kubectl apply -f - <<EOF
apiVersion: gateway.solo.io/v1
kind: VirtualService
metadata:
  name: auth-tutorial
  namespace: gloo-system
spec:
  virtualHost:
    domains:
      - 'foo'
    routes:
      - matchers:
        - prefix: /
        routeAction:
          single:
            upstream:
              name: json-upstream
              namespace: gloo-system
        options:
          autoHostRewrite: true
    options:
      extauth:
        configRef:
          name: apikey-auth
          namespace: gloo-system
EOF
{{< /highlight >}}

In the above example we have added the configuration to the Virtual Host. Each route belonging to a Virtual Host will inherit its `AuthConfig`, unless it [overwrites or disables]({{< versioned_link_path fromRoot="/guides/security/auth/extauth/#inheritance-rules" >}}) it.

### Testing denied requests
Let's try and resend the same request we sent earlier:

```shell
curl -v -H "Host: foo" $(glooctl proxy url)/posts/1
```

You will see that the response now contains a **401 Unauthorized** code, indicating that Gloo Edge denied the request.

{{< highlight shell "hl_lines=6" >}}
> GET /posts/1 HTTP/1.1
> Host: foo
> User-Agent: curl/7.54.0
> Accept: */*
>
< HTTP/1.1 401 Unauthorized
< www-authenticate: API key is missing or invalid
< date: Mon, 07 Oct 2019 19:28:14 GMT
< server: envoy
< content-length: 0
{{< /highlight >}}

### Testing authenticated requests
For a request to be allowed, it must include a header named `api-key` with the value set to the 
API key we previously stored in our secret. Now let's add the authorization headers:

```shell
curl -H "api-key: N2YwMDIxZTEtNGUzNS1jNzgzLTRkYjAtYjE2YzRkZGVmNjcy" -H "Host: foo" $(glooctl proxy url)/posts/1
```

We are now able to reach the Upstream again!

```json
{
  "userId": 1,
  "id": 1,
  "title": "sunt aut facere repellat provident occaecati excepturi optio reprehenderit",
  "body": "quia et suscipit\nsuscipit recusandae consequuntur expedita et cum\nreprehenderit molestiae ut ut quas totam\nnostrum rerum est autem sunt rem eveniet architecto"
}
```

## Summary

In this tutorial, we installed Gloo Edge Enterprise and created an unauthenticated Virtual Service that routes requests to a 
static upstream. We then created an API key `AuthConfig` object and used it to secure our Virtual Service. 
We first showed how unauthenticated requests fail with a `401 Unauthorized` response, and then showed how to send 
authenticated requests successfully to the upstream. 

Cleanup the resources by running:

```
kubectl delete ac -n gloo-system apikey-auth
kubectl delete vs -n gloo-system auth-tutorial
kubectl delete upstream -n gloo-system json-upstream
kubectl delete secret -n gloo-system infra-apikey
```
