---
title: OPA Authorization
weight: 50
description: Illustrating how to combine OpenID Connect with Open Policy Agent to achieve fine grained policy with Gloo.
---

## Motivation

Open Policy Agent (OPA for short) can be used to express versatile organization policies.
Starting in gloo-e version 0.18.21 you can use OPA policies to make authorization decisions
on incoming requests.
This allows you having uniform policy language all across your organization.
This also allows you to create more fine grained policies compared to RBAC authorization system. For more information, see [here](https://www.openpolicyagent.org/docs/latest/comparison-to-other-systems/).

## Prerequisites

- A Kubernetes cluster. [minikube](https://github.com/kubernetes/minikube) is a good way to get started
- `glooctl` - To install and interact with Gloo (optional).

## Install Gloo and Test Service

That's easy!

```shell
glooctl install gateway enterprise --license-key=$GLOO_KEY
kubectl --namespace default apply -f https://raw.githubusercontent.com/solo-io/gloo/master/example/petstore/petstore.yaml
```

See more information and options of installing Gloo [here](/installation/enterprise).

### Verify Install

Make sure all is deployed correctly:

```shell
curl $(glooctl proxy url)/api/pets
```

should respond with

```json
[
  { "id": 1, "name": "Dog", "status": "available" },
  { "id": 2, "name": "Cat", "status": "pending" }
]
```

## Configuring an Open Policy Agent Policy

Open Policy Agent policies are written in [Rego](https://www.openpolicyagent.org/docs/latest/how-do-i-write-policies/). The Rego language is inspired from Datalog, which inturn is a subset of Prolog. Rego is more suited to work with modern JSON documents.

### Create the Policy

Let's create a Policy to control what actions are allowed on our service, and apply it to Kubernetes as a ConfigMap:

```shell
cat <<EOF > /tmp/policy.rego
package test

default allow = false
allow {
    startswith(input.http_request.path, "/api/pets")
    input.http_request.method == "GET"
}
allow {
    input.http_request.path == "/api/pets/2"
    any({input.http_request.method == "GET",
        input.http_request.method == "DELETE"
    })
}
EOF
kubectl --namespace=gloo-system create configmap allow-get-users --from-file=/tmp/policy.rego
```

Let's break this down:

- This policy denies everything by default
- It is allowed if:
  - The path starts with "/api/pets" AND the http method is "GET"
  - **OR**
  - The path is exactly "/api/pets/2" AND the http method is either "GET" or "DELETE"

In the next setup, we will attach this policy to a Gloo VirtualService to enforce it.

### Create a VirtualService with the OPA Authorization

To enforce the policy, we will create a Gloo VirtualService with OPA Authorization enabled. We will refer to the policy created above, and add a query that allows access
only if the `allow` variable is `true`:

{{< tabs >}}
{{< tab name="glooctl" codelang="shell">}}
glooctl create vs --name default --enable-opa-auth --opa-query 'data.test.allow == true' --opa-module-ref gloo-system.allow-get-users
glooctl add route --name default --path-prefix / --dest-name default-petstore-8080 --dest-namespace gloo-system
{{< /tab >}}
{{< tab name="kubectl" codelang="yaml">}}
kind: VirtualService
metadata:
name: default
namespace: gloo-system
spec:
displayName: default
virtualHost:
domains: - '\*'
routes: - matcher:
prefix: /
routeAction:
single:
upstream:
name: default-petstore-8080
namespace: gloo-system
virtualHostPlugins:
extensions:
configs:
extauth:
configs: - opa_auth:
modules: - name: allow-get-users
namespace: gloo-system
query: "data.test.allow == true"
{{< /tab >}}
{{< /tabs >}}

That's all that is needed as far as configuration. Let's verify that all is working as expected.

## Verify Configuration

```shell
URL=$(glooctl proxy url)
```

Paths that don't start with /api/pets are not authorized (should return 403):

```shell
$ curl -s -w "%{http_code}\n" $URL/api/
403
```

Not allowed to delete pets/1 (should return 403):

```shell
$ curl -s -w "%{http_code}\n" $URL/api/pets/1 -X DELETE
403
```

Allowed to delete pets/2 (should return 204):

```shell
$ curl -s -w "%{http_code}\n" $URL/api/pets/2 -X DELETE
204
```

## Open Policy Agent and Open ID Connect

We can use OPA to verify policies on the JWT coming from Gloo's OpenID Connect authentication.

### Install Dex

Let's first configure an OpenID Connect provider on your cluster. Dex Identity provider is an OpenID Connect that's easy to install for our purposes:

```shell
cat > /tmp/dex-values.yaml <<EOF
config:
  issuer: http://dex.gloo-system.svc.cluster.local:32000

  staticClients:
  - id: gloo
    redirectURIs:
    - 'http://localhost:8080/callback'
    name: 'GlooApp'
    secret: secretvalue

  staticPasswords:
  - email: "admin@example.com"
    # bcrypt hash of the string "password"
    hash: "\$2a\$10\$2b2cU8CPhOTaGrs1HRQuAueS7JTT5ZHsHSzYiFPm1leZck7Mc8T4W"
    username: "admin"
    userID: "08a8684b-db88-4b73-90a9-3cd1661f5466"
  - email: "user@example.com"
    # bcrypt hash of the string "password"
    hash: "\$2a\$10\$2b2cU8CPhOTaGrs1HRQuAueS7JTT5ZHsHSzYiFPm1leZck7Mc8T4W"
    username: "user"
    userID: "123456789-db88-4b73-90a9-3cd1661f5466"
EOF

helm install --name dex --namespace gloo-system stable/dex -f /tmp/dex-values.yaml
```

This configuration deploys dex with two static users.

### Deploy Demo App

Deploy the pet clinic demo app

```shell
kubectl --namespace default apply -f https://raw.githubusercontent.com/solo-io/gloo/v0.8.4/example/petclinic/petclinic.yaml
```

### Create a Policy

```shell
cat <<EOF > /tmp/allow-jwt.rego
package test

default allow = false

allow {
    [header, payload, signature] = io.jwt.decode(input.state.jwt)
    payload["email"] = "admin@example.com"
}
allow {
    [header, payload, signature] = io.jwt.decode(input.state.jwt)
    payload["email"] = "user@example.com"
    not startswith(input.http_request.path, "/owners")
}
EOF

kubectl --namespace=gloo-system create configmap allow-jwt --from-file=/tmp/allow-jwt.rego
```

This policy allows the request if:

- The user's email is "admin@example.com" **OR**
- The user's email is "user@exmaple.com" **AND** The path being accessed does **NOT** start with /owners

### Configure Gloo

Cleanup the VirtualService from the previous section:

{{< tabs >}}
{{< tab name="glooctl" codelang="shell">}}
glooctl delete virtualservice default
{{< /tab >}}
{{< tab name="kubectl" codelang="shell">}}
kubectl -n gloo-system delete virtualservice default
{{< /tab >}}
{{< /tabs >}}

Create a new virtual service with the new policy and demo app.

{{< tabs >}}
{{< tab name="glooctl" codelang="shell">}}
glooctl create secret oauth --client-secret secretvalue oauth
glooctl create vs --name default --namespace gloo-system --oidc-auth-app-url http://localhost:8080/ --oidc-auth-callback-path /callback --oidc-auth-client-id gloo --oidc-auth-client-secret-name oauth --oidc-auth-client-secret-namespace gloo-system --oidc-auth-issuer-url http://dex.gloo-system.svc.cluster.local:32000/ --oidc-scope email --enable-oidc-auth --enable-opa-auth --opa-query 'data.test.allow == true' --opa-module-ref gloo-system.allow-jwt
glooctl add route --name default --path-prefix / --dest-name default-petclinic-80 --dest-namespace gloo-system
{{< /tab >}}
{{< tab name="kubectl" codelang="yaml">}}
apiVersion: v1
kind: Secret
type: Opaque
metadata:
annotations:
resource_kind: '\*v1.Secret'
name: oauth
namespace: gloo-system
data:
extension: Y29uZmlnOgogIGNsaWVudF9zZWNyZXQ6IHNlY3JldHZhbHVlCg==

---

apiVersion: gateway.solo.io/v1
kind: VirtualService
metadata:
name: default
namespace: gloo-system
spec:
displayName: default
virtualHost:
domains: - '\*'
routes: - matcher:
prefix: /
routeAction:
single:
upstream:
name: default-petclinic-80
namespace: gloo-system
virtualHostPlugins:
extensions:
configs:
extauth:
configs: - oauth:
app_url: http://localhost:8080/
callback_path: /callback
client_id: gloo
client_secret_ref:
name: oauth
namespace: gloo-system
issuer_url: http://dex.gloo-system.svc.cluster.local:32000/
scopes: - email - opa_auth:
modules: - name: allow-jwt
namespace: gloo-system
query: data.test.allow == true
{{< /tab >}}
{{< /tabs >}}

### Local Cluster Adjustments

As we are testing in a local cluster, add `127.0.0.1 dex.gloo-system.svc.cluster.local` to your `/etc/hosts` file:

```shell
echo "127.0.0.1 dex.gloo-system.svc.cluster.local" | sudo tee -a /etc/hosts
```

The OIDC flow redirects the browser to a login page hosted by dex. This line in the hosts file will allow this flow to work, with
Dex hosted inside our cluster (using `kubectl port-forward`).

Port forward to Gloo and Dex:

```shell
kubectl -n gloo-system port-forward svc/dex 32000:32000 &
kubectl -n gloo-system port-forward svc/gateway-proxy-v2 8080:80 &
```

### Verify

{{% notice note %}}
As the demo app doesn't have a sign-out button, use a private browser window (also known as incognito mode) to access the demo app. This will make it easy to change the user we logged in with.
If you would like to change the logged in user, just close and re-open the private browser window
{{% /notice %}}

Go to "localhost:8080". You can login with "admin@example.com" or "user@example.com" with the password "password".

You will notice that the admin user has access to all pages, and that the regular user can't access the "Find Owners" page.

**Success!**

## Summary

I this tutorial we explored Gloo's Open Policy Agent integration to enable policies on incoming requests. We also saw that we can combine OpenID Connect and Open Policy Agent together to create policies on JSON Web Tokens.

## Cleanup

```shell
helm delete --purge dex
kubectl delete -n gloo-system secret  dex-grpc-ca  dex-grpc-client-tls  dex-grpc-server-tls  dex-web-server-ca  dex-web-server-tls
kubectl delete -n gloo-system vs default
kubectl delete -n gloo-system configmap allow-get-users allow-jwt
```
