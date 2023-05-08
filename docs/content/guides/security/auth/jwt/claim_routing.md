---
title: JWT Claim Based Routing
weight: 3
description: Perform routing decisions using information in a JWT's claims
---

{{% notice note %}}
The features used here were introduced with **Gloo Edge Enterprise**, release 0.14.0. If you are using an earlier version, this tutorial will not work.
{{% /notice %}}

In this guide, we will show how to configure Gloo Edge to route requests to different services based on the claims contained 
in a JSON Web Token (JWT). In our example scenario, Solo.io employees will be routed to the canary instance instance of 
a service, while all other authenticated parties will be routed to the primary version of the same service.

## Setup
{{< readfile file="/static/content/setup_notes" markdown="true">}}

### Deploy sample application
Let's deploy a sample application which will simulate a canary deployment. We will use Hashicorp's 
[http-echo](https://github.com/hashicorp/http-echo) application, which listens for HTTP requests 
and echoes back a configurable string. We will deploy:
 
1. a pod that responds with the string "primary" to simulate the primary deployment
1. a pod that responds with the string "canary" to simulate the canary deployment
1. a service to route to them

First, let's create the primary deployment.

```
apiVersion: apps/v1
kind: Deployment
metadata:
  name: primary
  labels:
    app: echoapp
    stage: primary
spec:
  replicas: 1
  selector:
    matchLabels:
      app: echoapp
      stage: primary
  template:
    metadata:
      labels:
        app: echoapp
        stage: primary
    spec:
      containers:
      - name: primary
        image: hashicorp/http-echo
        imagePullPolicy: IfNotPresent
        args:
          - -listen=:8080
          - -text=primary
```

Next, the canary deployment.

```
apiVersion: apps/v1
kind: Deployment
metadata:
  name: canary
  labels:
    app: echoapp
    stage: canary
spec:
  replicas: 1
  selector:
    matchLabels:
      app: echoapp
      stage: canary
  template:
    metadata:
      labels:
        app: echoapp
        stage: canary
    spec:
      containers:
      - name: canary
        image: hashicorp/http-echo
        imagePullPolicy: IfNotPresent
        args:
          - -listen=:8080
          - -text=canary
```

Finally, we will create the service.

```shell
kubectl create service clusterip echoapp --tcp=80:8080
```

{{% notice note %}}
The pods have a label named **stage** that indicates whether they are canary or primary pods.
{{% /notice %}}

Next let's create a Gloo Edge upstream for the kube service. We will use Gloo Edge's subset routing feature and set it to use 
the 'stage' key to create subsets for the service:

```yaml
apiVersion: gloo.solo.io/v1
kind: Upstream
metadata:
  name: echoapp
  namespace: gloo-system
spec:
  kube:
    serviceName: echoapp
    serviceNamespace: default
    servicePort: 80
    subsetSpec:
      selectors:
      - keys:
        - stage
```

The `subsetSpec` configuration instructs Gloo Edge to partition the endpoints of the `echoapp` service into subsets based on 
the values of the `stage` label. In our case, this will result in two subsets, `primary` and `canary`.

### Generate JWTs
Let's create a private/public key pair that we will use to sign our JWTs:

```shell
openssl genrsa 2048 > private-key.pem
openssl rsa -in private-key.pem -pubout > public-key.pem
```

Please refer to the [JWT and Access Control guide]({{% versioned_link_path fromRoot="/guides/security/auth/jwt/access_control/#create-the-json-web-token-jwt" %}}) to see how to use the [jwt.io](http://jwt.io) website to create two RS256 JWTs:

- one for `solo.io` employees with the following payload:
```json
{
  "iss": "solo.io",
  "sub": "1234567890",
  "org": "solo.io"
}
```

- one for `othercompany.com` employees with the following payload:

```json
{
  "iss": "solo.io",
  "sub": "0987654321",
  "org": "othercompany.com"
}
```

Save the resulting tokens to two shell variables:
```shell
SOLO_TOKEN=<encoded token with solo.io payload from jwt.io>
OTHER_TOKEN=<encoded token with othercompany.com payload from jwt.io>
```

## Create a Virtual Service
Now let's configure our Virtual Service to route requests to one of the two subset based on the JWTs in the incoming 
requests themselves. 

{{< highlight shell "hl_lines=34-58" >}}
apiVersion: gateway.solo.io/v1
kind: VirtualService
metadata:
  name: echo
  namespace: gloo-system
spec:
  virtualHost:
    domains:
    - '*'
    routes:
    - matchers:
      - prefix: /
        headers:
        - name: x-company
          value: solo.io
      routeAction:
        single:
          upstream:
            name: echoapp
            namespace: gloo-system
          subset:
            values:
              stage: canary
    - matchers:
      - prefix: /
      routeAction:
        single:
          upstream:
            name: echoapp
            namespace: gloo-system
          subset:
            values:
              stage: primary
    options:
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
                  MIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKCAQEAtnkwSRAoyViwIuoDUiMv
                  ylnMjXgCYAAGH43IwzuCAtSyJG5Ufp5PnJ4jJJlINMwN3Wv887AYCSndoC8P83L3
                  1JPPqDgJY+MYuLmaP9vnjNN5GiDt15bG4Gby6XYFnKmjgvy2ugOP1QnTkedeVFAw
                  Y7dmSeBU1E55Xq+PxrDTKdDKHB9oiO77bfn4lWjzDECtcX+YPRbTufLJPWCNAhpF
                  41N6SdozbRenhOqgOWoHSPBsQtKir6+5NOKZjJt6amDSFYc08M7ESXZVymtCFUJ9
                  X7DtYS5ppyaW+Cyt8v5vgjrs5Cu4by//77mHWuxd918O047GhKP17l14O/DySeOF
                  7QIDAQAB
                  -----END PUBLIC KEY-----
{{< /highlight >}}

The `jwt` configuration defined on the Virtual Host instructs Gloo Edge to verify whether a JWT is present on incoming 
requests and whether the JWT is valid using the provided public key. If the JWT is valid, the `claimsToHeaders` field 
will cause Gloo Edge to copy the `org` claim to a header name `x-company`. 

At this point we can use normal header matching to do the routing:

- Our first route matches if the `x-company` header contains the value `solo.io` and routes to the canary subset. 
- If the first route does not match, we fall back to the second one, which that routes to the primary subset.

For convenience, we added the `tokenSource` settings so we can pass the token as a query parameter named `token`.

{{% notice warning %}} In order for JWT claim-based routing to work, there **must** be a fallback "catch all" route, such as the primary route above. {{% /notice %}}


### Testing our configuration
Send a request as a solo.io team member:
```
curl "$(glooctl proxy url)?token=$SOLO_TOKEN"
```
The output should be `canary`.

Send a request as a othercompany.com team member:
```
curl "$(glooctl proxy url)?token=$OTHER_TOKEN"
```
The output should be `primary`.

## Cleanup
You can clean up the resources created in this guide by running:

```shell
rm private-key.pem public-key.pem
kubectl delete virtualservice -n gloo-system echo
kubectl delete upstream -n gloo-system echoapp
kubectl delete pod primary-pod canary-pod
```
