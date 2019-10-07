---
title: JWT Claim Based Routing
weight: 3
description: Perform routing decisions using information in a JWT's claims
---

In this guide we will be using JWT claims to perform canary routing. Solo.io employee's will be routed to the canary instance. All other authenticated parties will be routed to the
primary version.

## Prerequisites
You will need:

- gloo-e 0.14.0 or higher installed.
- `openssl`

## Setup

### Generate JWTs
If you would like to generate your own JWTs, create a private/public key pair.
```shell
openssl genrsa 2048 > private-key.pem
openssl rsa -in private-key.pem -pubout > public-key.pem
```
If not, you can use the keys used in this guide to follow along.
The JWTs in following parts of the doc will match these keys (click to expand):
<details><summary>Private Key</summary>
```text
-----BEGIN RSA PRIVATE KEY-----
MIIEogIBAAKCAQEAqqFBFrh4Sc0aMBrywjoaZQhFMkV7f1uhRKKyQfDG+1efhJaD
lqwi2e1zi9xJTdBrpPtcbuIg4+cAF2aF3kHsy+nE1hsxAEeQvq/Bi/ZKSHHs3MuG
XIhZpU+aISItaVRdGBNFu/mjoyvZiFMIDnUoaHAtFKAd4B6VX1JZ1Eh4Pa09wgrx
bocJVQlbdCDGFiOR8WcfPyJ6IiL2M4uxQlRNpuw881qmEhONcdl6TNitBlMAvPQ4
Dsi4Cb6DOtVyUC9ToqL3Wi86DpCnHKNb5RbPXl053VyBpv06cDz7HGeAfba7kQKj
QinmIikZCxy6VGVBJzQUNRKwljFp8Uf8Bk8eEwIDAQABAoIBAEGkH2IaPUxG9xgi
hdlqeNT9RYF9cXEhUv0QifsMIcB3iQp8zMqeFho4WwcnC5w/3eluObT+kSCbsVFP
Q5ipS+t2Vx72/vbYkTqKaq7pZNJR4YlfUqUuXy5VXTn55/ZpWhb08xLJisYvDFSB
fMvzDkR/Qxh4MIYTvesZxyz/ZCJ1biuA5GpvuTYWyv0t4ql25Ok7wSBPViJmuyFM
y8pEk1m0UlvNVsh+KFSbuFSwHdXfOR+QPjq2UCW+8cYi8xsoPhIiGagBl6BPMyc5
xJkfnrSs3kB0S5VdHO4shZXmOuSENtv2OvONjvwoNCzh0sxOtABUMqeFvNEMhopm
5gs1H4ECgYEA2TnYxNZ98BXEUoc/xyXwHCeTNbZHNdDII6jyDOPMOlWbjGyX1OPo
3WGU5Nehvn5JUC3QDivm8oMXklVBMD76Jllx/4C6X72u8yorp4Q/Hj0qDAKWP8Pp
jn2cJX4SYXjXYvBJN+LuUIdkbVnEE3qZi85qRqJTPh5mcYjsOZeQXxECgYEAyRYy
lMeYUA9NeNJOszzJRFE7vgjfQ1NLqEKhTq4NmmHEkK661IhnrxnvFBfVtor99kSO
o7P3JZ8xcevoZqP1W3t4vO96TIxa0vPrn43C25xPYJHswrPqQteF3j65rWOto5o2
+SSUJCXYH0YPbNSAHqHajAXEZheuyYxUSB4hsuMCgYBST3IM+/2SeJ0AbJFFI+H8
uR41zxDimm8L3BuDuNmNDR04s3lAyO9W23/wyqhWJ0IeaI2aoRYMtJG8+CMQZfyh
hWkF2MBGQPjG2SbbfefwzFpfXKeUF+cq//un1UKfvotWyRflXk7RIsxyBv6eJumB
qUBp7V4/foNw5+Ii3IRvEQKBgETyln9K/J+ez5p4ycFNO1lwXQKouhy0h8F2ryZy
KXngwew17RuIdbylMMN79Kw1diSllx7sSvacYfDEyZe/6hXm/RwTJKTwjwe72POJ
QOHZ86GSB1MvK0il62GrsjCQd+4bp3O/pgfK7hKzDADtz8wxBOVz6MZ0olq7Af8E
TduvAoGALciccA3OE4gsUc5clZDaT8iZUx12J0MNV87IhK5mLnF9mdT5GVohPvrA
lLwvQs17ZdSgwMmnDZV4CHCnEog0R9jBAWRpdmSF7nXRYiUavmaqwINjUIWWZmvg
xTS0qnY2ReWxStgeIgcFRovI3BJJWAolcX+qIESOSBbFr++SdfI=
-----END RSA PRIVATE KEY-----
```
</details>
<details><summary>Public Key</summary>
```text
-----BEGIN PUBLIC KEY-----
MIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKCAQEAqqFBFrh4Sc0aMBrywjoa
ZQhFMkV7f1uhRKKyQfDG+1efhJaDlqwi2e1zi9xJTdBrpPtcbuIg4+cAF2aF3kHs
y+nE1hsxAEeQvq/Bi/ZKSHHs3MuGXIhZpU+aISItaVRdGBNFu/mjoyvZiFMIDnUo
aHAtFKAd4B6VX1JZ1Eh4Pa09wgrxbocJVQlbdCDGFiOR8WcfPyJ6IiL2M4uxQlRN
puw881qmEhONcdl6TNitBlMAvPQ4Dsi4Cb6DOtVyUC9ToqL3Wi86DpCnHKNb5RbP
Xl053VyBpv06cDz7HGeAfba7kQKjQinmIikZCxy6VGVBJzQUNRKwljFp8Uf8Bk8e
EwIDAQAB
-----END PUBLIC KEY-----
```
</details>

{{% notice warning %}}
Only use above keys for testing purposes! They are publicly available and therefore not secure!
{{% /notice %}}


Similar to the [JWT and Access Control guide](../access_control/#create-the-json-web-token-jwt), using `jwt.io` we can generate two RS256 JWTs:

One for solo.io employees with the following payload:
```json
{
  "iss": "solo.io",
  "sub": "1234567890",
  "org": "solo.io"
}
```

Save the resulting token to an variable:
```shell
SOLO_TOKEN=eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9.eyJpc3MiOiJzb2xvLmlvIiwic3ViIjoiMTIzNDU2Nzg5MCIsIm9yZyI6InNvbG8uaW8ifQ.WeYtM17EBdQc5Ka9PHPseKhX96krnQSARN8dLA806FyKY2MKWzdlAQL0UYfFi1c2C8_4pW0taK2vwhmKU2zgCvLb-_5tkOXFbPzILucAUumqT079139Q34wR64xFr6jQp1hES97IYumWnHfZOaNR_fZ3q5EZkke3YrdGhHHfo1ze41w77QCV234eDi72RmSawEaKyEGevZev16iw3M7Gfk_cet05DHfn9CPFlbuc9DkU8-r2vE9nz8NP0JC77iQtZ0YFmmb3FGxrlPDmcqDte0F45rfz8TR7hve-zqCP5PJU_euBVsZ3ShlRANbCS02x8N_ocO9S8_aypkCQqKNIlw
```

And another one for othercompany.com employees with the following payload:

```json
{
  "iss": "solo.io",
  "sub": "0987654321",
  "org": "othercompany.com"
}
```

Save the resulting token to an variable:
```shell
OTHER_TOKEN=eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9.eyJpc3MiOiJzb2xvLmlvIiwic3ViIjoiMDk4NzY1NDMyMSIsIm9yZyI6Im90aGVyY29tcGFueS5jb20ifQ.ULWH8i4LINvrHull2LKSiBhlGOJmNf9OkXdjPCyHmiZGC9GEWuLzBBiBkXUalNgJ_fLpHtwml9eN3ALoU8Ni9aAq_IRW9GE_fbqpdIztgd4IYxwbMBH-O5IwN7xBy2D2ZeHR1KMePtwxLyv1env3nQ8i4ZPPrm2fhjg90CdZmqWGhJZHlmZCXZIy-gQKIptzkDMI1t0eNtLKwoOa9BwihAdchcjbCYTxqgB0sYZvfxeNbtVxDhGdYQ4kIfYUEnr-IsfpNhT2d1LlBlEn4bpw4jABqZgoVRoPrfdKWlrixWbxdEpXsIsBxaajAiQCuDpgRfoHb3JNJNgYaa_jKuT0GA
```

### Example app
We will now create an example app. This app will simulate a primary/canary deployment mode. 
We will use Hashicorp's http-echo utility to send us a predefined response for demo purposes.
To create the demo app, we will deploy a pod to simulate the primary deployment, a pod to simulate the canary deployment and a service to route to them:
```
kubectl create ns echoapp
kubectl run -n echoapp --generator=run-pod/v1 --labels stage=primary,app=echoapp primary-pod --image=hashicorp/http-echo -- -text=primary -listen=:8080

kubectl run -n echoapp --generator=run-pod/v1 --labels stage=canary,app=echoapp canary-pod --image=hashicorp/http-echo -- -text=canary -listen=:8080

kubectl create -n echoapp service clusterip echoapp --tcp=80:8080
```

{{% notice note %}}
The pods have a label named 'stage' that indicates whether they are canary or primary pods.
{{% /notice %}}

Next let's create a Gloo upstream for the kube service. We will use Gloo's subset routing and set it to use the 'stage' key to create subsets for the service:
```shell
glooctl create upstream kube --kube-service echoapp --kube-service-namespace echoapp --kube-service-port 80 -n echoapp echoapp

# add subsets:
kubectl patch upstream --namespace echoapp echoapp --type=merge -p '{"spec":{"upstreamSpec":{"kube":{"subsetSpec":{"selectors":[{"keys":["stage"]}]}}}}}' -o yaml
```

## Gloo route

We are now ready setup routes. we will setup JWTs for solo.io employees to go to the canary subset,
and JWTs from other orgs to go to the primary subset.

To do that, we will use the `claimsToHeaders` field in the JWT extension, and copy the `org` claim
to a header name `x-company`. Then we can use normal header matching to do the routing. 

Our first route matches if the `x-company` header contains the value solo.io and routes to the canary subset. If it doesn't, the second route (that routes to the primary subset) will be selected.

Let's apply the following virtual host:

```bash
kubectl apply -f - <<EOF
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
        headers:
        - name: x-company
          value: solo.io
      routeAction:
        single:
          upstream:
            name: echoapp
            namespace: echoapp
          subset:
            values:
              stage: canary
    - matcher:
        prefix: /
      routeAction:
        single:
          upstream:
            name: echoapp
            namespace: echoapp
          subset:
            values:
              stage: primary
    virtualHostPlugins:
      extensions:
        configs:
          jwt:
            providers:
              solo:
                tokenSource:
                  headers:
                  - header: x-token
                  queryParams:
                  - token
                claimsToHeaders:
                - claim: org
                  header: x-company
                issuer: solo.io
                jwks:
                  local:
                    key: |
                         -----BEGIN PUBLIC KEY-----
                         MIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKCAQEAqqFBFrh4Sc0aMBrywjoa
                         ZQhFMkV7f1uhRKKyQfDG+1efhJaDlqwi2e1zi9xJTdBrpPtcbuIg4+cAF2aF3kHs
                         y+nE1hsxAEeQvq/Bi/ZKSHHs3MuGXIhZpU+aISItaVRdGBNFu/mjoyvZiFMIDnUo
                         aHAtFKAd4B6VX1JZ1Eh4Pa09wgrxbocJVQlbdCDGFiOR8WcfPyJ6IiL2M4uxQlRN
                         puw881qmEhONcdl6TNitBlMAvPQ4Dsi4Cb6DOtVyUC9ToqL3Wi86DpCnHKNb5RbP
                         Xl053VyBpv06cDz7HGeAfba7kQKjQinmIikZCxy6VGVBJzQUNRKwljFp8Uf8Bk8e
                         EwIDAQAB
                         -----END PUBLIC KEY-----
EOF
```

{{% notice note %}}
if you generated your own private/public key pair, replace this public key with yours.
You can use the JWTs provided in this guide with the public key above.
{{% /notice %}}

For convenience, we added the `tokenSource` settings so we can pass the token as a query parameter named `token`.

## Time to test!

get the url for the proxy
```
GATEWAY_URL=$(glooctl proxy url)
```
curl as a solo.io team member:
```
curl "$GATEWAY_URL?token=$SOLO_TOKEN"
```
The output should be `canary`.

curl as a othercompany.com team member:
```
curl "$GATEWAY_URL?token=$OTHER_TOKEN"
```
The output should be `primary`.

## Conclusion

In this guide we performed routing based on JWT claims. We used this to send solo.io employees to a canary version of our app, and send others to the primary version of our app.
