---
title: JWT and Access Control
weight: 2
description: JWT verification and Access Control (without an external auth server)
---

## Prerequisites

We will use the following utilities

- minikube
- `jq` (optional)
- `tr`, python (for text transformations)
- `glooctl` enterprise version v0.13.16 or later.

## Initial setup

Start minikube:
```shell script
minikube start
```
Make sure your kubectl context is configured to use the `default` namespace:
```shell script
kubectl config set-context --current --namespace default
```

Install gloo-enterprise and create a virtual service and an example app:
```shell
glooctl install gateway enterprise --license-key <YOUR KEY>
```

Wait for the deployments to finish:
```shell
kubectl -n gloo-system rollout status deployment/discovery
kubectl -n gloo-system rollout status deployment/gateway-v2
kubectl -n gloo-system rollout status deployment/gloo
kubectl -n gloo-system rollout status deployment/gateway-proxy-v2
```

Install the petstore demo app, add a route, and test that everything so far works (you may need to wait a few moments for all the Gloo containers to be initialized):
```shell
kubectl apply -f https://raw.githubusercontent.com/sololabs/demos2/master/resources/petstore.yaml
glooctl add route --name default --namespace gloo-system --path-prefix / --dest-name default-petstore-8080 --dest-namespace gloo-system
GATEWAY_URL=$(glooctl proxy url)
```

Test that everything so far works:
```shell script
curl "$GATEWAY_URL/api/pets/"
```
returns
```json
[{"id":1,"name":"Dog","status":"available"},{"id":2,"name":"Cat","status":"pending"}]
```

## Setting up JWT authorization

Let's create a test pod, with a different service account. We will use this pod in the guide to test
access with the new service account credentials.

```shell
kubectl create serviceaccount svc-a
kubectl run --generator=run-pod/v1 test-pod --image=fedora:30 --serviceaccount=svc-a --command sleep 10h
```

### Anatomy of kuberentes service account

When kuberentes starts a pod, it automatically attaches to it a JWT (JSON Web Token), that allows 
for authentication with the credentials of the pod's service account.
Inside the JWT are *claims* that provide identity information, and a signature for verification.

To verify these JWT, the kubernetes api server is provided with a public key. We can use this public 
key to perform JWT verification for kubernetes service accounts in Gloo.

Let's see the claims for `svc-a` - the service account we just created:

```shell
CLAIMS=$(kubectl exec test-pod cat /var/run/secrets/kubernetes.io/serviceaccount/token | cut -d. -f2)
PADDING_LEN=$(( 4 - ( ${#CLAIMS} % 4 ) ))
PADDING=$(head -c $PADDING_LEN /dev/zero | tr '\0' =)
PADDED_CLAIMS="${CLAIMS}${PADDING}"
# Note: jq makes the output easier to read. It can be omitted if you do not have it installed
echo $PADDED_CLAIMS | base64 --decode | jq .
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

The most important claims for this guide are the *iss* claim and the *sub* claim. We will use these
claims later to verify the identity of the JWT.

### Configuring Gloo to verify service account JWT

To get the public key to verify service accounts, use this command:
```shell
minikube ssh sudo cat /var/lib/minikube/certs/sa.pub | tee public-key.pem
```
This command will output the public key, and will save it to a file called `public-key.pem`.
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
If the above command doesn't produce the expected output, it could be that the
`/var/lib/minikube/certs/sa.pub` is different on your minikube.
The public key is given to the kube api-server in the command line arg `--service-account-key-file`.
You can see it like so: `minikube ssh ps ax ww | grep kube-apiserver`
{{% /notice %}}

Configure JWT verification in Gloo's default virtual service:

```shell
# escape the spaces in the public key file:
PUBKEY=$(cat public-key.pem | python -c 'import json,sys; print(json.dumps(sys.stdin.read()).replace(" ", "\\u0020"))')
# patch the default virtual service
kubectl patch virtualservice --namespace gloo-system default --type=merge -p '{"spec":{"virtualHost":{"virtualHostPlugins":{"extensions":{"configs":{"jwt":{"providers":{"kube":{"jwks":{"local":{"key":'$PUBKEY'}},"issuer":"kubernetes/serviceaccount"}}}}}}}}}' -o yaml
```
The output should look like so:
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
    - matcher:
        prefix: /
      routeAction:
        single:
          upstream:
            name: default-petstore-8080
            namespace: gloo-system
    virtualHostPlugins:
      extensions:
        configs:
          jwt:
            providers:
              kube:
                issuer: kubernetes/serviceaccount
                jwks:
                  local:
                    key: "-----BEGIN PUBLIC KEY-----\r\nMIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKCAQEA4XbzUpqbgKbDLngsLp4b\r\npjf04WkMzXx8QsZAorkuGprIc2BYVwAmWD2tZvez4769QfXsohu85NRviYsrqbyC\r\nw/NTs3fMlcgld+ayfb/1X3+6u4f1Q8JsDm4fkSWoBUlTkWO7Mcts2hF8OJ8LlGSw\r\nzUDj3TJLQXwtfM0Ty1VzGJQMJELeBuOYHl/jaTdGogI8zbhDZ986CaIfO+q/UM5u\r\nkDA3NJ7oBQEH78N6BTsFpjDUKeTae883CCsRDbsytWgfKT8oA7C4BFkvRqVMSek7\r\nFYkg7AesknSyCIVMObSaf6ZO3T2jVGrWc0iKfrR3Oo7WpiMH84SdBYXPaS1VdLC1\r\n7QIDAQAB\r\n-----END
                      PUBLIC KEY-----\r\n"
```

The updated virtual service now contains JWT configuration with the public key, and the issuer for the JWT.
JWTs will be authorized if they can be verified with this public key, and have 'kubernetes/serviceaccount' in their 'iss' claim.

## Configuring Gloo to perform access control for the service account

To make this interesting, we can add an access control policy for JWT. Let's add a policy to the virtual service:
```shell
POLICIES='{
"policies": {
    "viewer": {
        "principals":[{
            "jwtPrincipal":{"claims":{"sub":"system:serviceaccount:default:svc-a"}}
        }],
        "permissions":{
            "pathPrefix":"/api/pets",
            "methods":["GET"]
        }
    }
}
}'
# remove spaces, we can use `tr` as there are no spaces in the values.
POLICIES=$(echo $POLICIES | tr -d '[:space:]')
kubectl patch virtualservice --namespace gloo-system default --type=merge -p '{"spec":{"virtualHost":{"virtualHostPlugins":{"extensions":{"configs":{"rbac":{"config":'$POLICIES'}}}}}}}' -o yaml
```

The output should look like so:
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
    - matcher:
        prefix: /
      routeAction:
        single:
          upstream:
            name: default-petstore-8080
            namespace: gloo-system
    virtualHostPlugins:
      extensions:
        configs:
          jwt:
            providers:
              kube:
                issuer: kubernetes/serviceaccount
                jwks:
                  local:
                    key: "-----BEGIN PUBLIC KEY-----\r\nMIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKCAQEA4XbzUpqbgKbDLngsLp4b\r\npjf04WkMzXx8QsZAorkuGprIc2BYVwAmWD2tZvez4769QfXsohu85NRviYsrqbyC\r\nw/NTs3fMlcgld+ayfb/1X3+6u4f1Q8JsDm4fkSWoBUlTkWO7Mcts2hF8OJ8LlGSw\r\nzUDj3TJLQXwtfM0Ty1VzGJQMJELeBuOYHl/jaTdGogI8zbhDZ986CaIfO+q/UM5u\r\nkDA3NJ7oBQEH78N6BTsFpjDUKeTae883CCsRDbsytWgfKT8oA7C4BFkvRqVMSek7\r\nFYkg7AesknSyCIVMObSaf6ZO3T2jVGrWc0iKfrR3Oo7WpiMH84SdBYXPaS1VdLC1\r\n7QIDAQAB\r\n-----END
                      PUBLIC KEY-----\r\n"
          rbac:
            config:
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
```

### Test

Let's verify that everything is working properly:

An un-authenticated request should fail (will output *Jwt is missing*):
```shell
kubectl exec test-pod -- bash -c 'curl -s http://gateway-proxy-v2.gloo-system/api/pets/1'
```

An authenticated GET request to a path that starts with `/api/pets` should succeed:
```shell
kubectl exec test-pod -- bash -c 'curl -s http://gateway-proxy-v2.gloo-system/api/pets/1 -H "Authorization: Bearer $(cat /var/run/secrets/kubernetes.io/serviceaccount/token)"'
```

An authenticated POST request to a path that starts with `/api/pets` should fail (will output *RBAC: access denied*):
```shell
kubectl exec test-pod -- bash -c 'curl -s -X POST http://gateway-proxy-v2.gloo-system/api/pets/1 -H "Authorization: Bearer $(cat /var/run/secrets/kubernetes.io/serviceaccount/token)"'
```

An authenticated GET request to a path that doesn't start with `/api/pets` should fail (will output *RBAC: access denied*):
```shell
kubectl exec test-pod -- bash -c 'curl -s http://gateway-proxy-v2.gloo-system/foo/ -H "Authorization: Bearer $(cat /var/run/secrets/kubernetes.io/serviceaccount/token)"'
```

## Conclusion

We have used Gloo to verify service account identity, and provide access control. In this guide we demonstrated using gloo as an internal API gateway, and performing access control using kubernetes service accounts.

## Cleanup

To clean up individual resources created:
```shell
kubectl delete pod test-pod
kubectl delete -f https://raw.githubusercontent.com/sololabs/demos2/master/resources/petstore.yaml
glooctl uninstall
rm public-key.pem
```

Alternatively, you can just tear down minikube:
```
minikube delete
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
