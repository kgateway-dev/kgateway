---
title: JWT and Access Control
weight: 2
description: JWT verification and Access Control (without an external auth server)
---

{{% notice note %}}
The JWT feature was introduced with **Gloo Edge Enterprise**, release 0.13.16. If you are using an earlier version, this tutorial will not work.
{{% /notice %}}

## Table of Contents
- [Table of Contents](#table-of-contents)
- [Setup](#setup)
- [Verifying Kubernetes service account JWTs](#verifying-kubernetes-service-account-jwts)
  - [Deploy sample application](#deploy-sample-application)
  - [Create a Virtual Service](#create-a-virtual-service)
  - [Setting up JWT authorization](#setting-up-jwt-authorization)
    - [Anatomy of Kubernetes service account](#anatomy-of-kubernetes-service-account)
    - [Retrieve the Kubernetes API server public key](#retrieve-the-kubernetes-api-server-public-key)
    - [Secure the Virtual Service](#secure-the-virtual-service)
  - [Testing our configuration](#testing-our-configuration)
  - [Cleanup](#cleanup)
- [Appendix - Use a remote JSON Web Key Set (JWKS) server](#appendix---use-a-remote-json-web-key-set-jwks-server)
  - [Create the private key](#create-the-private-key)
  - [Create the JSON Web Key Set (JWKS)](#create-the-json-web-key-set-jwks)
  - [Create JWKS server](#create-jwks-server)
  - [Create the JSON Web Token (JWT)](#create-the-json-web-token-jwt)
  - [Testing the configuration](#testing-the-configuration)
  - [Cleanup](#cleanup-1)
    
    
## Setup
{{< readfile file="/static/content/setup_notes" markdown="true">}}

It is also assumed that you are using a local `minikube` cluster.

## Verifying Kubernetes service account JWTs
In this guide, we will show how to use Gloo Edge to verify Kubernetes service account JWTs and how to define RBAC policies to 
control the resources service accounts are allowed to access.

### Deploy sample application
Let's deploy a sample application that we will route requests to during this guide:

```shell
kubectl apply -f https://raw.githubusercontent.com/solo-io/gloo/v1.14.x/example/petstore/petstore.yaml
```

### Create a Virtual Service
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

### Setting up JWT authorization
Let's create a test pod, with a different service account. We will use this pod to test access with the new service account credentials.

```shell
kubectl create serviceaccount svc-a
```

We can use a simple `test-pod` deployment that invokes a `sleep` command.

```
apiVersion: apps/v1
kind: Deployment
metadata:
  name: test-pod
  labels:
    app: test-pod
spec:
  replicas: 1
  selector:
    matchLabels:
      app: test-pod
  template:
    metadata:
      labels:
        app: test-pod
    spec:
      containers:
      - name: sleep
        image: fedora:30
        command: ['sleep','10h']
      serviceAccountName: svc-a
```

#### Anatomy of Kubernetes service account
A service account provides an identity for processes that run inside a Pod. When kubernetes starts a pod, it automatically generates a JWT contains information about the pod's service account and attaches it to the pod. Inside the JWT are *claims* that provide identity information, and a signature for verification. To verify these JWTs, the Kubernetes API server is provided with a public key. Gloo Edge can use this public key to perform JWT verification for kubernetes service accounts.

Let's see the claims for `svc-a`, the service account we just created:

```shell
# Execute a command inside the pod to copy the payload of the JWT to the CLAIMS shell variable.
# The three parts of a JWT are separated by dots: header.payload.signature
CLAIMS=$(kubectl exec test-pod -- cat /var/run/secrets/kubernetes.io/serviceaccount/token | cut -d. -f2)

# Pad the CLAIMS string to ensure that we can display valid JSON
PADDING_LEN=$(( 4 - ( ${#CLAIMS} % 4 ) ))
PADDING=$(head -c $PADDING_LEN /dev/zero | tr '\0' =)
PADDED_CLAIMS="${CLAIMS}${PADDING}"

# Note: the `jq` utility makes the output easier to read. It can be omitted if you do not have it installed
echo $PADDED_CLAIMS | base64 --decode | jq
```

The output should look like so:
```json
{
  "iss": "kubernetes/serviceaccount",
  "kubernetes.io/serviceaccount/namespace": "default",
  "kubernetes.io/serviceaccount/secret.name": "svc-a-token-tssts",
  "kubernetes.io/serviceaccount/service-account.name": "svc-a",
  "kubernetes.io/serviceaccount/service-account.uid": "279d1e33-8d59-11e9-8f04-80c697af5b67",
  "sub": "system:serviceaccount:default:svc-a"
}
```

{{% notice note %}}
In your output the `kubernetes.io/serviceaccount/service-account.uid` claim will be different than displayed here.
{{% /notice %}}

The most important claims for this guide are the **iss** claim and the **sub** claim. We will use these claims later to verify the identity of the JWT.

#### Retrieve the Kubernetes API server public key

Let's get the public key that the Kubernetes API server uses to  verify service accounts:

```shell
minikube ssh sudo cat /var/lib/minikube/certs/sa.pub | tee public-key.pem
```

This command will output the public key, and will save it to a file called `public-key.pem`. The content of the 
`public-key.pem` file key should look similar to the following:

```text
-----BEGIN PUBLIC KEY-----
MIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKCAQEA4XbzUpqbgKbDLngsLp4b
pjf04WkMzXx8QsZAorkuGprIc2BYVwAmWD2tZvez4769QfXsohu85NRviYsrqbyC
w/NTs3fMlcgld+ayfb/1X3+6u4f1Q8JsDm4fkSWoBUlTkWO7Mcts2hF8OJ8LlGSw
zUDj3TJLQXwtfM0Ty1VzGJQMJELeBuOYHl/jaTdGogI8zbhDZ986CaIfO+q/UM5u
kDA3NJ7oBQEH78N6BTsFpjDUKeTae883CCsRDbsytWgfKT8oA7C4BFkvRqVMSek7
FYkg7AesknSyCIVMObSaf6ZO3T2jVGrWc0iKfrR3Oo7WpiMH84SdBYXPaS1VdLC1
7QIDAQAB
-----END PUBLIC KEY-----
```

{{% notice note %}}
If the above command doesn't produce the expected output, it could be that the `/var/lib/minikube/certs/sa.pub` is different on your minikube. The public key is given to the Kubernetes API server in the `--service-account-key-file` command line flag. You can check which value was passed via this flag by running `minikube ssh ps ax ww | grep kube-apiserver`.
{{% /notice %}}

#### Secure the Virtual Service
Now let's configure our Virtual Service to verify JWTs in the incoming request using this public key:

{{< highlight shell "hl_lines=20-36" >}}
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
      jwt:
        providers:
          kube:
            issuer: kubernetes/serviceaccount
            jwks:
              local:
                key: |
                  -----BEGIN PUBLIC KEY-----
                  MIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKCAQEApj2ac/hNZLm/66yoDQJ2
                  mNtQPX+3RXcTMhLnChtFEsvpDhoMlS0Gakqkmg78OGWs7U4f6m1nD/Jc7UThxxks
                  o9x676sxxLKxo8TC1w6t47HQHucJE0O5wFNtC8+4jwl4zOBVwnkAEeN+X9jJq2E7
                  AZ+K6hUycOkWo8ZtZx4rm1bnlDykOa9VCuG3MCKXNexujLIixHOeEOylp7wNedSZ
                  4Wfc5rM9Cich2F6pIoCwslHYcED+3FZ1ZmQ07h1GG7Aaak4N4XVeJLsDuO88eVkv
                  FHlGdkW6zSj9HCz10XkSPK7LENbgHxyP6Foqw10MANFBMDQpZfNUHVPSo8IaI+Ot
                  xQIDAQAB
                  -----END PUBLIC KEY-----
{{< /highlight >}}

With the above configuration, the Virtual Service will look for a JWT on incoming requests and allow the request only if:
 
- a JWT is present,
- it can be verified with the given public key, and 
- it has  an `iss` claim with value `kubernetes/serviceaccount`.

{{% notice note %}}
To see all the attributes supported by the JWT API, be sure to check out the correspondent <b>{{< protobuf display="API docs" name="jwt.options.gloo.solo.io.VhostExtension">}}</b>.
{{% /notice %}}

To make things more interesting, we can further configure Gloo Edge to enforce an access control policy on incoming JWTs. Let's add a policy to our Virtual Service:

{{< highlight shell "hl_lines=37-48" >}}
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
      jwt:
        providers:
          kube:
            issuer: kubernetes/serviceaccount
            jwks:
              local:
                key: |
                  -----BEGIN PUBLIC KEY-----
                  MIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKCAQEApj2ac/hNZLm/66yoDQJ2
                  mNtQPX+3RXcTMhLnChtFEsvpDhoMlS0Gakqkmg78OGWs7U4f6m1nD/Jc7UThxxks
                  o9x676sxxLKxo8TC1w6t47HQHucJE0O5wFNtC8+4jwl4zOBVwnkAEeN+X9jJq2E7
                  AZ+K6hUycOkWo8ZtZx4rm1bnlDykOa9VCuG3MCKXNexujLIixHOeEOylp7wNedSZ
                  4Wfc5rM9Cich2F6pIoCwslHYcED+3FZ1ZmQ07h1GG7Aaak4N4XVeJLsDuO88eVkv
                  FHlGdkW6zSj9HCz10XkSPK7LENbgHxyP6Foqw10MANFBMDQpZfNUHVPSo8IaI+Ot
                  xQIDAQAB
                  -----END PUBLIC KEY-----
      rbac:
        policies:
          viewer:
            permissions:
              methods:
              - GET
              pathPrefix: /api/pets
            principals:
            - jwtPrincipal:
                claims:
                  sub: system:serviceaccount:default:svc-a
{{< /highlight >}}

The above configuration defines an RBAC policy named `viewer` which only allows requests upstream if:

- the request method is `GET`
- the request URI starts with `/api/pets`
- the request contains a verifiable JWT
- the JWT has a `sub` claim with value `system:serviceaccount:default:svc-a`
  - **Note**: By default, matching is supported for only top-level claims of the JWT. To enable matching against nested claims, or claims that are children of top-level claims, see [Matching against nested JWT claims](./access_control_examples/#matching-against-nested-jwt-claims).
  - **Note**: By default, claims are matched against values by using exact string comparison. To instead match claims against non-string values, see [Matching against non-string JWT claim values](./access_control_examples/#matching-against-non-string-jwt-claims).
{{% notice note %}}
To see all the attributes supported by the RBAC API, be sure to check out the correspondent <b>{{< protobuf display="API docs" name="rbac.options.gloo.solo.io.ExtensionSettings">}}</b>.
{{% /notice %}}

### Testing our configuration
Now we are ready to test our configuration. We will be sending requests from inside the `test-pod` pod that we deployed at the beginning of this guide. Remember that the encrypted JWT is stored inside the pod under `/var/run/secrets/kubernetes.io/serviceaccount/token`.

An unauthenticated request should fail:
```shell
kubectl exec test-pod -- bash -c 'curl -sv http://gateway-proxy.gloo-system/api/pets/1'
```
{{< highlight shell "hl_lines=6 12" >}}
> GET /api/pets/1 HTTP/1.1
> Host: gateway-proxy.gloo-system
> User-Agent: curl/7.65.3
> Accept: */*
>
< HTTP/1.1 401 Unauthorized
< content-length: 14
< content-type: text/plain
< date: Sat, 09 Nov 2019 23:05:56 GMT
< server: envoy
<
Jwt is missing%
{{< /highlight >}}

An authenticated GET request to a path that starts with `/api/pets` should succeed:
```shell
kubectl exec test-pod -- bash -c 'curl -sv http://gateway-proxy.gloo-system/api/pets/1 \
    -H "Authorization: Bearer $(cat /var/run/secrets/kubernetes.io/serviceaccount/token)"'
```
{{< highlight shell "hl_lines=7" >}}
> GET /api/pets/1 HTTP/1.1
> Host: gateway-proxy.gloo-system
> User-Agent: curl/7.65.3
> Accept: */*
> Authorization: Bearer <this is the JWT>
>
< HTTP/1.1 200 OK
< content-type: text/xml
< date: Sat, 09 Nov 2019 23:09:43 GMT
< content-length: 43
< x-envoy-upstream-service-time: 2
< server: envoy
<
{{< /highlight >}}

An authenticated POST request to a path that starts with `/api/pets` should fail:
```shell
kubectl exec test-pod -- bash -c 'curl -sv -X POST http://gateway-proxy.gloo-system/api/pets/1 \
    -H "Authorization: Bearer $(cat /var/run/secrets/kubernetes.io/serviceaccount/token)"'
```
{{< highlight shell "hl_lines=7 13" >}}
> POST /api/pets/1 HTTP/1.1
> Host: gateway-proxy.gloo-system
> User-Agent: curl/7.65.3
> Accept: */*
> Authorization: Bearer <this is the JWT>
>
< HTTP/1.1 403 Forbidden
< content-length: 19
< content-type: text/plain
< date: Sat, 09 Nov 2019 23:13:06 GMT
< server: envoy
<
RBAC: access denied%
{{< /highlight >}}

An authenticated GET request to a path that doesn't start with `/api/pets` should fail:
```shell
kubectl exec test-pod -- bash -c 'curl -sv http://gateway-proxy.gloo-system/foo/ \
    -H "Authorization: Bearer $(cat /var/run/secrets/kubernetes.io/serviceaccount/token)"'
```
{{< highlight shell "hl_lines=7 13" >}}
> GET /foo/ HTTP/1.1
> Host: gateway-proxy.gloo-system
> User-Agent: curl/7.65.3
> Accept: */*
> Authorization: Bearer <this is the JWT>
>
< HTTP/1.1 403 Forbidden
< content-length: 19
< content-type: text/plain
< date: Sat, 09 Nov 2019 23:15:32 GMT
< server: envoy
<
RBAC: access denied%
{{< /highlight >}}

### Cleanup
You can clean up the resources created in this guide by running:

```shell
rm public-key.pem
kubectl delete pod test-pod
kubectl delete virtualservice -n gloo-system petstore
# Don't remove the sample application if you want to run through the appendix to this guide
kubectl delete -f https://raw.githubusercontent.com/solo-io/gloo/v1.14.x/example/petstore/petstore.yaml
```

## Appendix - Use a remote JSON Web Key Set (JWKS) server
In the previous part of the guide we saw how to configure Gloo Edge with a public key to verify JWTs. The way we provided Gloo Edge with the key was to include the key itself into the Virtual Service definition. While the simplicity of this approach make it a good candidate for test setups and quick prototyping, it can quickly become unwieldy. A more flexible and scalable approach is to use a **JSON Web Key Set (JWKS) Server**. A JWKS server allows us to manage the verification keys independently and centrally, making routine tasks such as key rotation much easier.

In this appendix we will demonstrate how to use an external JSON Web Key Set (JWKS) server with Gloo Edge. We will:

1. Create a private key that will be used to sign and verify custom JWTs that we will create;
1. Convert the key from PEM to JSON Web Key format;
1. Deploy a JWKS server to serve the key;
1. Configure Gloo Edge to verify JWTs using the key stored in the server;
1. Create and sign a custom JWT and use it to authenticate with Gloo Edge.

### Create the private key
Let's start by creating a private key that we will use to sign our JWTs:
```shell
openssl genrsa 2048 > private-key.pem
```

{{% notice warning %}}
Storing a key on your laptop as done here is not considered secure! Do not use this workflow for production workloads. Use appropriate secret management tools to store sensitive information.
{{% /notice %}}

### Create the JSON Web Key Set (JWKS)
We can use the `openssl` command to extract a PEM encoded public key from the private key. We can then use the `pem-jwk` utility to convert our public key to a JSON Web Key format.

```shell
# install pem-jwk utility.
npm install -g pem-jwk

# extract public key and convert it to JWK.
openssl rsa -in private-key.pem -pubout | pem-jwk | jq . > jwks.json
```

The resulting `jwks.json` file should have the following format:
```json
{
  "kty": "RSA",
  "n": "4XbzUpqbgKbDLngsLp4bpjf04WkMzXx8QsZAorkuGprIc2BYVwAmWD2tZvez4769QfXsohu85NRviYsrqbyCw_NTs3fMlcgld-ayfb_1X3-6u4f1Q8JsDm4fkSWoBUlTkWO7Mcts2hF8OJ8LlGSwzUDj3TJLQXwtfM0Ty1VzGJQMJELeBuOYHl_jaTdGogI8zbhDZ986CaIfO-q_UM5ukDA3NJ7oBQEH78N6BTsFpjDUKeTae883CCsRDbsytWgfKT8oA7C4BFkvRqVMSek7FYkg7AesknSyCIVMObSaf6ZO3T2jVGrWc0iKfrR3Oo7WpiMH84SdBYXPaS1VdLC17Q",
  "e": "AQAB"
}
```

To that we'll add the signing algorithm and usage:
```shell
jq '.+{alg:"RS256"}|.+{use:"sig"}' jwks.json | tee tmp.json && mv tmp.json jwks.json
```
{{< highlight json "hl_lines=5-6" >}}
{
    "kty": "RSA",
    "n": "4XbzUpqbgKbDLngsLp4bpjf04WkMzXx8QsZAorkuGprIc2BYVwAmWD2tZvez4769QfXsohu85NRviYsrqbyCw_NTs3fMlcgld-ayfb_1X3-6u4f1Q8JsDm4fkSWoBUlTkWO7Mcts2hF8OJ8LlGSwzUDj3TJLQXwtfM0Ty1VzGJQMJELeBuOYHl_jaTdGogI8zbhDZ986CaIfO-q_UM5ukDA3NJ7oBQEH78N6BTsFpjDUKeTae883CCsRDbsytWgfKT8oA7C4BFkvRqVMSek7FYkg7AesknSyCIVMObSaf6ZO3T2jVGrWc0iKfrR3Oo7WpiMH84SdBYXPaS1VdLC17Q",
    "e": "AQAB",
    "alg": "RS256",
    "use": "sig"
}
{{< /highlight >}}

{{% notice note %}}
For details about the above JWT fields, see <b>[this section](https://tools.ietf.org/html/rfc7517#section-4)</b> of the JWT specification. 
{{% /notice %}}

Finally, let's turn the single key into a key set:
```shell
jq '{"keys":[.]}' jwks.json | tee tmp.json && mv tmp.json jwks.json
```
{{< highlight json "hl_lines=1-2 10-11" >}}
{
    "keys": [
        {
            "kty": "RSA",
            "n": "4XbzUpqbgKbDLngsLp4bpjf04WkMzXx8QsZAorkuGprIc2BYVwAmWD2tZvez4769QfXsohu85NRviYsrqbyCw_NTs3fMlcgld-ayfb_1X3-6u4f1Q8JsDm4fkSWoBUlTkWO7Mcts2hF8OJ8LlGSwzUDj3TJLQXwtfM0Ty1VzGJQMJELeBuOYHl_jaTdGogI8zbhDZ986CaIfO-q_UM5ukDA3NJ7oBQEH78N6BTsFpjDUKeTae883CCsRDbsytWgfKT8oA7C4BFkvRqVMSek7FYkg7AesknSyCIVMObSaf6ZO3T2jVGrWc0iKfrR3Oo7WpiMH84SdBYXPaS1VdLC17Q",
            "e": "AQAB",
            "alg": "RS256",
            "use": "sig"
        }
    ]
}
{{< /highlight >}}

Our `jwks.json` file now contains a valid JSON Web Key Set (JWKS).

### Create JWKS server
Let's create our JWKS server. All that the server needs to do is to serve a JSON Web Key Set file. Later we will configure Gloo Edge to grab the JSON Web Key Set from the server.

We will start by copying the `jwks.json` to a ConfigMap:

```shell
kubectl -n gloo-system create configmap jwks --from-file=jwks.json=jwks.json
```

Now let's mount the ConfigMap to an `nginx` container that will serve as our JWKS server and expose the deployment as a service:

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  labels:
    app: jwks-server
  name: jwks-server
  namespace: gloo-system
spec:
  selector:
    matchLabels:
      app: jwks-server
  template:
    metadata:
      labels:
        app: jwks-server
    spec:
      containers:
      - image: nginx
        name: nginx
        volumeMounts:
        - mountPath: /usr/share/nginx/html
          name: jwks-vol
      volumes:
      - configMap:
          name: jwks
        name: jwks-vol
---
apiVersion: v1
kind: Service
metadata:
  labels:
    app: jwks-server
  name: jwks-server
  namespace: gloo-system
spec:
  ports:
  - port: 80
    protocol: TCP
    targetPort: 80
  selector:
    app: jwks-server
```

Gloo Edge should have discovered the service and created an upstream named `gloo-system-jwks-server-80`; you can verify this by running `kubectl get upstreams -n gloo-system`. Should this not be the case (for example because you have disabled the discovery feature), you have to create the upstream yourself:

{{< tabs >}}
{{< tab name="glooctl" codelang="shell">}}
glooctl create upstream kube --kube-service jwks-server --kube-service-namespace gloo-system --kube-service-port 80 -n gloo-system gloo-system-jwks-server-80
{{< /tab >}}
{{< tab name="kubectl" codelang="yaml">}}
apiVersion: gloo.solo.io/v1
kind: Upstream
metadata:
  labels:
    app: jwks-server
  name: gloo-system-jwks-server-80
  namespace: gloo-system
spec:
  kube:
    selector:
      app: jwks-server
    serviceName: jwks-server
    serviceNamespace: gloo-system
    servicePort: 80
{{< /tab >}}
{{< /tabs >}} 

Now we can configure Gloo Edge to use the JWKS server:

{{< highlight yaml "hl_lines=20-30" >}}
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
      jwt:
        providers:
          solo-provider:
            issuer: solo.io
            jwks:
              remote:
                upstreamRef:
                  name: gloo-system-jwks-server-80
                  namespace: gloo-system
                url: http://jwks-server/jwks.json
{{< /highlight >}}

### Create the JSON Web Token (JWT)
We have everything we need to sign and verify a custom JWT with our custom claims. We will use the [jwt.io](https://jwt.io) debugger to do so easily.

- Go to https://jwt.io.
- In the "Debugger" section, change the algorithm combo-box to "RS256".
- In the "VERIFY SIGNATURE" section, paste the contents of the file `private-key.pem` to the 
  bottom box (labeled "Private Key").
- Paste the following to the payload data (replacing what is already there):

Payload:
```json
{
  "iss": "solo.io",
  "sub": "1234567890",
  "solo.io/company":"solo"
}
```

You should now have an encoded JWT token in the "Encoded" box. Copy it and save to to a file called `token.jwt`

{{% notice note %}}
You may have noticed **jwt.io** complaining about an invalid signature in the bottom left corner. This is fine because we don't need the public key to create an encoded JWT.If you'd like to resolve the invalid signature, under the "VERIFY SIGNATURE" section, paste the output of `openssl rsa -pubout -in private-key.pem` to the bottom box (labeled "Public Key")
{{% /notice %}}

Here is an image of how the page should look like (click to enlarge):

<img src="../jwt-io.png" alt="jwt.io debugger" style="border: solid 1px; color: lightgrey" width="500px"/>

### Testing the configuration
Now we are ready to test our configuration. Let's port-forward the Gateway Proxy service so that it is reachable from your machine at `localhost:8080`:

```
kubectl -n gloo-system port-forward svc/gateway-proxy 8080:80 &
portForwardPid=$!
```

A request without a token should be rejected (will output *Jwt is missing*):

```shell
curl -sv "localhost:8080/api/pets"
```
{{< highlight shell "hl_lines=6 12" >}}
> GET /api/pets HTTP/1.1
> Host: localhost:8080
> User-Agent: curl/7.54.0
> Accept: */*
>
< HTTP/1.1 401 Unauthorized
< content-length: 14
< content-type: text/plain
< date: Sun, 10 Nov 2019 01:35:02 GMT
< server: envoy
<
Jwt is missing%
{{< /highlight >}}

A request with a token should be accepted:

```shell
curl -sv "localhost:8080/api/pets?access_token=$(cat token.jwt)"
```
{{< highlight shell "hl_lines=6 13" >}}
> GET /api/pets?access_token= <JWT here> HTTP/1.1
> Host: localhost:8080
> User-Agent: curl/7.54.0
> Accept: */*
>
< HTTP/1.1 200 OK
< content-type: application/xml
< date: Sun, 10 Nov 2019 01:35:09 GMT
< content-length: 86
< x-envoy-upstream-service-time: 1
< server: envoy
<
[{"id":1,"name":"Dog","status":"available"},{"id":2,"name":"Cat","status":"pending"}]
{{< /highlight >}}

### Cleanup
You can clean up the resources created in this guide by running:

```shell
kill $portForwardPid
rm private-key.pem jwks.json token.jwt
kubectl -n gloo-system delete svc jwks-server
kubectl -n gloo-system delete deployment jwks-server
kubectl -n gloo-system delete configmap jwks
kubectl delete virtualservice -n gloo-system petstore
kubectl delete -f https://raw.githubusercontent.com/solo-io/gloo/v1.14.x/example/petstore/petstore.yaml
```
