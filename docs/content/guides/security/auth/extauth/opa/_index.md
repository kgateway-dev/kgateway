---
title: OPA Authorization
weight: 50
description: Illustrating how to combine OpenID Connect with Open Policy Agent to achieve fine grained policy with Gloo Edge.
---

The [Open Policy Agent](https://www.openpolicyagent.org/) (OPA) is an open source, general-purpose policy engine that can be used to define and enforce versatile policies in a uniform way across your organization. Compared to an RBAC authorization system, OPA allows you to create more fine-grained policies. For more information, see [the official docs](https://www.openpolicyagent.org/docs/latest/comparison-to-other-systems/).

Be sure to check the external auth [configuration overview]({{% versioned_link_path fromRoot="/guides/security/auth/extauth/#auth-configuration-overview" %}}) for detailed information about how authentication is configured on Virtual Services.

{{% notice note %}}
As of **Gloo Edge Enterprise** release 1.5.0-beta1, you can see Gloo Edge's version of OPA in the extauth service logs.
{{% /notice %}}

## Table of Contents
- [OPA policy overview](#opa-policy-overview)
    - [OPA input structure](#opa-input-structure)
- [Validate requests attributes with Open Policy Agent](#validate-requests-attributes-with-open-policy-agent)
    - [Deploy sample application](#deploy-a-sample-application)
    - [Creating a Virtual Service](#creating-a-virtual-service)
    - [Secure the Virtual Service](#securing-the-virtual-service)
        - [Define an OPA policy](#define-an-opa-policy)
        - [Create an OPA AuthConfig CRD](#create-an-opa-authconfig-crd)
        - [Update the Virtual Service](#updating-the-virtual-service)
    - [Testing our configuration](#testing-the-configuration)
- [Validate JWTs with Open Policy Agent](#validate-jwts-with-open-policy-agent)
    - [Deploy sample application](#deploy-sample-application)
    - [Create a Virtual Service](#create-a-virtual-service)
    - [Secure the Virtual Service](#secure-the-virtual-service)
        - [Install Dex](#install-dex)
        - [Make the client secret accessible to Gloo Edge](#make-the-client-secret-accessible-to-gloo-edge)
        - [Create a Policy](#create-a-policy)
        - [Create a multi-step AuthConfig](#create-a-multi-step-authconfig)
        - [Update the Virtual Service](#update-the-virtual-service)
    - [Testing our configuration](#testing-our-configuration)
- [Troubleshooting OPA](#troubleshooting-opa)

## Setup
{{< readfile file="/static/content/setup_notes" markdown="true">}}

## OPA policy overview
Open Policy Agent policies are written in [Rego](https://www.openpolicyagent.org/docs/latest/how-do-i-write-policies/). The _Rego_ language is inspired from _Datalog_, which in turn is a subset of _Prolog_. _Rego_ is more suited to work with modern JSON documents.

Gloo Edge's OPA integration will populate an `input` document which can be used in your OPA policies. The structure of the `input` document depends on the context of the incoming request. See the following section for details.

### OPA input structure
- `input.check_request` - By default, all OPA policies will contain an [Envoy Auth Service `CheckRequest`](https://www.envoyproxy.io/docs/envoy/latest/api-v2/service/auth/v2/external_auth.proto#service-auth-v2-checkrequest). This object contains all the information Envoy has gathered of the request being processed. See the Envoy docs and [proto files for `AttributeContext`](https://github.com/envoyproxy/envoy/blob/b3949eaf2080809b8a3a6cf720eba2cfdf864472/api/envoy/service/auth/v2/attribute_context.proto#L39) for the structure of this object.
- `input.http_request` - When processing an HTTP request, this field will be populated for convenience. See the [Envoy `HttpRequest` docs](https://www.envoyproxy.io/docs/envoy/latest/api-v2/service/auth/v2/attribute_context.proto#service-auth-v2-attributecontext-httprequest) and [proto files](https://github.com/envoyproxy/envoy/blob/b3949eaf2080809b8a3a6cf720eba2cfdf864472/api/envoy/service/auth/v2/attribute_context.proto#L90) for the structure of this object.
- `input.state.jwt` - When the [OIDC auth plugin]({{< versioned_link_path fromRoot="/guides/security/auth/extauth/oauth/" >}}) is utilized, the token retrieved during the OIDC flow is placed into this field. See the section below on [validating JWTs](#validate-jwts-with-open-policy-agent) for an example.

## Validate requests attributes with Open Policy Agent

### Deploy a sample application
Let's deploy a sample application that we will route requests to during this guide:

```shell script
kubectl apply -f https://raw.githubusercontent.com/solo-io/gloo/v1.14.x/example/petstore/petstore.yaml
```

### Creating a Virtual Service
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

### Securing the Virtual Service
{{% notice warning %}}
{{% extauth_version_info_note %}}
{{% /notice %}}

As we just saw, we were able to reach the upstream without having to provide any credentials. This is because by default Gloo Edge allows any request on routes that do not specify authentication configuration. Let's change this behavior. We will update the Virtual Service so that only requests that comply with a given OPA policy are allowed.

#### Define an OPA policy 

Let's create a Policy to control which actions are allowed on our service:

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

#### Create an OPA AuthConfig CRD
Gloo Edge expects OPA policies to be stored in a Kubernetes ConfigMap, so let's go ahead and create a ConfigMap with the contents of the above policy file:

```
kubectl -n gloo-system create configmap allow-get-users --from-file=policy.rego
```

Now we can create an `AuthConfig` CRD with our OPA authorization configuration:

{{< highlight shell "hl_lines=9-13" >}}
kubectl apply -f - <<EOF
apiVersion: enterprise.gloo.solo.io/v1
kind: AuthConfig
metadata:
  name: opa
  namespace: gloo-system
spec:
  configs:
  - opaAuth:
      modules:
      - name: allow-get-users
        namespace: gloo-system
      query: "data.test.allow == true"
EOF
{{< /highlight >}}

The above `AuthConfig` references the ConfigMap  (`modules`) we created earlier and adds a query that allows access only if the `allow` variable is `true`. 

#### Updating the Virtual Service
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
          name: opa
          namespace: gloo-system
EOF
{{< /highlight >}}

In the above example we have added the configuration to the Virtual Host. Each route belonging to a Virtual Host will inherit its `AuthConfig`, unless it [overwrites or disables]({{% versioned_link_path fromRoot="/guides/security/auth/extauth/#inheritance-rules" %}}) it.

### Testing the configuration
Paths that don't start with `/api/pets` are not authorized (should return 403):
```
curl -s -w "%{http_code}\n" $(glooctl proxy url)/api/
```

Not allowed to delete `pets/1` (should return 403):
```
curl -s -w "%{http_code}\n" $(glooctl proxy url)/api/pets/1 -X DELETE
```

Allowed to delete `pets/2` (should return 204):
```
curl -s -w "%{http_code}\n" $(glooctl proxy url)/api/pets/2 -X DELETE
```

#### Cleanup
You can clean up the resources created in this guide by running:

```
kubectl delete vs -n gloo-system petstore
kubectl delete ac -n gloo-system opa
kubectl delete -f https://raw.githubusercontent.com/solo-io/gloo/v1.14.x/example/petstore/petstore.yaml
rm policy.rego
```

## Validate JWTs with Open Policy Agent
The Open Policy Agent policy language has [in-built support](https://www.openpolicyagent.org/docs/latest/policy-reference/#token-verification)
for [JSON Web Tokens](https://jwt.io/) (JWTs), allowing you to define policies based on the claims contained in a JWT.
If you are using an **authentication** mechanism that conveys identity information via JWTs (e.g. [OpenID Connect](https://en.wikipedia.org/wiki/OpenID_Connect)),
this feature makes it easy to implement **authorization** for authenticated users.

In this guide we will see how to use OPA to enforce policies on the JWTs produced by Gloo Edge's **OpenID Connect** (OIDC) authentication module.

{{% notice note %}}
It is possible to use the OPA without the OIDC authentication module.
Each request that is passed to the OPA check contains all HTTP request headers as part of the
[ `CheckRequest`](https://www.envoyproxy.io/docs/envoy/latest/api-v2/service/auth/v2/external_auth.proto#service-auth-v2-checkrequest)
object provided in `input.check_request`.

For example, a JWT passed via a `api-token` header could be used in a policy as
```
jwt := input.check_request.attributes.request.http.headers["api-token"]
```
{{% /notice %}}

### Deploy sample application
{{% notice warning %}}
The sample `petclinic` application deploys a MySql server. If you are using `minikube` v1.5 to run this guide, this 
service is likely to crash due a `minikube` [issue](https://github.com/kubernetes/minikube/issues/5751). 
To get around this, you can start `minikube` with the following flag:

```shell
minikube start --docker-opt="default-ulimit=nofile=102400:102400" 
```
{{% /notice %}}

Let's deploy a sample web application that we will use to demonstrate these features:

```shell script
kubectl apply -f https://raw.githubusercontent.com/solo-io/gloo/v1.2.9/example/petclinic/petclinic.yaml
```

### Create a Virtual Service
Now we can create a Virtual Service that routes all requests (note the `/` prefix) to the `petclinic` service.

```yaml
apiVersion: gateway.solo.io/v1
kind: VirtualService
metadata:
  name: petclinic
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
              name: petclinic
              namespace: default
            port: 8080
```

To verify that the Virtual Service has been accepted by Gloo Edge, let's port-forward the Gateway Proxy service so that it is 
reachable from you machine at `localhost:8080`:
```
kubectl -n gloo-system port-forward svc/gateway-proxy 8080:80
```

If you open your browser and navigate to [http://localhost:8080](http://localhost:8080) you should see the following page:

![Pet Clinic app homepage]({{% versioned_link_path fromRoot="/img/petclinic-home.png" %}})

### Secure the Virtual Service
As we just saw, we were able to reach the service without having to provide any credentials. This is because by default 
Gloo Edge allows any request on routes that do not specify authentication configuration. Let's change this behavior. 
We will update the Virtual Service so that each request to the sample application is:
 
- authenticated using an **OpenID Connect** flow and
- authorized by applying an **OPA policy** to the resulting JWT [ID token](https://auth0.com/docs/tokens/id-tokens_) 
representing the identity of the authenticated user.

#### Install Dex
To implement the authentication flow, we need an OpenID Connect provider to be running in your cluster. To this end, we 
will deploy the [Dex](https://github.com/dexidp/dex) identity service, as it easy to install and configure.

Let's start by defining a `dex-values.yaml` Helm values file with some bootstrap configuration for Dex:

```yaml
cat > dex-values.yaml <<EOF
config:
  # The base path of dex and the external name of the OpenID Connect service.
  # This is the canonical URL that all clients MUST use to refer to dex. If a
  # path is provided, dex's HTTP service will listen at a non-root URL.
  issuer: http://dex.gloo-system.svc.cluster.local:32000

  # Instead of reading from an external storage, use this list of clients.
  staticClients:
  - id: gloo
    redirectURIs:
    - 'http://localhost:8080/callback'
    name: 'GlooApp'
    secret: secretvalue
  
  # A static list of passwords to login the end user. By identifying here, dex
  # won't look in its underlying storage for passwords.
  staticPasswords:
  - email: "admin@example.com"
    # bcrypt hash of the string "password"
    hash: $2a$10$2b2cU8CPhOTaGrs1HRQuAueS7JTT5ZHsHSzYiFPm1leZck7Mc8T4W
    username: "admin"
    userID: "08a8684b-db88-4b73-90a9-3cd1661f5466"
  - email: "user@example.com"
    # bcrypt hash of the string "password"
    hash: $2a$10$2b2cU8CPhOTaGrs1HRQuAueS7JTT5ZHsHSzYiFPm1leZck7Mc8T4W
    username: "user"
    userID: "123456789-db88-4b73-90a9-3cd1661f5466"
EOF
```

This configures Dex with two static users. Notice the **client secret** with value `secretvalue`.

Using this configuration, we can deploy Dex to our cluster using Helm.

If `help repo list` doesn't list the `stable` repo, invoke:

```shell
helm repo add stable https://charts.helm.sh/stable
```

And then install dex (helm 3 command follows):
```shell
helm install dex --namespace gloo-system stable/dex -f dex-values.yaml
```

#### Make the client secret accessible to Gloo Edge
To be able to act as our OIDC client, Gloo Edge needs to have access to the **client secret** we just defined, so that it can 
use it to identify itself with the Dex authorization server. Gloo Edge expects the client secret to be stored in a specific format 
inside of a Kubernetes `Secret`. 

Let's create the secret and name it `oauth`:

{{< tabs >}}
{{< tab name="glooctl" codelang="shell">}}
glooctl create secret oauth --client-secret secretvalue oauth
{{< /tab >}}
{{< tab name="kubectl" codelang="yaml">}}
apiVersion: v1
kind: Secret
type: extauth.solo.io/oauth
metadata:
  name: oauth
  namespace: gloo-system
data:
  # The value is a base64 encoding of the following YAML:
  # client_secret: secretvalue
  # Gloo Edge expected OAuth client secrets in this format.
  client-secret: Y2xpZW50U2VjcmV0OiBzZWNyZXR2YWx1ZQo=
{{< /tab >}}
{{< /tabs >}} 
<br>

#### Create a Policy
We now need to define a Policy to control access to our sample application based on the properties contained in the JWT 
ID tokens issued to authenticated requests by our OIDC provider. Let's store the policy in a file named `check-jwt.rego` 
(see [the previous guide](#define-an-opa-policy) for more info about the policy language):

```shell
cat <<EOF > check-jwt.rego
package test

default allow = false
[header, payload, signature] := io.jwt.decode(input.state.jwt)

allow {
    payload["email"] == "admin@example.com"
}
allow {
    payload["email"] == "user@example.com"
    not startswith(input.http_request.path, "/owners")
}
EOF
```

This policy allows the request if:

- the user's email is "admin@example.com" **OR**
- the user's email is "user@example.com" **AND** the path being accessed does **NOT** start with `/owners`.

Notice how we are using the `io.jwt.decode` function to decode the JWT and how we access claims in the `payload`. 

Gloo Edge expects OPA policies to be stored in a Kubernetes ConfigMap, so let's go ahead and create a ConfigMap with the 
contents of the above policy file:

```
kubectl --namespace=gloo-system create configmap allow-jwt --from-file=check-jwt.rego
```

#### Create a multi-step AuthConfig
{{% notice warning %}}
{{% extauth_version_info_note %}}
{{% /notice %}}

Now that all the necessary resources are in place we can create the `AuthConfig` resource that we will use to secure our 
Virtual Service.  Save the code block below as `jwt-opa.yaml`.

{{< highlight shell "hl_lines=8-22" >}}
apiVersion: enterprise.gloo.solo.io/v1
kind: AuthConfig
metadata:
  name: jwt-opa
  namespace: gloo-system
spec:
  configs:
  - oauth2:
      oidcAuthorizationCode:
        appUrl: http://localhost:8080
        callbackPath: /callback
        clientId: gloo
        clientSecretRef:
          name: oauth
          namespace: gloo-system
        issuerUrl: http://dex.gloo-system.svc.cluster.local:32000/
        session:
          cookieOptions:
            notSecure: true
        scopes:
        - email
  - opaAuth:
      modules:
      - name: allow-jwt
        namespace: gloo-system
      query: "data.test.allow == true"
{{< /highlight >}}

```
kubectl apply -f jwt-opa.yaml
```

The above `AuthConfig` defines two configurations that Gloo Edge will execute in order: 

1. First, Gloo Edge will use its extauth OIDC module to authenticate the incoming request. If authentication was successful, 
Gloo Edge will add the JWT ID token to the `Authorization` request header and execute the next configuration; otherwise it 
will deny the request. Notice how the configuration references the client secret we created earlier and compare the 
configuration values with the ones we used to bootstrap Dex.
1. If authentication was successful, Gloo Edge will check the request against the `allow-jwt` OPA policy to determine whether 
it should be allowed. Notice how the configuration references the `modules` ConfigMap we created earlier and defines a 
query that allows access only if the `allow` variable in the policy evaluates to `true`.

#### Update the Virtual Service
Once the AuthConfig has been created, we can use it to secure our Virtual Service:

{{< highlight yaml "hl_lines=20-24" >}}
apiVersion: gateway.solo.io/v1
kind: VirtualService
metadata:
  name: petclinic
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
              name: petclinic
              namespace: default
            port: 8080
    options:
      extauth:
        configRef:
          name: jwt-opa
          namespace: gloo-system
{{< /highlight >}}

### Testing our configuration
The OIDC flow redirects the client (in this case, your browser) to a login page hosted by Dex. Since Dex is running in 
your cluster and is not publicly reachable, we need some additional configuration to make our example work. Please note 
that this is just a workaround to reduce the amount of configuration necessary for this example to work.

1. Port-forward the Dex service so that it is reachable from you machine at `localhost:32000`:
```shell
kubectl -n gloo-system port-forward svc/dex 32000:32000 & 
portForwardPid1=$! # Store the port-forward pid so we can kill the process later
```

1. Add an entry to the `/etc/hosts` file on your machine, mapping the `dex.gloo-system.svc.cluster.local` hostname to your 
`localhost` (the loopback IP address `127.0.0.1`).
```shell
echo "127.0.0.1 dex.gloo-system.svc.cluster.local" | sudo tee -a /etc/hosts
```

1. Port-forward the Gloo Edge Proxy service so that it is reachable from you machine at `localhost:8080`:
```
kubectl -n gloo-system port-forward svc/gateway-proxy 8080:80 &
portForwardPid2=$!
```

Now we are ready to test our complete setup! Open you browser and navigate to
[http://localhost:8080](http://localhost:8080). You should see the following login page:

![Dex login page]({{% versioned_link_path fromRoot="/img/dex-login.png" %}})

{{% notice note %}}
As the demo app doesn't have a sign-out button, use a private browser window (also known as incognito mode) to access the demo app. This will make it easy to change the user we logged in with.
If you would like to change the logged in user, just close and re-open the private browser window.
{{% /notice %}}

You can login with `admin@example.com` or `user@example.com` with the password `password`. Notice that the admin user 
has access to all pages, while the regular user can't access the `"Find Owners"` page.

### Cleanup
You can clean up the resources created in this guide by running:

```
sudo sed '/127.0.0.1 dex.gloo-system.svc.cluster.local/d' /etc/hosts # remove line from hosts file
kill $portForwardPid1
kill $portForwardPid2
helm delete --purge dex
kubectl delete -n gloo-system secret oauth dex-grpc-ca  dex-grpc-client-tls  dex-grpc-server-tls  dex-web-server-ca  dex-web-server-tls
kubectl delete virtualservice -n gloo-system petclinic
kubectl delete authconfig -n gloo-system jwt-opa
kubectl delete -n gloo-system configmap allow-jwt
kubectl delete -f https://raw.githubusercontent.com/solo-io/gloo/v1.2.9/example/petclinic/petclinic.yaml
rm check-jwt.rego dex-values.yaml
```

## Troubleshooting OPA

You can get more insight into the exact behavior of your OPA configuration by turning on debug 
logging in the `extauth` deployment. 

First, expose the pod on port 9091 with: 
```
kubectl port-forward -n gloo-system deploy/extauth 9091
```

Now, you can set the log level to debug with the following curl command:
```
curl -XPUT -H "Content-Type: application/json" localhost:9091/logging -d '{"level":"debug"}'
```

With debug logging enabled, the extauth server logs will now contain the OPA input, evaluation trace, 
and result. Run this command to stream the OPA debug log in a user-friendly way, for easy, real-time
debugging:
```
kubectl logs -n gloo-system deploy/extauth -f | jq -r -j "select(.trace != null) | .trace"
```

Note: this command uses `jq` for parsing and pretty-printing the structured json logs. 

When OPA is enabled on a route and a request is submitted, you'll start to see OPA trace logs. Here's an example:

```
{
  "check_request": {
    "attributes": {
      "source": {
        "address": {
          "Address": {
            "SocketAddress": {
              "address": "10.142.0.53",
              "PortSpecifier": {
                "PortValue": 58181
              }
            }
          }
        }
      },
      "destination": {
        "address": {
          "Address": {
            "SocketAddress": {
              "address": "10.52.1.28",
              "PortSpecifier": {
                "PortValue": 8080
              }
            }
          }
        }
      },
      "request": {
        "time": {
          "seconds": 1586274307,
          "nanos": 128021000
        },
        "http": {
          "id": "8223534253882703044",
          "method": "GET",
          "headers": {
            ":authority": "35.227.100.106:80",
            ":method": "GET",
            ":path": "/sample-route-1",
            "accept-encoding": "gzip",
            "user-agent": "Go-http-client/1.1",
            "x-forwarded-proto": "http",
            "x-request-id": "5477c542-4145-459a-8ef3-c6b27d126982",
            "x-token": "eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9.eyJpc3MiOiJzb2xvLmlvIiwic3ViIjoiMTIzNDU2Nzg5MCIsInR5cGUiOiJTTVMiLCJudW1iZXIiOiIyMDAifQ.quxs99EylhY2Eod3Ns-NkGRAVbM3riZLQLaCHvPPcpeTn7fEmcATPL82rZvUENLX6nsj_FXetd5dpvAJwPTCTRFhnEmVlK6J9i46nNqlA2JAFwXTww4WlrrpoD6p1fGoq5cGqzqdNBrfK-uee1w5N-c5de3waLAQXK7W6_x-L-0ovAqb0wz4i-fIcTKHGELpReGCh762rrj_iMuwaZMg3SJmIfSbGB7SFfdCcY1kE8fTdwZayoxzG1EzeNFTHd7D-h1Y_odafi_PGn5zwkpU4NkBqTcPx2TbZCS5QPG9VjSgWIi5cWW1tQiPyuv7UOmjgmgZFbXXG-Uf_SBpPZdUhg"
          },
          "path": "/sample-route-1",
          "host": "35.227.100.106:80",
          "protocol": "HTTP/1.1"
        }
      },
      "context_extensions": {
        "config_id": "gloo-system.opa-auth",
        "source_name": "gloo-system.gateway-proxy-listener-::-8080-gloo-system_petstore",
        "source_type": "virtual_host"
      },
      "metadata_context": {}
    }
  },
  "http_request": {
    "id": "8223534253882703044",
    "method": "GET",
    "headers": {
      ":authority": "35.227.100.106:80",
      ":method": "GET",
      ":path": "/sample-route-1",
      "accept-encoding": "gzip",
      "user-agent": "Go-http-client/1.1",
      "x-forwarded-proto": "http",
      "x-request-id": "5477c542-4145-459a-8ef3-c6b27d126982",
      "x-token": "eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9.eyJpc3MiOiJzb2xvLmlvIiwic3ViIjoiMTIzNDU2Nzg5MCIsInR5cGUiOiJTTVMiLCJudW1iZXIiOiIyMDAifQ.quxs99EylhY2Eod3Ns-NkGRAVbM3riZLQLaCHvPPcpeTn7fEmcATPL82rZvUENLX6nsj_FXetd5dpvAJwPTCTRFhnEmVlK6J9i46nNqlA2JAFwXTww4WlrrpoD6p1fGoq5cGqzqdNBrfK-uee1w5N-c5de3waLAQXK7W6_x-L-0ovAqb0wz4i-fIcTKHGELpReGCh762rrj_iMuwaZMg3SJmIfSbGB7SFfdCcY1kE8fTdwZayoxzG1EzeNFTHd7D-h1Y_odafi_PGn5zwkpU4NkBqTcPx2TbZCS5QPG9VjSgWIi5cWW1tQiPyuv7UOmjgmgZFbXXG-Uf_SBpPZdUhg"
    },
    "path": "/sample-route-1",
    "host": "35.227.100.106:80",
    "protocol": "HTTP/1.1"
  },
  "state": null
}Enter __localq0__ = data.test.allow; equal(__localq0__, true, _)
| Eval __localq0__ = data.test.allow
| Index __localq0__ = data.test.allow matched 0 rules)
| Enter data.test.allow
| | Eval true
| | Exit data.test.allow
| Eval equal(__localq0__, true, _)
| Exit __localq0__ = data.test.allow; equal(__localq0__, true, _)
Redo __localq0__ = data.test.allow; equal(__localq0__, true, _)
| Redo equal(__localq0__, true, _)
| Redo __localq0__ = data.test.allow
| Redo data.test.allow
| | Redo true
{
  "expressions": [
    {
      "value": false,
      "text": "data.test.allow == true",
      "location": {
        "row": 1,
        "col": 1
      }
    }
  ]

```

