---
title: Http Passthrough Auth
weight: 10
description: Authenticating using an external Http service. 
---

When using Gloo Edge's external authentication server, it may be convenient to authenticate requests with your own HTTP server.
By creating requests from the external authentication server to your own authentication component, Gloo Edge can use your authentication server
to authenticate requests.

## Setup
{{< readfile file="/static/content/setup_notes" markdown="true">}}

Let's start by creating a [Static Upstream]({{< versioned_link_path fromRoot="/guides/traffic_management/destination_types/static_upstream/" >}}) 
that routes to a website; we will send requests to it during this tutorial.

{{< tabs >}}
{{< tab name="kubectl" codelang="yaml">}}
{{< readfile file="/static/content/upstream.yaml">}}
{{< /tab >}}
{{< tab name="glooctl" codelang="shell" >}}
glooctl create upstream static --static-hosts jsonplaceholder.typicode.com:80 --name json-upstream
{{< /tab >}}
{{< /tabs >}}

### Creating an authentication service

In this example, we will be deploying a Http authentication service. 

{{< highlight shell >}}
kubectl apply -f - <<EOF
apiVersion: apps/v1
kind: Deployment
metadata:
  name: extauth-httpservice
spec:
  selector:
    matchLabels:
      app: http-extauth
  replicas: 1
  template:
    metadata:
      labels:
        app: http-extauth
    spec:
      containers:
        - name: http-extauth
          image: gcr.io/solo-public/passthrough-http-service-example
          imagePullPolicy: IfNotPresent
          ports:
            - containerPort: 9001
EOF
{{< /highlight >}}

The source code for the Http service can be found in the Gloo Edge repository [here](https://github.com/solo-io/gloo/tree/main/docs/examples/http-passthrough-auth).

Once we create the authentication service, we also want to apply the following Service to assign it a static cluster IP.
{{< highlight shell >}}
kubectl apply -f - <<EOF
apiVersion: v1
kind: Service
metadata:
  name: example-http-auth-service
  labels:
      app: http-extauth
spec:
  ports:
  - port: 9001
    protocol: TCP
  selector:
      app: http-extauth
EOF
{{< /highlight >}}

## Creating a Virtual Service
Now let's configure Gloo Edge to route requests to the upstream we just created. To do that, we define a simple Virtual 
Service to match all requests that:

- contain a `Host` header with value `foo` and
- have a path that starts with `/` (this will match all requests).

Apply the following virtual service:
{{< readfile file="guides/security/auth/extauth/basic_auth/test-no-auth-vs.yaml" markdown="true">}}

Let's send a request that matches the above route to the Gloo Edge gateway and make sure it works:

```shell
curl -H "Host: foo" $(glooctl proxy url)/posts/1
```

The above command should produce the following output:

```json
{
  "userId": 1,
  "id": 1,
  "title": "sunt aut facere repellat provident occaecati excepturi optio reprehenderit",
  "body": "quia et suscipit\nsuscipit recusandae consequuntur expedita et cum\nreprehenderit molestiae ut ut quas totam\nnostrum rerum est autem sunt rem eveniet architecto"
}
```

If you are getting a connection error, make sure you are port-forwarding the `glooctl proxy url` port to port 8080.

# Securing the Virtual Service 
As we just saw, we were able to reach the upstream without having to provide any credentials. This is because by default 
Gloo Edge allows any request on routes that do not specify authentication configuration. Let's change this behavior. 
We will update the Virtual Service so that all requests will be authenticated by our own Http auth service.

{{< highlight shell "hl_lines=9-14" >}}
kubectl apply -f - <<EOF
apiVersion: enterprise.gloo.solo.io/v1
kind: AuthConfig
metadata:
  name: passthrough-auth
  namespace: gloo-system
spec:
  configs:
  - passThroughAuth:
      http:
        # Url of the http auth server to use for auth
        url: http://example-http-auth-service.default.svc.cluster.local:9001
        # Set a connection timeout to external service, default is 5 seconds
        connectionTimeout: 3s
EOF
{{< /highlight >}}

{{% notice note %}}
Passthrough services also allow for failing "open" through the [`failureModeAllow`]({{< versioned_link_path fromRoot="/reference/api/github.com/solo-io/gloo/projects/gloo/api/v1/enterprise/options/extauth/v1/extauth.proto.sk/#settings" >}}) field. 
By setting this field to `true`, the auth service responds with an `OK` if either your server returns a `5XX` response or the request times out.
{{% /notice %}}

Once the `AuthConfig` has been created, we can use it to secure our Virtual Service:

{{< highlight shell "hl_lines=21-25" >}}
kubectl apply -f - <<EOF
apiVersion: gateway.solo.io/v1
kind: VirtualService
metadata:
  name: auth-tutorial
  namespace: gloo-system
spec:
  virtualHost:
    domains:
      - 'foo'
    routes:
      - matchers:
        - prefix: /
        routeAction:
          single:
            upstream:
              name: json-upstream
              namespace: gloo-system
        options:
          autoHostRewrite: true      
    options:
      extauth:
        configRef:
          name: passthrough-auth
          namespace: gloo-system
EOF
{{< /highlight >}}

In the above example we have added the configuration to the Virtual Host. Each route belonging to a Virtual Host will 
inherit its `AuthConfig`, unless it [overwrites or disables]({{< versioned_link_path fromRoot="/guides/security/auth/extauth/#inheritance-rules" >}}) it.

### Metrics

{{% notice note %}}
For more information on how Gloo Edge handles observability and metrics, view our [observability introduction]({{< versioned_link_path fromRoot="/introduction/observability/" >}}).
{{% /notice %}}

* Failure Mode Allow
  * Metric Name: `extauth.solo.io/http_passthrough_bypass_failure`
  * Description: The number of times a server error or timeout occurred and was bypassed through the `failure_mode_allow=true` setting


### Logging

If Gloo Edge is running on kubernetes, the extauth server logs can be viewed with:
```
kubectl logs -n gloo-system deploy/extauth -f
```
If the auth config has been received successfully, you should see the log line:
```
"logger":"extauth","caller":"runner/run.go:179","msg":"got new config"
```

## Testing the secured Virtual Service
The virtual service that we have created should now be secured using our external authentication service. To test this, we can try our original command, and the request should not be allowed through because of missing authentication.

```shell
curl -v -H "Host: foo" $(glooctl proxy url)/posts/1
```

In fact, if we check the logs of our sample http auth service, we see the following message:

```text
Did not reach the right path, use the /auth to authenticate requests! Make sure to include the header 'authorization: authorize me' too.
```

To comply with what our sample auth service needs, we can modify the auth service as follows:

{{< highlight shell "hl_lines=9-18" >}}
kubectl apply -f - <<EOF
apiVersion: enterprise.gloo.solo.io/v1
kind: AuthConfig
metadata:
  name: passthrough-auth
  namespace: gloo-system
spec:
  configs:
  - passThroughAuth:
      http:
        # Url of the http auth server to use for auth
        url: http://example-http-auth-service.default.svc.cluster.local:9001/auth
        # Set a connection timeout to external service, default is 5 seconds
        connectionTimeout: 3s
        request:
          allowedHeaders:
            - authorization
EOF
{{< /highlight >}}


The sample Http authentication service has been implemented such that any request with the header `authorization: authorize me` to path `/auth` will be authorized. We configured our
passthrough auth to use the `/auth` path with the http auth server and to passthrough `Authorization` header. We can now add this header to our curl request as follows:

```shell
curl -H "Host: foo" -H "authorization: authorize me" $(glooctl proxy url)/posts/1
```

The request should now be authorized!

## Http Passthrough Auth Config Options

For more information about configuration options, see the [API docs]({{< versioned_link_path fromRoot="/reference/api/github.com/solo-io/gloo/projects/gloo/api/v1/enterprise/options/extauth/v1/extauth.proto.sk/#passthroughhttp" >}}).

```yaml
apiVersion: enterprise.gloo.solo.io/v1
kind: AuthConfig
metadata:
  name: passthrough-auth
  namespace: gloo-system
spec:
  configs:
  - passThroughAuth:
      http:
        # Url of the http auth server to use for auth
        # This can include path, and can also use https. 
        # In order to use a https passthrough server, provide the cert in the HTTPS_PASSTHROUGH_CA_CERT environment variable to the ext-auth-service pod as a base64-encoded string.
        url: http://example-http-auth-service.default.svc.cluster.local:9001
        # Set a connection timeout to external service, default is 5 seconds
        connectionTimeout: 3s
        # These options will modify the request going to the passthrough auth server
        request:
          # These headers will be copied from the downstream request to the auth server request
          allowedHeaders: string[]
          # These headers will be added to the auth server request, and will overwrite any same headers from `allowedHeaders`.
          headersToAdd: map<string, string>
          
          # The following three "pass-through" options will use the HTTP body to passthrough relevant request components to the auth server.
          # When any of these are set, the body will be a json with the following shape, where the keys will only be included if the appropriate
          # passthrough config is set to true:
          # {
          #  "state": object
          #   "filterMetadata": object
          #   "body": string
          # }
          # If all of the passthrough options are unset or false, the body of the http auth request will be empty.
          
          # Setting this to true will include the ext-auth "state" in the request body
          passThroughState: bool
          # Setting this to true will include the envoy "filter metadata" in the request body.
          passThroughFilterMetadata: bool
          # Setting this to true will include the original http request's body in the body of the auth request.
          # In order for the body to be passed through to the auth service, the settings.extauth.requestBody must be set in the Gloo Edge Settings CRD so that
          # the request body is buffered and sent to the ext-auth service.
          passThroughBody: bool
        # These options will modify the original request to the upstream if the auth request is authorized
        # or the denied response back to the downstream client if the auth request is not authorized
        response: 
          # This is a list of headers that will be included from the authorization response and sent along with the original upstream request headers.
          # If any of these headers already exist in the original request, the auth server headers will be appended to the original headers
          allowedUpstreamHeaders: string[]
          # This is a list of headers that will be included from the authorization response and sent back to the client in the http resposne when the auth request is denied.
          allowedClientHeadersOnDenied: string[]
          # Setting this to true will allow the http auth server to modify the state by sending back a state object in the http response.
          # The state object should have the JSON shape:
          # { "state": map[string]object }.
          # If the state fails to be set for any reason, the auth request will be denied and an error will be logged in the ext-auth-service pod. 
          readStateFromResponse: bool
          # When this is set, authorization response headers that have a header in this list will be added to the original client request and sent to the upstream
          # when the auth request is successful. These will overwrite to any request headers that already exist.
          # If this and allowed_upstream_headers are empty, by default, no authorization response headers will be added to the upstream request.
          # Header names may not be included in both allowed_upstream_headers and allowed_upstream_headers_to_overwrite.
          allowedClientHeadersOnDenied: string[]
```



