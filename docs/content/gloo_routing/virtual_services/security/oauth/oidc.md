---
title: OIDC
weight: 10
description: How to setup authentication OpenID Connect (OIDC) identity providers. 
---

## Authentication
Gloo supports authentication with external OpenID Connect (OIDC) identity providers.
In this document we will show how to setup google account login for your application via gloo.
This document is here to get you started and does not cover everything you need to (like setting up 
a domain and SSL certificates)

## Setup
Gloo supports authentication via Envoy's external auth feature. First, make sure that gloo is 
configured, with the auth add-on:

```shell
kubectl get settings -n gloo-system default -o yaml
```

It should look like this:

```yaml
apiVersion: gloo.solo.io/v1
kind: Settings
metadata:
  name: default
  namespace: gloo-system
spec:
  bindAddr: 0.0.0.0:9977
  devMode: true
  discoveryNamespace: gloo-system
  extensions:
    configs:
      extauth:
        extauthzServerRef:
          name: extauth
          namespace: gloo-system
  kubernetesArtifactSource: {}
  kubernetesConfigSource: {}
  kubernetesSecretSource: {}
  refreshRate: 60s
```

If it's not configured, add it with this command (this assumes that it was installed with the default settings):

```shell
glooctl edit settings --namespace gloo-system --name default  externalauth --extauth-server-namespace gloo-system --extauth-server-name extauth
```

## Create google OAuth credentials

Go to <https://console.developers.google.com/apis/credentials/consent> and write an  Application name.
Then, go to <https://console.developers.google.com/apis/credentials/> and:

- Click 'Create credentials', and then 'OAuth client ID'. 
- In this example, we will select 'Other' as the type if the client (as we are only going to use it for demonstration purposes) and click 'Create'.

For your convenience, set the client id and secret in environment variables:

```noop
CLIENT_ID=825...imq.apps.googleusercontent.com
CLIENT_SECRET=CCh...lmT
```

## Configure Gloo

### Network
Note that since we didn't register any URL with Google, for security reasons, they will only allow authentication with applications running on localhost.
We can make the gloo gateway available in localhost using `kubectl port-forward`:

```shell
kubectl port-forward -n gloo-system deploy/gateway-proxy 8080
```

### Configuration

First, let's create a secret with the client secret:

```shell
glooctl create secret --namespace gloo-system --name google oauth --client-secret $CLIENT_SECRET
```

Create a virtual service, and add the open id connect information to it:

```shell
glooctl create virtualservice --namespace gloo-system --name default --enable-oidc-auth \
--oidc-auth-client-secret-name google \
--oidc-auth-client-secret-namespace gloo-system \
--oidc-auth-issuer-url https://accounts.google.com \
--oidc-auth-client-id $CLIENT_ID \
--oidc-auth-app-url http://localhost:8080/ \
--oidc-auth-callback-path /callback
```

In addition to the Client ID and secret, the following parameters are provided:

- issuer url: The url of the OpenID Connect identity provider. OpenID Connect configuration will be 
  automatically discovered, by going to the '.well-known/open-configuration' endpoint. in our example,
  this gloo will expect to find open id discovery information in `https://accounts.google.com/.well-known/openid-configuration`
- app url: The public URL of your application. this is used in combination with the callback parameter.
- callback path: The callback path relative to the app url. In our example, the full callback url will be
  `http://localhost:8080/callback`. used in the authentication process - 
  once authenticated, the identity provider will redirect the user to this url. Gloo will process
  request coming to this path and will not forward them upstream.

Add the appropriate routes in the virtual service for your application.

Note: you can disable authentication on specific routes (this can be useful for example, for the login page itself)

That's it! Go in to <http://localhost:8080> and try it!
