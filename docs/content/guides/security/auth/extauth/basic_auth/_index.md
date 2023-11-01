---
title: Basic Auth
weight: 10
description: Authenticating using a dictionary of usernames and passwords on a virtual service. 
---

{{% notice note %}}
{{< readfile file="static/content/enterprise_only_feature_disclaimer" markdown="true">}}
{{% /notice %}}

In certain cases - such as during testing or when releasing a new API to a small number of known users - it may be 
convenient to secure a Virtual Service using [**Basic Authentication**](https://en.wikipedia.org/wiki/Basic_access_authentication). 
With this simple authentication mechanism the encoded user credentials are sent along with the request in a standard header.

To secure your Virtual Services using Basic Authentication, you first need to provide Gloo Edge with a set of known users and 
their passwords. You can then use this information to decide who is allowed to access which routes.
If a request matches a route on which Basic Authentication is configured, Gloo Edge will verify the credentials in the 
standard `Authorization` header before sending the request to its destination. If the user associated with the credentials 
is not explicitly allowed to access that route, Gloo Edge will return a 401 response to the downstream client.

Be sure to check the external auth [configuration overview]({{< versioned_link_path fromRoot="/guides/security/auth/extauth/#auth-configuration-overview" >}}) 
for detailed information about how authentication is configured on Virtual Services.

## Setup
{{< readfile file="/static/content/setup_notes" markdown="true">}}

Let's start by creating a [Static Upstream]({{< versioned_link_path fromRoot="/guides/traffic_management/destination_types/static_upstream/" >}}) 
that routes to a website; we will send requests to it during this tutorial.

{{< tabs >}}
{{< tab name="kubectl" codelang="yaml">}}
{{< readfile file="/static/content/upstream.yaml">}}
{{< /tab >}}
{{< tab name="glooctl" codelang="shell" >}}
glooctl create upstream static --static-hosts jsonplaceholder.typicode.com:80 --name json-upstream
{{< /tab >}}
{{< /tabs >}}

## Creating a Virtual Service
Now let's configure Gloo Edge to route requests to the upstream we just created. To do that, we define a simple Virtual 
Service to match all requests that:

- contain a `Host` header with value `foo` and
- have a path that starts with `/` (this will match all requests).

Apply the following virtual service:
{{< readfile file="guides/security/auth/extauth/basic_auth/test-no-auth-vs.yaml" markdown="true">}}

Let's send a request that matches the above route to the Gloo Edge gateway and make sure it works:

```shell
curl -H "Host: foo" $(glooctl proxy url)/posts/1
```

The above command should produce the following output:

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
Gloo Edge allows any request on routes that do not specify authentication configuration. Let's change this behavior. 
We will update the Virtual Service so that only requests by the user `user` with password `password` are allowed.
Gloo Edge expects password to be hashed and [salted](https://en.wikipedia.org/wiki/Salt_(cryptography)) using the
[APR1](https://httpd.apache.org/docs/2.4/misc/password_encryptions.html) format. Passwords in this format follow this pattern:

> $apr1$**SALT**$**HASHED_PASSWORD**

To generate such a password you can use the `htpasswd` utility:

```shell
htpasswd -nbm user password
```

Running the above command returns a string like `user:$apr1$TYiryv0/$8BvzLUO9IfGPGGsPnAgSu1`, where:

- `TYiryv0/` is the salt and
- `8BvzLUO9IfGPGGsPnAgSu1` is the hashed password.

Now that we have a password in the required format, let's go ahead and create an `AuthConfig` CRD with our 
Basic Authentication configuration:

{{< highlight shell "hl_lines=13-14" >}}
kubectl apply -f - <<EOF
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
            salt: "TYiryv0/"
            hashedPassword: "8BvzLUO9IfGPGGsPnAgSu1"
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
          name: basic-auth
          namespace: gloo-system
EOF
{{< /highlight >}}

In the above example we have added the configuration to the Virtual Host. Each route belonging to a Virtual Host will 
inherit its `AuthConfig`, unless it [overwrites or disables]({{< versioned_link_path fromRoot="/guides/security/auth/extauth/#inheritance-rules" >}}) it.

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
< www-authenticate: Basic realm=""
< date: Mon, 07 Oct 2019 13:36:58 GMT
< server: envoy
< content-length: 0
{{< /highlight >}}

### Testing authenticated requests
For a request to be allowed, it must now include the user credentials inside the expected header, which has the 
following format:

```
Authorization: basic <base64_encoded_credentials>
```

To encode the credentials, just run:

```shell
echo -n "user:password" | base64
```

This outputs `dXNlcjpwYXNzd29yZA==`. Let's include the header with this value in our request:

```shell
curl -H "Authorization: basic dXNlcjpwYXNzd29yZA==" -H "Host: foo" $(glooctl proxy url)/posts/1
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

### Logging

If Gloo Edge is running on kubernetes, the extauth server logs can be viewed with:
```
kubectl logs -n gloo-system deploy/extauth -f
```
If the auth config has been received successfully, you should see the log line:
```
"logger":"extauth","caller":"runner/run.go:179","msg":"got new config"
```

### Extended configuration
{{% notice warning %}}
The auth configuration format that is shown on this page was introduced with [Gloo Edge Enterprise release 1.16.0]({{< versioned_link_path fromRoot="/reference/changelog/enterprise" >}}).
To find the configuration format for an earlier version, see [Configuration format history]({{< versioned_link_path fromRoot="/guides/security/auth/extauth/configuration_format_history/" >}}). 
{{% /notice %}}

An extended configuration is available that allows use of the SHA1 hashing algorithm instead of APR.

This configuration splits definition the users from the definition of the encryption algorithm. The configuration with the same functionality as the example would look like:

{{< highlight shell "hl_lines=9-15" >}}
apiVersion: enterprise.gloo.solo.io/v1
kind: AuthConfig
metadata:
  name: basic-auth
  namespace: gloo-system
spec:
  configs:
  - basicAuth:
      encryption:
        apr: {}
      userList:
        users:
          user:
            salt: "TYiryv0/"
            hashedPassword: "8BvzLUO9IfGPGGsPnAgSu1"
{{< /highlight >}}

To switch to SHA1 encryption, the following config would support the same user list (with updated salted and hashed passwords):

{{< highlight shell "hl_lines=10" >}}
apiVersion: enterprise.gloo.solo.io/v1
kind: AuthConfig
metadata:
  name: basic-auth
  namespace: gloo-system
spec:
  configs:
  - basicAuth:
      encryption:
        sha1: {}
      userList:
        users:
          user:
            salt: "TYiryv0/"
            hashedPassword: "010eb058a59f4ac5ba05639b0263cf91b4345fd6"
{{< /highlight >}}

The same `curls` should work with this config as the hashing algorithm only affects the hashed password stored on the server side.
```shell
curl -H "Authorization: basic dXNlcjpwYXNzd29yZA==" -H "Host: foo" $(glooctl proxy url)/posts/1
```

The hashed password is case-insensitive as the alphabetic characters represent hexadecimal digits.

When using the extended configuration, the `proxy-authorization` header is also supported.
```shell
curl -H "Proxy- Authorization: basic dXNlcjpwYXNzd29yZA==" -H "Host: foo" $(glooctl proxy url)/posts/1
```

## Summary

In this tutorial, we installed Gloo Edge Enterprise and created an unauthenticated Virtual Service that routes requests to a 
static upstream. We then created a Basic Authentication `AuthConfig` object and used it to secure our Virtual Service. 
We first showed how unauthenticated requests fail with a `401 Unauthorized` response, and then showed how to send 
authenticated requests successfully to the upstream. 

Cleanup the resources by running:

```
kubectl delete ac -n gloo-system basic-auth
kubectl delete vs -n gloo-system auth-tutorial
kubectl delete upstream -n gloo-system json-upstream
```
