---
title: JWT and Access Control
weight: 2
description: JWT verification and Access Control (without an external auth server)
---

{{% notice note %}}
The JWT feature was introduced with **Gloo Enterprise**, release 0.13.16. If you are using an earlier version, this tutorial will not work.
{{% /notice %}}

In this guide, we will show how to use Gloo to verify kubernetes service account JWTs and how to define RBAC policies to 
control the resources service accounts are allowed to access.

## Setup
{{< readfile file="/static/content/setup_notes" markdown="true">}}

It is also assumed that you are using a local `minikube` cluster.

### Deploy sample application
Let's deploy a sample application that we will route requests to during this guide:

```shell script
kubectl apply -f https://raw.githubusercontent.com/solo-io/gloo/master/example/petstore/petstore.yaml
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
    - matcher:
        prefix: /
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
curl $GATEWAY_URL/api/pets
```

You should see the following output:

```json
[{"id":1,"name":"Dog","status":"available"},{"id":2,"name":"Cat","status":"pending"}]
```

## Setting up JWT authorization
Let's create a test pod, with a different service account. We will use this pod to test access 
with the new service account credentials.

```shell
kubectl create serviceaccount svc-a
kubectl run --generator=run-pod/v1 test-pod --image=fedora:30 --serviceaccount=svc-a --command sleep 10h
```

### Anatomy of kubernetes service account
A service account provides an identity for processes that run inside a Pod. When kubernetes starts a pod, it automatically 
generates a JWT contains information about the pod's service account and attaches it to the pod. 
Inside the JWT are *claims* that provide identity information, and a signature for verification. 
To verify these JWTs, the Kubernetes API server is provided with a public key. Gloo can use this public key to perform 
JWT verification for kubernetes service accounts.

Let's see the claims for `svc-a`, the service account we just created:

```shell
# Execute a command inside the pod to copy the payload of the JWT to the CLAIMS shell variable.
# The three parts of a JWT are separated by dots: header.payload.signature
CLAIMS=$(kubectl exec test-pod cat /var/run/secrets/kubernetes.io/serviceaccount/token | cut -d. -f2)

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

The most important claims for this guide are the **iss** claim and the **sub** claim. We will use these
claims later to verify the identity of the JWT.

### Retrieve the Kubernetes API server public key
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
If the above command doesn't produce the expected output, it could be that the `/var/lib/minikube/certs/sa.pub` is 
different on your minikube. The public key is given to the Kubernetes API server in the `--service-account-key-file` 
command line flag. You can check which value was passed via this flag by running `minikube ssh ps ax ww | grep kube-apiserver`.
{{% /notice %}}

### Secure the Virtual Service
Now let's configure our Virtual Service to verify JWTs in incoming request using this public key:

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
To see all the attributes supported by the JWT API, be sure to check out the correspondent 
<b>{{< protobuf display="API docs" name="jwt.plugins.gloo.solo.io.VhostExtension">}}</b>.
{{% /notice %}}

To make things more interesting, we can further configure Gloo to enforce an access control policy on incoming JWTs. 
Let's add a policy to our Virtual Service:

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

{{% notice note %}}
To see all the attributes supported by the RBAC API, be sure to check out the correspondent 
<b>{{< protobuf display="API docs" name="rbac.plugins.gloo.solo.io.ExtensionSettings">}}</b>.
{{% /notice %}}

### Testing our configuration
Now we are ready to test our configuration. We will be sending requests from inside the `test-pod` pod that we deployed 
at the beginning of this guide. Remember that the encrypted JWT is stored inside the pod under `/var/run/secrets/kubernetes.io/serviceaccount/token`.

An unauthenticated request should fail:
```shell
kubectl exec test-pod -- bash -c 'curl -sv http://gateway-proxy-v2.gloo-system/api/pets/1'
```
{{< highlight shell "hl_lines=6 12" >}}
> GET /api/pets/1 HTTP/1.1
> Host: gateway-proxy-v2.gloo-system
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
kubectl exec test-pod -- bash -c 'curl -sv http://gateway-proxy-v2.gloo-system/api/pets/1 \
    -H "Authorization: Bearer $(cat /var/run/secrets/kubernetes.io/serviceaccount/token)"'
```
{{< highlight shell "hl_lines=7" >}}
> GET /api/pets/1 HTTP/1.1
> Host: gateway-proxy-v2.gloo-system
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
kubectl exec test-pod -- bash -c 'curl -sv -X POST http://gateway-proxy-v2.gloo-system/api/pets/1 \
    -H "Authorization: Bearer $(cat /var/run/secrets/kubernetes.io/serviceaccount/token)"'
```
{{< highlight shell "hl_lines=7 13" >}}
> POST /api/pets/1 HTTP/1.1
> Host: gateway-proxy-v2.gloo-system
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
kubectl exec test-pod -- bash -c 'curl -sv http://gateway-proxy-v2.gloo-system/foo/ \
    -H "Authorization: Bearer $(cat /var/run/secrets/kubernetes.io/serviceaccount/token)"'
```
{{< highlight shell "hl_lines=7 13" >}}
> GET /foo/ HTTP/1.1
> Host: gateway-proxy-v2.gloo-system
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

## Cleanup
You can clean up the resources created in this guide by running:

```shell
kubectl delete pod test-pod
kubectl delete virtualservice -n gloo-system petstore
kubectl delete -f https://raw.githubusercontent.com/solo-io/gloo/master/example/petstore/petstore.yaml
rm public-key.pem
```

## Appendix - Use a Remote Json Web Key Set (JWKS) Server
In the previous part of the guide we saw how to configure Gloo with a public key to verify JWTs.
In this appendix we will demonstrate how to use an external Json Web Key Set (JWKS) server with Gloo. 

Using a Json Web Key Set (JWKS) server allows us to manage the verification keys independently and 
centrally. This, for example, can allow for easy key rotation.

Here's the plan:

1. Use `openssl` to create the private key we will use to sign and verify the custom JWT we will create.
1. We will use `npm` to install a conversion utility to convert the key from PEM to Json Web Key format.
1. Deploy a JWKS server to serve the key.
1. Configure Gloo to verify JWTs using the key stored in the server.
1. Create and sign a custom JWT and use it to authenticate with Gloo.

### Create the Private Key

Let's create a private key that we will used to sign our JWT:
```shell
openssl genrsa 2048 > private-key.pem
```

{{% notice warning %}}
Storing a key on your laptop as done here is not considered secure! Do not use this workflow
for production workloads. Use appropriate secret management tools to store sensitive information.
{{% /notice %}}

### Create the Json Web Key Set (JWKS)

We can use the openssl command to extract a PEM encoded public key from the private key. We can 
then use the `pem-jwk` utility to convert our public key to a Json Web Key format.
```shell
# install pem-jwk utility.
npm install -g pem-jwk
# extract public key and convert it to JWK.
openssl rsa -in private-key.pem -pubout | pem-jwk | jq . > jwks.json
```

Output should look similar to:
```json
{
  "kty": "RSA",
  "n": "4XbzUpqbgKbDLngsLp4bpjf04WkMzXx8QsZAorkuGprIc2BYVwAmWD2tZvez4769QfXsohu85NRviYsrqbyCw_NTs3fMlcgld-ayfb_1X3-6u4f1Q8JsDm4fkSWoBUlTkWO7Mcts2hF8OJ8LlGSwzUDj3TJLQXwtfM0Ty1VzGJQMJELeBuOYHl_jaTdGogI8zbhDZ986CaIfO-q_UM5ukDA3NJ7oBQEH78N6BTsFpjDUKeTae883CCsRDbsytWgfKT8oA7C4BFkvRqVMSek7FYkg7AesknSyCIVMObSaf6ZO3T2jVGrWc0iKfrR3Oo7WpiMH84SdBYXPaS1VdLC17Q",
  "e": "AQAB"
}
```

To that, we'll add the signing algorithm and usage:
```shell script
jq '.+{alg:"RS256"}|.+{use:"sig"}' jwks.json | tee tmp.json && mv tmp.json jwks.json
```
returns
{{< highlight json "hl_lines=5-6" >}}
{
    "kty": "RSA",
    "n": "4XbzUpqbgKbDLngsLp4bpjf04WkMzXx8QsZAorkuGprIc2BYVwAmWD2tZvez4769QfXsohu85NRviYsrqbyCw_NTs3fMlcgld-ayfb_1X3-6u4f1Q8JsDm4fkSWoBUlTkWO7Mcts2hF8OJ8LlGSwzUDj3TJLQXwtfM0Ty1VzGJQMJELeBuOYHl_jaTdGogI8zbhDZ986CaIfO-q_UM5ukDA3NJ7oBQEH78N6BTsFpjDUKeTae883CCsRDbsytWgfKT8oA7C4BFkvRqVMSek7FYkg7AesknSyCIVMObSaf6ZO3T2jVGrWc0iKfrR3Oo7WpiMH84SdBYXPaS1VdLC17Q",
    "e": "AQAB",
    "alg": "RS256",
    "use": "sig"
}
{{< /highlight >}}

One last modification, is to turn the single key into a key set:
```shell script
jq '{"keys":[.]}' jwks.json | tee tmp.json && mv tmp.json jwks.json
```
returns
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

We now have a valid Json Web Key Set (JWKS), saved into a file called `jwks.json`.

### Create JWKS Server

Let's create our JWKS server. All that the server needs to do is to serve a Json Web Key Set file. 
We will configure Gloo later to grab the the Json Web Key Set from that server.

To deploy the server, we will copy our jwks file to a ConfigMap and mount it to an nginx 
container that will serve as our JWKS server:

```shell
# create a config map
kubectl -n gloo-system create configmap jwks --from-file=jwks.json=jwks.json
# deploy nginx
kubectl -n gloo-system create deployment jwks-server --image=nginx 
# mount the config map to nginx
kubectl -n gloo-system patch deployment jwks-server --type=merge -p '{"spec":{"template":{"spec":{"volumes":[{"name":"jwks-vol","configMap":{"name":"jwks"}}],"containers":[{"name":"nginx","image":"nginx","volumeMounts":[{"name":"jwks-vol","mountPath":"/usr/share/nginx/html"}]}]}}}}' -o yaml
# create a service for the nginx deployment
kubectl -n gloo-system expose deployment jwks-server --port 80
# create an upstream for gloo
glooctl create upstream kube --kube-service jwks-server --kube-service-namespace gloo-system --kube-service-port 80 -n gloo-system jwks-server
```

Configure gloo to use the JWKS server:
```shell
# remove the settings from the previous part of the guide
kubectl patch virtualservice --namespace gloo-system default --type=json -p '[{"op":"remove","path":"/spec/virtualHost/virtualHostPlugins/extensions"}]' -o yaml
# add the remote jwks
kubectl patch virtualservice --namespace gloo-system default --type=merge -p '{"spec":{"virtualHost":{"virtualHostPlugins":{"extensions":{"configs":{"jwt":{"providers":{"solo-provider":{"jwks":{"remote":{"url":"http://jwks-server/jwks.json","upstream_ref":{"name":"jwks-server","namespace":"gloo-system"}}},"issuer":"solo.io"}}}}}}}}}' -o yaml
```

### Create the Json Web Token (JWT)

We have everything we need to sign and verify a custom JWT with our custom claims.
We will use the [jwt.io](https://jwt.io) debugger to do so easily.

- Go to https://jwt.io.
- Under the "Debugger" section, change the algorithm combo-box to "RS256".
- Under the "VERIFY SIGNATURE" section, paste the contents of the file `private-key.pem` to the 
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

You should now have an encoded JWT token in the "Encoded" box. Copy it and save to to a file called 
`token.jwt`

{{% notice note %}}
 You may have noticed **jwt.io** complaining about an invalid signature in the bottom left corner. This is fine
 because we don't need the public key to create an encoded JWT.
 If you'd like to resolve the invalid signature, under the "VERIFY SIGNATURE" section, paste the output of
 `openssl rsa -pubout -in private-key.pem` to the bottom box (labeled "Public Key")
{{% /notice %}}

This is how it should look like (click to enlarge):

<img src="../jwt.io.png" alt="jwt.io debugger" style="border: dashed 2px;" width="500px"/>

That's it! time to test...

### Test

Start a proxy to the kubernetes API server.
```shell
kubectl proxy &
```

We will use kubernetes api server service proxy capabilities to reach Gloo's gateway-proxy service.
The kubernetes api server will proxy traffic going to `/api/v1/namespaces/gloo-system/services/gateway-proxy-v2:80/proxy/` to port 80 on the `gateway-proxy-v2` service, in the `gloo-system` namespace.

A request without a token should be rejected (will output *Jwt is missing*):
```shell
curl -s "localhost:8001/api/v1/namespaces/gloo-system/services/gateway-proxy-v2:80/proxy/api/pets"
```

A request with a token should be accepted:
```shell
curl -s "localhost:8001/api/v1/namespaces/gloo-system/services/gateway-proxy-v2:80/proxy/api/pets?access_token=$(cat token.jwt)"
```
### Conclusion
We have created a JWKS server, signed a custom JWT and used Gloo to verify that JWT
and authorize our request.
