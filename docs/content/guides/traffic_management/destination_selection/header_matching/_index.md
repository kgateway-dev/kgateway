---
title: Header Matching
weight: 20
description: Matching based on incoming or generated headers
---

The route rules in a *Virtual Service* can use header matching rules to match requests to routes based on the contents of the headers. When configuring the matcher on a route, you may want to specify one or more {{< protobuf name="gloo.solo.io.HeaderMatcher" display="Header Matchers">}} to require headers with matching values be present on the request. Each header matcher has three attributes:

* `name` - the name of the request header. Note: Gloo Edge/Envoy use HTTP/2 so if you want to match against HTTP/1 `Host`,
use `:authority` (HTTP/2) as the name instead.
* `regex` - boolean (true|false) defaults to `false`. Indicates how to interpret the `value` attribute:
  * `false` (default) - treat `value` field as an [Envoy exact_match](https://www.envoyproxy.io/docs/envoy/latest/api-v3/config/route/v3/route.proto#envoy-api-field-route-headermatcher-exact-match)
  * `true` - treat `value` field as a regular expression as defined by [Envoy regex_match](https://www.envoyproxy.io/docs/envoy/latest/api-v3/config/route/v3/route.proto#envoy-api-field-route-headermatcher-regex-match)
* `value`
  * If no value is specified, then the presence of the header in the request with any value will match
([Envoy present_match](https://www.envoyproxy.io/docs/envoy/latest/api-v3/config/route/v3/route.proto#envoy-api-field-route-headermatcher-present-match))
  * If present, then field value interpreted based on the value of `regex` field
  * `invertMatch` - inverts the matching logic. A request matches if it does **not** match the above criteria. Note that Gloo Edge 0.20.9 or later is required to use this setting.

{{% notice note %}}
When you use header matchers, you **must also specify a prefix matcher**. Note that the path you define in prefix matcher cannot contain any hyphens (`-`).
{{% /notice %}}

In this guide, we're going to take a closer look at an example Virtual Service that uses multiple headers to match requests.  We'll begin by creating an *Upstream* and then creating the Virtual Service to route requests to that Upstream based on the headers submitted as part of the request.

---

## Setup

If you have not yet deployed Gloo Edge, you can start by following the directions contained within the guide [Installing Gloo Edge on Kubernetes]({{% versioned_link_path fromRoot="/installation/gateway/kubernetes/" %}}).

This guide also assumes that you are running Gloo Edge in a Kubernetes cluster. Each example can be adapted to alternative deployments, such as using the HashiCorp stack of Nomad, Consul, and Vault.

{{< readfile file="/static/content/setup_notes" markdown="true">}}

---

## Create an Upstream

First we are going to create a simple Upstream for testing called `json-upstream`, that routes to a static site.

<video controls loop>
  <source src="https://solo-docs.s3.us-east-2.amazonaws.com/gloo/videos/pathmatch_createupstream.mp4" type="video/mp4">
</video>

{{< tabs >}}
{{< tab name="kubectl" codelang="yaml">}}
{{< readfile file="/static/content/upstream.yaml">}}
{{< /tab >}}
{{< tab name="glooctl" codelang="shell" >}}
glooctl create upstream static --static-hosts jsonplaceholder.typicode.com:80 --name json-upstream
{{< /tab >}}
{{< /tabs >}}

---

## Create a Virtual Service

Let's create a Virtual Service with several header match rules. For simplicity, we'll set the path matcher to prefix on `/` to match all request paths. Note that when you use header matchers, you **must also specify a prefix matcher**, and the path you define in prefix matcher cannot contain any hyphens (`-`).


<video controls loop>
  <source src="https://solo-docs.s3.us-east-2.amazonaws.com/gloo/videos/headermatch_createvs.mp4" type="video/mp4">
</video>

{{< tabs >}}
{{< tab name="kubectl" codelang="yaml">}}
{{< readfile file="guides/traffic_management/destination_selection/header_matching/virtual-service.yaml">}}
{{< /tab >}}
{{< /tabs >}}

We can now make a curl request to the new Virtual Service and set valid values for each header. In this case, 

- `header1` must be present and equal **value1**, 
- `header2` must be present and can equal any value, 
- `header3` must be present and can equal any value that is a single lowercase letter
- `header4` must not be present
- `header5` must be present and must **not** equal `value5`

Let's send a request that satisfies all these criteria.

<video controls loop>
  <source src="https://solo-docs.s3.us-east-2.amazonaws.com/gloo/videos/headermatch_test1.mp4" type="video/mp4">
</video>

```shell
curl -v -H "Host: foo" -H "header1: value1" -H "header2: value2" -H "header3: v" \
  -H "header5: x" $(glooctl proxy url)/posts
```

This returns a `json` list of posts. 

If we use an incorrect value for `header1`, we'll see a 404.

<video controls loop>
  <source src="https://solo-docs.s3.us-east-2.amazonaws.com/gloo/videos/headermatch_test2.mp4" type="video/mp4">
</video>

```shell
curl -v -H "Host: foo" -H "header1: othervalue" -H "header2: value2" -H "header3: v"  \
  -H "header5: x" $(glooctl proxy url)/posts
```

If we use a different value for `header2`, we'll see all the posts.

<video controls loop>
  <source src="https://solo-docs.s3.us-east-2.amazonaws.com/gloo/videos/headermatch_test3.mp4" type="video/mp4">
</video>

```shell
curl -v -H "Host: foo" -H "header1: value1" -H "header2: othervalue" -H "header3: v"  \
  -H "header5: x" $(glooctl proxy url)/posts
```

If we use an invalid value for `header3`, we'll get a 404.

<video controls loop>
  <source src="https://solo-docs.s3.us-east-2.amazonaws.com/gloo/videos/headermatch_test4.mp4" type="video/mp4">
</video>

```shell
curl -v -H "Host: foo" -H "header1: value1" -H "header2: value2" -H "header3: value3"  \
  -H "header5: x" $(glooctl proxy url)/posts
```

The `invertMatch` attribute for `header4` causes request to be matched only if it does **not** include a header named `header4`. If we send a request with that header, we'll get a 404 response.

<video controls loop>
  <source src="https://solo-docs.s3.us-east-2.amazonaws.com/gloo/videos/headermatch_test5.mp4" type="video/mp4">
</video>

```shell
curl -v -H "Host: foo" -H "header1: value1" -H "header2: value2" -H "header3: v" \
  -H "header4: value4"  -H "header5: x" $(glooctl proxy url)/posts
```

The `invertMatch` attribute can be combined with value match specifications. Where `header4` had no value spec, the inversion invalidated all possible values. The match constraints on `header5`, on the other hand, mean that the request will only match if the value is not equal to `value5`. If we send a request with that value, we'll get a 404 response.

<video controls loop>
  <source src="https://solo-docs.s3.us-east-2.amazonaws.com/gloo/videos/headermatch_test6.mp4" type="video/mp4">
</video>

```shell
curl -v -H "Host: foo" -H "header1: value1" -H "header2: value2" -H "header3: v" \
  -H "header5: value5" $(glooctl proxy url)/posts
```

---

## Summary

In this guide, we added header matchers to a Virtual Service route. We used exact match and regex matchers for a header value, and also showed how to match on a header without any specific value. 

Let's cleanup the Virtual Service and Upstream we used.

<video controls loop>
  <source src="https://solo-docs.s3.us-east-2.amazonaws.com/gloo/videos/headermatch_delete.mp4" type="video/mp4">
</video>

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

### Next Steps

Header matching rules are not the only rules available for routing decisions. We recommend checking out any of the following guides next:

* [Path Matching]({{< versioned_link_path fromRoot="/guides/traffic_management/destination_selection/path_matching/" >}})
* [Query Parameter Matching]({{< versioned_link_path fromRoot="/guides/traffic_management/destination_selection/query_parameter_matching/" >}})
* [HTTP Method Matching]({{< versioned_link_path fromRoot="/guides/traffic_management/destination_selection/http_method_matching/" >}})

