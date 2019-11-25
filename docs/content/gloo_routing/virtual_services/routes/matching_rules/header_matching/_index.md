---
title: Header Matching
weight: 20
description: Matching based on incoming or generated headers
---

{{% notice note %}}
The `invertMatch` attribute was introduced with **Gloo**, release 0.20.9. If you are using an earlier version, the 
attribute will not be available.
{{% /notice %}}

When configuring the matcher on a route, you may want to specify one or more 
{{< protobuf name="gloo.solo.io.HeaderMatcher" display="Header Matchers">}} to require headers 
with matching values be present on the request. Each header matcher has three attributes:

* `name` - the name of the request header. Note: Gloo/Envoy use HTTP/2 so if you want to match against HTTP/1 `Host`,
use `:authority` (HTTP/2) as the name instead.
* `regex` - boolean (true|false) defaults to `false`. Indicates how to interpret the `value` attribute:
  * `false` (default) - treat `value` field as an [Envoy exact_match](https://www.envoyproxy.io/docs/envoy/latest/api-v2/api/v2/route/route.proto#envoy-api-field-route-headermatcher-exact-match)
  * `true` - treat `value` field as a regular expression as defined by [Envoy regex_match](https://www.envoyproxy.io/docs/envoy/latest/api-v2/api/v2/route/route.proto#envoy-api-field-route-headermatcher-regex-match)
* `value`
  * If no value is specified, then the presence of the header in the request with any value will match
([Envoy present_match](https://www.envoyproxy.io/docs/envoy/latest/api-v2/api/v2/route/route.proto#envoy-api-field-route-headermatcher-present-match))
  * If present, then field value interpreted based on the value of `regex` field
  * `invertMatch` - inverts the matching logic. A request matches if it does **not** match the above criteria.

## Setup

{{< readfile file="/static/content/setup_notes" markdown="true">}}

Let's create a simple upstream for testing called `json-upstream`, that routes to a static site:

{{< tabs >}}
{{< tab name="kubectl" codelang="yaml">}}
{{< readfile file="/static/content/upstream.yaml">}}
{{< /tab >}}
{{< tab name="glooctl" codelang="shell" >}}
glooctl create upstream static --static-hosts jsonplaceholder.typicode.com:80 --name json-upstream
{{< /tab >}}
{{< /tabs >}}

## Example

Let's create a virtual service with several header match rules. For simplicity, we'll set the path matcher to prefix on `/` to match all request paths: 

{{< tabs >}}
{{< tab name="kubectl" codelang="yaml">}}
{{< readfile file="gloo_routing/virtual_services/routes/matching_rules/header_matching/virtual-service.yaml">}}
{{< /tab >}}
{{< tab name="glooctl" codelang="shell">}}
glooctl create vs --name test-header --namespace gloo-system --domains foo 
glooctl add route --name test-header --header "header1=value1" --header "header2=" --header "header3=[a-z]{1}" --path-prefix / --dest-name json-upstream
{{< /tab >}}
{{< /tabs >}}

We can now make a curl request to the new virtual service and set valid values for each header. In this case, 

- `header1` must be present and equal value1, 
- `header2` must be present and can equal any value, 
- `header3` must be present and can equal any value that is a single lowercase letter, and
- `header4` must not be present
- `header5` if present, must not equal `value5`

Let's send a request that satisfies all these criteria:

```shell
curl -v -H "Host: foo" -H "header1: value1" -H "header2: value2" -H "header3: v"  $(glooctl proxy url)/posts
```

This returns a `json` list of posts. 

If we use an incorrect value for `header1`, we'll see a 404:

```shell
curl -v -H "Host: foo" -H "header1: othervalue" -H "header2: value2" -H "header3: v"  $(glooctl proxy url)/posts
```

If we use a different value for `header2`, we'll see all the posts:
```shell
curl -v -H "Host: foo" -H "header1: value1" -H "header2: othervalue" -H "header3: v"  $(glooctl proxy url)/posts
```

If we use an invalid value for `header3`, we'll get a 404: 
```shell
curl -v -H "Host: foo" -H "header1: value1" -H "header2: value2" -H "header3: value3"  $(glooctl proxy url)/posts
```

The `invertMatch` attribute in the last entry causes request to be matched only if it does **not** include a header named 
`header4`. If we send a request with that header, we'll get a 404 response:
```shell
curl -v -H "Host: foo" -H "header1: value1" -H "header2: value2" -H "header3: v" -H "header4: value4"  $(glooctl proxy url)/posts
```

The `invertMatch` attribute can be combined with value match specifications. Where `header4` had no value spec, the inversion invalidated all possible values.
The match constraints on `header5`, on the other hand, mean that if `header5` is present, the request will only match if the value is not equal to ``value5``.
If we send a request with that value, we'll get a 404 response:
```shell
curl -v -H "Host: foo" -H "header1: value1" -H "header2: value2" -H "header3: v" -H "header5: value5"  $(glooctl proxy url)/posts
```

## Summary

In this example, we added header matchers to a virtual service route. We used exact match and regex matchers for a header value, and 
also showed how to match on a header without any specific value. 

Let's cleanup the virtual service and upstream we used:

{{< tabs >}}
{{< tab name="kubectl" codelang="yaml">}}
kubectl delete vs -n gloo-system test-header
kubectl delete upstream -n gloo-system json-upstream
{{< /tab >}}
{{< tab name="glooctl" codelang="shell" >}}
glooctl delete vs test-header
glooctl delete upstream json-upstream
{{< /tab >}}
{{< /tabs >}}

<br /> 
<br />

