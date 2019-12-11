---
title: API Keys
weight: 40
description: How to setup ApiKey authentication. 
---

{{% notice note %}}
The API keys authentication feature was introduced with **Gloo Enterprise**, release 0.18.5. If you are using an earlier version, this tutorial will not work.
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

To secure your services using API keys, you first need to provide Gloo with your secret API keys in the form of `Secrets`. 
After your API key secrets are in place, you can configure authentication on your Virtual Services by referencing the 
secrets in one of two ways:

1. you can specify a **label selector** that matches one or more labelled API key secrets (this is the preferred option), or
2. you can **explicitly reference** a set of secrets by their identifier (namespace and name).

When Gloo matches a request to a route secured with API keys, it looks for a valid API key in the `api-key` header. If 
the header is not present, or if the API key it contains does not match one of the API keys in the secrets referenced on 
the Virtual Service, Gloo will deny the request and return a 401 response to the downstream client.

Be sure to check the external auth [configuration overview]({{< versioned_link_path fromRoot="/security/auth#auth-configuration-overview" >}}) 
for detailed information about how authentication is configured on Virtual Services.

## Setup
{{< readfile file="/static/content/setup_notes" markdown="true">}}

Let's create a [Static Upstream]({{< versioned_link_path fromRoot="/gloo_routing/virtual_services/routes/route_destinations/single_upstreams/static_upstream" >}}) 
named `json-upstream` that routes to a static website; we will send requests to it during this tutorial.

{{< tabs >}}
{{< tab name="kubectl" codelang="yaml">}}
{{< readfile file="/static/content/upstream.yaml">}}
{{< /tab >}}
{{< tab name="glooctl" codelang="shell" >}}
glooctl create upstream static --static-hosts jsonplaceholder.typicode.com:80 --name json-upstream
{{< /tab >}}
{{< /tabs >}}

## Creating a Virtual Service
Now let's configure Gloo to route requests to the upstream we just created. To do that, we define a simple Virtual 
Service to match all requests that:

- contain a `Host` header with value `foo` and
- have a path that starts with `/` (this will match all requests).

{{< tabs >}}
{{< tab name="kubectl" codelang="yaml">}}
{{< readfile file="security/auth/apikey_auth/test-no-auth-vs.yaml">}}
{{< /tab >}}
{{< /tabs >}} 

Let's send a request that matches the above route to the Gloo Gateway and make sure it works:

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

As we just saw, we were able to reach the upstream without having to provide any credentials. This is because by default 
Gloo allows any request on routes that do not specify authentication configuration. Let's change this behavior. 
We will update the Virtual Service so that only requests containing a valid API key in their `api-key` header are allowed.

We start by creating an API key secret using `glooctl`:

```shell
glooctl create secret apikey infra-apikey --apikey N2YwMDIxZTEtNGUzNS1jNzgzLTRkYjAtYjE2YzRkZGVmNjcy --apikey-labels team=infrastructure
```

The above command creates a secret that:

- is named `infra-apikey`,
- is placed in the `gloo-system` namespace (this  is the default if no namespace is provided via the `--namespace` flag),
- is of kind `apikey` (this is just a Kubernetes secret with additional metadata that can be interpreted by Gloo),
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
metadata:
  annotations:
    resource_kind: '*v1.Secret'
  labels:
    team: infrastructure
  name: infra-apikey
  namespace: gloo-system
type: Opaque
data:
  apiKey: YXBpS2V5OiBOMll3TURJeFpURXROR1V6TlMxak56Z3pMVFJrWWpBdFlqRTJZelJrWkdWbU5qY3kKbGFiZWxzOgotIHRlYW09aW5mcmFzdHJ1Y3R1cmUK
```

Now let's take the value of `data.apiKey`, which is base64-encoded, and decode it:

```shell
echo YXBpS2V5OiBOMll3TURJeFpURXROR1V6TlMxak56Z3pMVFJrWWpBdFlqRTJZelJrWkdWbU5qY3kKbGFiZWxzOgotIHRlYW09aW5mcmFzdHJ1Y3R1cmUK | base64 -D
```

You should get the following 
{{< protobuf display="API key secret configuration" name="enterprise.gloo.solo.io.ApiKeySecret" >}}:

```yaml
config:
  api_key: N2YwMDIxZTEtNGUzNS1jNzgzLTRkYjAtYjE2YzRkZGVmNjcy
  labels:
  - team=infrastructure
```

Our API key is indeed `N2YwMDIxZTEtNGUzNS1jNzgzLTRkYjAtYjE2YzRkZGVmNjcy`! 

Now that we have a valid API key secret, let's go ahead and create an `AuthConfig` CRD with our API key 
authentication configuration:

{{< highlight shell "hl_lines=9-11" >}}
kubectl apply -f - <<EOF
apiVersion: enterprise.gloo.solo.io/v1
kind: AuthConfig
metadata:
  name: apikey-auth
  namespace: gloo-system
spec:
  configs:
  - api_key_auth:
      label_selector:
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
        config_ref:
          name: apikey-auth
          namespace: gloo-system
EOF
{{< /highlight >}}

In the above example we have added the configuration to the Virtual Host. Each route belonging to a Virtual Host will 
inherit its `AuthConfig`, unless it [overwrites or disables]({{< versioned_link_path fromRoot="/security/auth#inheritance-rules" >}}) it.

### Testing denied requests
Let's try and resend the same request we sent earlier:

```shell
curl -v -H "Host: foo" $(glooctl proxy url)/posts/1
```

You will see that the response now contains a **401 Unauthorized** code, indicating that Gloo denied the request.

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
For a request to be allowed, it must include a header named `api-key` with the value set to the API key we previously 
stored in our secret. Now let's add the authorization headers:

```shell
curl -H "api-key: N2YwMDIxZTEtNGUzNS1jNzgzLTRkYjAtYjE2YzRkZGVmNjcy" -H "Host: foo" $(glooctl proxy url)/posts/1
```

We are now able to reach the upstream again!

```json
{
  "userId": 1,
  "id": 1,
  "title": "sunt aut facere repellat provident occaecati excepturi optio reprehenderit",
  "body": "quia et suscipit\nsuscipit recusandae consequuntur expedita et cum\nreprehenderit molestiae ut ut quas totam\nnostrum rerum est autem sunt rem eveniet architecto"
}
```

## Summary

In this tutorial, we installed Gloo Enterprise and created an unauthenticated Virtual Service that routes requests to a 
static upstream. We then created an API key `AuthConfig` object and used it to secure our Virtual Service. 
We first showed how unauthenticated requests fail with a `401 Unauthorized` response, and then showed how to send 
authenticated requests successfully to the upstream. 

Cleanup the resources by running:

```
kubectl delete ac -n gloo-system apikey-auth
kubectl delete vs -n gloo-system auth-tutorial
kubectl delete upstream -n gloo-system json-upstream
```
