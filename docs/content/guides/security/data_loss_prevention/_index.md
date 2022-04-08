---
title: Data Loss Prevention
weight: 50
description: Data Loss Prevention (DLP) is a method of ensuring that sensitive data isn't logged or leaked.
---

### Understanding DLP

Data Loss Prevention (DLP) is a method of ensuring that sensitive data isn't logged or leaked. This is done by doing
a series of regex replacements on the response body.

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

DLP is configured as a list of `Action`s, applied in order, on an HTTP listener, virtual service, or route. If
configured on the listener, an additional matcher is paired with a list of `Action`s, and the first DLP rule that
matches a request will be applied.

DLP is one of the first filters run by Envoy. Gloo Edge's current filter order follows:

1. Fault Stage (Fault injection)
1. CORS/DLP Stage (order here is not guaranteed to be idempotent)
1. WAF Stage
1. Rest of the filters ... (not all in the same stage)

### DLP for access logs

By default, DLP will only run regex replacements on the response body. If 
[access logging]({{% versioned_link_path fromRoot="/guides/security/access_logging/" %}}) is configured, the DLP actions
can also be applied to the headers and dynamic metadata that is logged by the configured access loggers. To do so, the `enabledFor`
DLP configuration option must be set to `ACCESS_LOGS` or `ALL` (to mask access logs AND the response bodies).

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
kubectl apply -f https://raw.githubusercontent.com/solo-io/gloo/v1.2.9/example/petstore/petstore.yaml
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
            regex:
            - '(?!"name":"[\s]*)[^"]+(?=",)'
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

### Summary

In this tutorial we installed Gloo Edge Enterprise and demonstrated rewriting responses from upstreams
with both the provided default regex patterns as well as the custom regex config.

### Cleanup

```shell script
kubectl delete vs vs -n gloo-system
kubectl delete us json-upstream -n gloo-system
```
