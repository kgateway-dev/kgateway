---
title: OPA Authorization
weight: 50
description: Illustrating how to combine OpenID Connect with Open Policy Agent to achieve fine grained policy with Gloo.
---

{{% notice note %}}
The OPA feature was introduced with **Gloo Enterprise**, release 0.18.21. If you are using an earlier version, this tutorial will not work.
{{% /notice %}}

The [Open Policy Agent](https://www.openpolicyagent.org/) (OPA) is an open source, general-purpose policy engine that 
can be used to define and enforce versatile policies in a uniform way across your organization. 
Compared to an RBAC authorization system, OPA allows you to create more fine-grained policies. For more information, see 
[the official docs](https://www.openpolicyagent.org/docs/latest/comparison-to-other-systems/).

Be sure to check the external auth [configuration overview]({{< ref "gloo_routing/virtual_services/security#configuration-overview" >}}) 
for detailed information about how authentication is configured on Virtual Services.

## Setup
{{< readfile file="/static/content/setup_notes" markdown="true">}}

Let's deploy a sample application that we will route requests to during this guide:

```shell script
kubectl apply -f https://raw.githubusercontent.com/solo-io/gloo/master/example/petstore/petstore.yaml
```

### Creating a Virtual Service

Now we can create a Virtual Service that routes any requests with the `/echo` prefix to the `http-echo` service.

{{< highlight shell "hl_lines=17-21" >}}
kubectl apply -f - << EOF
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
    - matcher:
        prefix: /
      routeAction:
        single:
          kube:
            ref:
              name: petstore
              namespace: default
            port: 8080
EOF
{{< /highlight >}}


To verify that the Virtual Service works, let's send a request to `/api/pets`:

```shell
curl $GATEWAY_URL/api/pets
```

You should see the following output:

```json
[{"id":1,"name":"Dog","status":"available"},{"id":2,"name":"Cat","status":"pending"}]
```

## Secure the Virtual Service
{{% notice warning %}}
{{% extauth_version_info_note %}}
{{% /notice %}}

As we just saw, we were able to reach the upstream without having to provide any credentials. This is because by default 
Gloo allows any request on routes that do not specify authentication configuration. Let's change this behavior. 
We will update the Virtual Service so that only requests that comply with a given OPA policy are allowed.


### Define an OPA policy 
Open Policy Agent policies are written in [Rego](https://www.openpolicyagent.org/docs/latest/how-do-i-write-policies/). 
The _Rego_ language is inspired from _Datalog_, which in turn is a subset of _Prolog_. _Rego_ is more suited to work 
with modern JSON documents. Let's create a Policy to control which actions are allowed on our service:

```shell
cat <<EOF > policy.rego
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
```

This policy:

- denies everything by default,
- allows requests if:
  - the path starts with `/api/pets` AND the http method is `GET` **OR**
  - the path is exactly `/api/pets/2` AND the http method is either `GET` or `DELETE`


### Create an OPA AuthConfig CRD
Gloo expects OPA policies to be stored in a Kubernetes ConfigMap, so let't go ahead and create a ConfigMap with the 
contents of the above policy file:

```
kubectl -n gloo-system create configmap allow-get-users --from-file=policy.rego
```

First we can to create an `AuthConfig` CRD with our OPA authorization configuration:

{{< highlight shell "hl_lines=9-13" >}}
kubectl apply -f - <<EOF
apiVersion: enterprise.gloo.solo.io/v1
kind: AuthConfig
metadata:
  name: opa
  namespace: gloo-system
spec:
  configs:
  - opa_auth:
      modules:
      - name: allow-get-users
        namespace: gloo-system
      query: "data.test.allow == true"
EOF
{{< /highlight >}}

The above `AuthConfig` references the ConfigMap  (`modules`) we created earlier and adds a query that allows access only 
if the `allow` variable is `true`. 

### Update the Virtual Service
Once the `AuthConfig` has been created, we can use it to secure our Virtual Service:

{{< highlight shell "hl_lines=21-25" >}}
kubectl apply -f - <<EOF
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
    - matcher:
        prefix: /
      routeAction:
        single:
          kube:
            ref:
              name: petstore
              namespace: default
            port: 8080
    virtualHostPlugins:
      extauth:
        config_ref:
          name: opa
          namespace: gloo-system
EOF
{{< /highlight >}}

In the above example we have added the configuration to the Virtual Host. Each route belonging to a Virtual Host will 
inherit its `AuthConfig`, unless it [overwrites or disables]({{< ref "gloo_routing/virtual_services/security#inheritance-rules" >}}) it.

### Testing our configuration
Paths that don't start with `/api/pets` are not authorized (should return 403):
```
curl -s -w "%{http_code}\n" $GATEWAY_URL/api/

403
```

Not allowed to delete `pets/1` (should return 403):
```
curl -s -w "%{http_code}\n" $GATEWAY_URL/api/pets/1 -X DELETE

403
```

Allowed to delete `pets/2` (should return 204):
```
curl -s -w "%{http_code}\n" $GATEWAY_URL/api/pets/2 -X DELETE

204
```

## Open Policy Agent and Open ID Connect
We can use OPA to verify policies on the JWT coming from Gloo's OpenID Connect authentication.

### Install Dex
Let's first configure an OpenID Connect provider on your cluster. Dex Identity provider is an OpenID Connect that's easy to install for our purposes:

```
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

- The user's email is "admin@example.com"
- **OR**
 - The user's email is "user@exmaple.com" 
 - **AND**
 - The path being accessed does **NOT** start with /owners

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
glooctl create  secret oauth --client-secret secretvalue oauth
glooctl create vs --name default --namespace gloo-system --oidc-auth-app-url http://localhost:8080/ --oidc-auth-callback-path /callback --oidc-auth-client-id gloo --oidc-auth-client-secret-name oauth --oidc-auth-client-secret-namespace gloo-system --oidc-auth-issuer-url http://dex.gloo-system.svc.cluster.local:32000/ --oidc-scope email --enable-oidc-auth --enable-opa-auth --opa-query 'data.test.allow == true' --opa-module-ref gloo-system.allow-jwt
glooctl add route --name default --path-prefix / --dest-name default-petclinic-80 --dest-namespace gloo-system
{{< /tab >}}
{{< tab name="kubectl" codelang="yaml">}}
apiVersion: v1
kind: Secret
type: Opaque
metadata:
  annotations:
    resource_kind: '*v1.Secret'
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
    domains:
    - '*'
    routes:
    - matcher:
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
            configs:
            - oauth:
                app_url: http://localhost:8080/
                callback_path: /callback
                client_id: gloo
                client_secret_ref:
                  name: oauth
                  namespace: gloo-system
                issuer_url: http://dex.gloo-system.svc.cluster.local:32000/
                scopes:
                - email
            - opa_auth:
                modules:
                - name: allow-jwt
                  namespace: gloo-system
                query: data.test.allow == true
{{< /tab >}}
{{< /tabs >}} 


### Local Cluster Adjustments
As we are testing in a local cluster, add `127.0.0.1 dex.gloo-system.svc.cluster.local` to your `/etc/hosts` file:
```
echo "127.0.0.1 dex.gloo-system.svc.cluster.local" | sudo tee -a /etc/hosts
```

The OIDC flow redirects the browser to a login page hosted by dex. This line in the hosts file will allow this flow to work, with 
Dex hosted inside our cluster (using `kubectl port-forward`).

Port forward to Gloo and Dex:
```
kubectl -n gloo-system port-forward svc/dex 32000:32000 &
kubectl -n gloo-system port-forward svc/gateway-proxy-v2 8080:80 &
```

### Verify!

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

```
helm delete --purge dex
kubectl delete -n gloo-system secret  dex-grpc-ca  dex-grpc-client-tls  dex-grpc-server-tls  dex-web-server-ca  dex-web-server-tls
kubectl delete -n gloo-system vs default
kubectl delete -n gloo-system configmap allow-get-users allow-jwt
```
