---
title: Query Parameter Matching
weight: 30
description: Request to route matching based on query parameters
---

When configuring the matcher on a route, you may want to specify one or more 
{{< protobuf name="gloo.solo.io.QueryParameterMatcher">}}
to require query parameters with matching values be present on the request. Each query parameter matcher has three attributes:

* `name` - the name of the query parameter
* `regex` - boolean (true|false) defaults to `false`. Indicates how to interpret the `value` attribute:
  * `false` (default) - will match if `value` exactly matches query parameter value
  * `true` - treat `value` field as a regular expression
* `value`
  * If no value is specified, then the presence of the query parameter in the request with any value will match
  * If present, the `value` field will be interpreted based on the value of `regex` field

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

Now let's create a virtual service with a query parameter match. For simplicity, we'll set the path matcher to prefix on `/` to match all request paths: 
                                                                 
{{< tabs >}}
{{< tab name="kubectl" codelang="yaml">}}
{{< readfile file="gloo_routing/virtual_services/routes/matching_rules/query_parameter_matching/virtual-service.yaml">}}
{{< /tab >}}
{{< /tabs >}}

We've created a virtual service that will match if the request contains a query param called `param1` with an exact value of `value1`. 
The request must also have a query parameter `param2` with any value, and `param3` set to a single lowercase letter. 

To test we can run the following command. Note that the URL must have quotes around it for curl to accept query parameters. 

```shell
curl -v -H "Host: foo" "$(glooctl proxy url)/posts?param1=value1&param2=value2&param3=v"
```

This should return a large json response. We can set an incorrect value for query param 1, and see the curl command return a 404:

```shell
curl -v -H "Host: foo" "$(glooctl proxy url)/posts?param1=othervalue&param2=value2&param3=v"
```

If we set a different value for query param 2, the command should work:
```shell
curl -v -H "Host: foo" "$(glooctl proxy url)/posts?param1=value1&param2=othervalue&param3=v"
```

Finally, if we set an invalid value for query param 3, the command will return a 404:

```shell
curl -v -H "Host: foo" "$(glooctl proxy url)/posts?param1=value1&param2=value2&param3=vv"
```

## Summary

In this guide, we created a virtual service that utilized query parameter matching and showed how to match on an exact value, 
any value, and a regex. 

Let's cleanup the virtual service and upstream we used:

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

<br /> 
<br />


