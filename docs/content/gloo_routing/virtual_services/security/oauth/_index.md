---
title: OAuth
weight: 20
description: External Auth with Oauth
---

## OAuth

- Prior to creating an OAuth config, you must create a client secret. This can be done through `glooctl create secret --namespace gloo-system --name google oauth --client-secret $CLIENT_SECRET`

The values required for configuring OAuth are as follows:

```yaml
extauth:
  oauth:
    app_url: myapp.com
    callback_path: /my/callback/path/
    client_id: myclientid
    client_secret_ref:
      name: myoauthsecret
      namespace: gloo-system
    issuer_url: theissuer.com
```

- See below for how to create a virtual service with OAuth configured.

## Create a new virtual service with authorization enabled

The minimum required configuration in order to create a new virtual service with authorization is shown below.

```yaml
apiVersion: gateway.solo.io/v1
kind: VirtualService
metadata:
  name: myvs
  namespace: gloo-system
spec:
  virtualHost:
    domains:
    - example.com
    virtualHostPlugins:
      extensions:
        configs:
          extauth:
            oauth:
              app_url: myapp.com
              callback_path: /my/callback/path/
              client_id: myclientid
              client_secret_ref:
                name: myoauthsecret
                namespace: gloo-system
              issuer_url: theissuer.com
```

- run `kubectl apply -f <filename>` to create this virtualservice

## Edit the authorization config on an existing virtual service

Print your virtual service specification with:

```shell
kubectl get virtualservice -n <namespace> <virtual_service_name> -o yaml -f <filename>
```

In our example above, we expect to see something like:

```yaml
apiVersion: gateway.solo.io/v1
kind: VirtualService
metadata:
  annotations:
    kubectl.kubernetes.io/last-applied-configuration: |
      {"apiVersion":"gateway.solo.io/v1","kind":"VirtualService","metadata":{"annotations":{},"name":"myvs","namespace":"gloo-system"},"spec":{"virtualHost":{"domains":["example.com"],"name":"gloo-system.myvs","virtualHostPlugins":{"extensions":{"configs":{"extauth":{"oauth":{"app_url":"myapp.com","callback_path":"/my/callback/path/","client_id":"myclientid","client_secret_ref":{"name":"myoauthsecret","namespace":"gloo-system"},"issuer_url":"theissuer.com"}}}}}}}}
  creationTimestamp: 2019-02-11T22:07:13Z
  generation: 1
  name: myvs
  namespace: gloo-system
  resourceVersion: "25119"
  selfLink: /apis/gateway.solo.io/v1/namespaces/gloo-system/virtualservices/myvs
  uid: 6270f853-2e49-11e9-b196-0800271c7f63
spec:
  virtualHost:
    domains:
    - example.com
    name: gloo-system.myvs
    virtualHostPlugins:
      extensions:
        configs:
          extauth:
            oauth:
              app_url: myapp.com
              callback_path: /my/callback/path/
              client_id: myclientid
              client_secret_ref:
                name: myoauthsecret
                namespace: gloo-system
              issuer_url: theissuer.com
status:
  reported_by: gateway
  state: 1
  subresource_statuses:
    '*v1.Proxy gloo-system gateway-proxy':
      reported_by: gloo
      state: 1
```

- edit the `extauth` portion of the spec as you desire
- run `kubectl apply -f <filename>` to update the virtualservice
