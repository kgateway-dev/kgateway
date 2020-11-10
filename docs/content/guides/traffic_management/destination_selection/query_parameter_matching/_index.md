---
title: Query Parameter Matching
weight: 30
description: Request to route matching based on query parameters
---

The route rules in a *Virtual Service* can use query parameter matching rules to match requests to routes based on the values stored in parameters submitted with the request. When configuring the matcher on a route, you may want to specify one or more {{< protobuf name="gloo.solo.io.QueryParameterMatcher">}} to require query parameters with matching values be present on the request. Each query parameter matcher has three attributes:

* `name` - the name of the query parameter
* `regex` - boolean (true|false) defaults to `false`. Indicates how to interpret the `value` attribute:
  * `false` (default) - will match if `value` exactly matches query parameter value
  * `true` - treat `value` field as a regular expression
* `value`
  * If no value is specified, then the presence of the query parameter in the request with any value will match
  * If present, the `value` field will be interpreted based on the value of `regex` field

---

## Setup

{{< readfile file="/static/content/setup_notes" markdown="true">}}

If you have not yet deployed Gloo Edge, you can start by following the directions contained within the guide [Installing Gloo Edge on Kubernetes]({{% versioned_link_path fromRoot="/installation/gateway/kubernetes/" %}}).

This guide also assumes that you are running Gloo Edge in a Kubernetes cluster. Each example can be adapted to alternative deployments, such as using the HashiCorp stack of Nomad, Consul, and Vault.

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

The site referenced in the Upstream is JSONPlaceholder - an online REST API for testing and prototyping. 

---

## Query parameter matching

Now let's create a Virtual Service with a query parameter match. For simplicity, we'll set the path matcher to prefix on `/` to match all request paths.

<video controls loop>
  <source src="https://solo-docs.s3.us-east-2.amazonaws.com/gloo/videos/querymatch_createvs.mp4" type="video/mp4">
</video>
                                                                 
{{< tabs >}}
{{< tab name="kubectl" codelang="yaml">}}
{{< readfile file="guides/traffic_management/destination_selection/query_parameter_matching/virtual-service.yaml">}}
{{< /tab >}}
{{< /tabs >}}

We've created a virtual service that will match if the request contains a query param called `param1` with an exact value of `value1`. The request must also have a query parameter `param2` with any value, and `param3` set to a single lowercase letter. 

To test we can run several curl commands with different parameter combinations. Note that the URL must have quotes around it for curl to accept query parameters. 

### Correct parameters

In the first request, we will set the parameters to the expected values in our Virtual Service. The response will be a list of blog posts from the Upstream.

<video controls loop>
  <source src="https://solo-docs.s3.us-east-2.amazonaws.com/gloo/videos/querymatch_test1.mp4" type="video/mp4">
</video>

```shell
curl -v -H "Host: foo" "$(glooctl proxy url)/posts?param1=value1&param2=value2&param3=v"
```

### Testing the first parameter

For our next request, we will set an incorrect value for query `param1`. The response will be a 404 from the Virtual Service since it has no valid route for the request.

<video controls loop>
  <source src="https://solo-docs.s3.us-east-2.amazonaws.com/gloo/videos/querymatch_test2.mp4" type="video/mp4">
</video>

```shell
curl -v -H "Host: foo" "$(glooctl proxy url)/posts?param1=othervalue&param2=value2&param3=v"
```

### Testing the second parameter

The second parameter (`param2`) does not have a required value specified. If we set a different value for query `param2`, the response will be successfully sourced from the Upstream.

<video controls loop>
  <source src="https://solo-docs.s3.us-east-2.amazonaws.com/gloo/videos/querymatch_test3.mp4" type="video/mp4">
</video>

```shell
curl -v -H "Host: foo" "$(glooctl proxy url)/posts?param1=value1&param2=othervalue&param3=v"
```

### Testing the third parameter

The third parameter (`param3`) is expecting a single lower case letter. If we set it to more than one character, the request will return a 404 response.

<video controls loop>
  <source src="https://solo-docs.s3.us-east-2.amazonaws.com/gloo/videos/querymatch_test4.mp4" type="video/mp4">
</video>

```shell
curl -v -H "Host: foo" "$(glooctl proxy url)/posts?param1=value1&param2=value2&param3=vv"
```

## Summary

In this tutorial, we created a static Upstream and Virtual Service that utilized query parameter matching. We saw how the route rules matched on an exact value, any value, and a regex. 

Let's cleanup the Virtual Service and Upstream we used.

<video controls loop>
  <source src="https://solo-docs.s3.us-east-2.amazonaws.com/gloo/videos/querymatch_delete.mp4" type="video/mp4">
</video>

{{< tabs >}}
{{< tab name="kubectl" codelang="yaml">}}
kubectl delete vs -n gloo-system test-query-parameter
kubectl delete upstream -n gloo-system json-upstream
{{< /tab >}}
{{< tab name="glooctl" codelang="shell" >}}
glooctl delete vs test-query-parameter
glooctl delete upstream json-upstream
{{< /tab >}}
{{< /tabs >}}


### Next Steps

Query parameter matching rules are not the only rules available for routing decisions. We recommend checking out any of the following guides next:

* [Path Matching]({{< versioned_link_path fromRoot="/guides/traffic_management/destination_selection/path_matching/" >}})
* [Header Matching]({{< versioned_link_path fromRoot="/guides/traffic_management/destination_selection/header_matching/" >}})
* [HTTP Method Matching]({{< versioned_link_path fromRoot="/guides/traffic_management/destination_selection/http_method_matching/" >}})

