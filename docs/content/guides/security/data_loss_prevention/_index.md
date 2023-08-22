---
title: Data Loss Prevention
weight: 50
description: Data Loss Prevention (DLP) is a method of ensuring that sensitive data isn't logged or leaked.
---

### Understanding DLP

Data Loss Prevention (DLP) is a method of ensuring that sensitive data isn't logged or leaked. This is done by doing
a series of regex replacements on the response body and content that is logged by Envoy ([see Access Logging]({{% versioned_link_path fromRoot="/guides/security/access_logging/" %}})).

{{% notice info %}}
Valid regex patterns are those in the [RE2 syntax](https://github.com/google/re2/wiki/Syntax). Note that some features such as lookaheads are not supported by RE2.
{{% /notice %}}

For example, we can use Gloo Edge to transform this response:
```json
{
   "fakevisa": "4397945340344828",
   "ssn": "123-45-6789"
}
```

into this response:

```json
{
   "fakevisa": "XXXXXXXXXXXX4828",
   "ssn": "XXX-XX-X789"
}
```

DLP is configured as an ordered list of `Action`s on an HTTP listener, virtual service, or route. If
configured on the listener, an additional matcher is paired with a list of `Action`s, and the first DLP rule that
matches a request will be applied.

The DLP filter will be run by Envoy after any other filters which might add data to be masked into the dynamic metadata. Gloo Edge's current filter order follows:

1. Fault Stage (Fault injection)
1. DLP
1. CORS
1. Rest of the filters ... (not all in the same stage)

### DLP for access logs

By default, DLP will only run regex replacements on the response body. If 
[access logging]({{% versioned_link_path fromRoot="/guides/security/access_logging/" %}}) is configured, the DLP actions
can also be applied to the headers and dynamic metadata that is logged by the configured access loggers. To do so, the `enabledFor`
DLP configuration option must be set to `ACCESS_LOGS` or `ALL` (to mask access logs AND the response bodies).

{{% notice info %}}
WAF access logs will only be masked when logged to Dynamic metadata. WAF logs written to Filter State will not be masked.
{{% /notice %}}

### Prerequisites

Install Gloo Edge Enterprise.

### Simple Example

In this example we will demonstrate masking responses using one of the predefined DLP Actions, rather than providing
a custom regex.

First let's begin by configuring a simple static upstream to an echo site.

{{< tabs >}}
{{< tab name="kubectl" codelang="yaml">}}
kubectl apply -f - <<EOF
{{< readfile file="guides/security/data_loss_prevention/us-echo-test.yaml">}}
EOF
{{< /tab >}}
{{< tab name="glooctl" codelang="shell script">}}
glooctl create upstream static --static-hosts echo.jsontest.com:80 --name json-upstream
{{< /tab >}}
{{< /tabs >}}

Now let's configure a simple virtual service to send requests to the upstream.
```yaml
{{< readfile file="guides/security/data_loss_prevention/vs-json-upstream.yaml">}}
```

Run the following `curl` to get the unmasked response:
```shell script
curl $(glooctl proxy url)/ssn/123-45-6789/fakevisa/4397945340344828
```

The `curl` should return:
```json
{
   "fakevisa": "4397945340344828",
   "ssn": "123-45-6789"
}
```

Now let's mask the SSN and credit card, apply the following virtual service:

{{< highlight yaml "hl_lines=19-23" >}}
kubectl apply -f - <<EOF
apiVersion: gateway.solo.io/v1
kind: VirtualService
metadata:
  name: vs
  namespace: gloo-system
spec:
  virtualHost:
    domains:
    - '*'
    routes:
    - routeAction:
        single:
          upstream:
            name: json-upstream
            namespace: gloo-system
      options:
        autoHostRewrite: true
    options:
      dlp:
        actions:
        - actionType: SSN
        - actionType: ALL_CREDIT_CARDS
EOF
{{< /highlight >}}

Run the same `curl` as before:
```shell script
curl $(glooctl proxy url)/ssn/123-45-6789/fakevisa/4397945340344828
```

This time it will return a masked response:
```json
{
   "fakevisa": "XXXXXXXXXXXX4828",
   "ssn": "XXX-XX-X789"
}
```

As noted above, DLP can also mask [access logs]({{% versioned_link_path fromRoot="/guides/security/access_logging/" %}})
by using a configuration like:

```yaml
    options:
      dlp:
        enabledFor: ALL
        actions:
        - actionType: SSN
        - actionType: ALL_CREDIT_CARDS
```

### Custom Example

In this example we will demonstrate defining our own custom DLP Action, rather than leveraging one of
the predefined regular expressions.

Let's start by creating our typical petstore microservice:

```shell
kubectl apply -f https://raw.githubusercontent.com/solo-io/gloo/v1.14.x/example/petstore/petstore.yaml
```

Apply the following virtual service to route to the Gloo Edge discovered petstore upstream:

```yaml
kubectl apply -f - <<EOF
apiVersion: gateway.solo.io/v1
kind: VirtualService
metadata:
  name: vs
  namespace: gloo-system
spec:
  virtualHost:
    domains:
    - '*'
    routes:
    - routeAction:
        single:
          upstream:
            name: default-petstore-8080
            namespace: gloo-system
EOF
```

Query the petstore microservice for a list of pets:

```shell
curl $(glooctl proxy url)/api/pets
```

You should obtain the following response:

```json
[{"id":1,"name":"Dog","status":"available"},{"id":2,"name":"Cat","status":"pending"}]
```

Names are often used as personally identifying information, or **PII**. Let's write our own regex to mask the
names returned by the petstore service:

{{< highlight yaml "hl_lines=17-27" >}}
kubectl apply -f - <<EOF
apiVersion: gateway.solo.io/v1
kind: VirtualService
metadata:
  name: vs
  namespace: gloo-system
spec:
  virtualHost:
    domains:
    - '*'
    routes:
    - routeAction:
        single:
          upstream:
            name: default-petstore-8080
            namespace: gloo-system
    options:
      dlp:
        actions:
        - customAction:
            maskChar: "X"
            name: test   # only used for logging
            percent:
              value: 60  # % of regex match to mask
            regexActions:
            - regex: '"name":[^"]*"([^"]*)"'
              subgroup: 1
EOF
{{< /highlight >}}

Query for pets again:

```shell script
curl $(glooctl proxy url)/api/pets
```

You should get a masked response:

```json
[{"id":1,"name":"XXg","status":"available"},{"id":2,"name":"XXt","status":"pending"}]
```

### Key/value (header masking) example

In this example, you define a key/value DLP action, which you can use to mask the value associated with a specified request header.

{{% notice info %}}
Predefined and Custom actions will only match based on header value in access logs. To match against a header name, use a key/value action.
{{% /notice %}}


1. Get started by creating the petstore microservice.
   ```shell
   kubectl apply -f https://raw.githubusercontent.com/solo-io/gloo/v1.14.x/example/petstore/petstore.yaml
   ```

2. Apply the following virtual service to route to the Gloo Edge discovered upstream for petstore.
   ```yaml
   apiVersion: gateway.solo.io/v1
   kind: VirtualService
   metadata:
     name: vs
     namespace: gloo-system
   spec:
     virtualHost:
       domains:
       - '*'
       routes:
       - routeAction:
           single:
             upstream:
               name: default-petstore-8080
               namespace: gloo-system
   ```

3. Apply the following gateway definition, which configures the `gateway-proxy` deployment to log the value of the `test-header` request header.
   ```yaml
   apiVersion: gateway.solo.io/v1
   kind: Gateway
   metadata:
     name: gateway-proxy
     namespace: gloo-system
   spec:
     bindAddress: '::'
     bindPort: 8080
     proxyNames:
     - gateway-proxy
     useProxyProto: false
     options:
       accessLoggingService:
         accessLog:
         - fileSink:
             stringFormat: "test-header: %REQ(test-header)%\n"
             path: /dev/stdout
   ```

4. Query the petstore microservice. The `test-value` value is specified for the `test-header` request header.
   ```shell
   curl $(glooctl proxy url)/api/pets -H test-header:test-value
   ```

5. Get the access logs from the gateway-proxy deployment.
   ```shell
   kubectl logs deployment/gateway-proxy -n gloo-system
   ```
   Verify that you see the following log entry:
   ```
   test-header: test-value 
   ```

6. To mask the value of the `test-header` request header, update the virtual service that you created in step 2 to use a DLP key/value matcher.
   {{< highlight shell "hl_lines=16-26" >}}
   apiVersion: gateway.solo.io/v1
   kind: VirtualService
   metadata:
     name: vs
     namespace: gloo-system
   spec:
     virtualHost:
       domains:
       - '*'
       routes:
       - routeAction:
           single:
             upstream:
               name: default-petstore-8080
               namespace: gloo-system
       options:
         dlp:
           enabledFor: ALL
           actions:
           - keyValueAction:
               maskChar: "*"
               name: test-header-mask   # only used for logging
               keyToMask: test-header
               percent:
                 value: 100  # % of regex match to mask
             actionType: KEYVALUE
   {{< /highlight >}}

7. Send another request to the petstore service.
   ```shell
   curl $(glooctl proxy url)/api/pets -H test-header:test-value
   ```

8. Check the gateway-proxy access logs again.
   ```shell
   kubectl logs deployment/gateway-proxy -n gloo-system
   ```
   Verify that you see the following log entry, in which the value is masked:
   ```
   test-header: ****-*****
   ```

Some notes on key/value actions:
 - You cannot use key/value actions to mask pseudo headers.
 - Key/value actions do not mask data in response bodies. They mask only the value of request headers, response headers, and dynamic metadata in access logs.
 - To apply key/value actions, you must set `dlp.enabledFor` to `ALL` or `ACCESS_LOGS`.

### Summary

In this tutorial we installed Gloo Edge Enterprise and demonstrated rewriting responses from upstreams
with both the provided default regex patterns as well as the custom regex config.

### Cleanup

```shell script
kubectl delete vs vs -n gloo-system
kubectl delete us json-upstream -n gloo-system
```
