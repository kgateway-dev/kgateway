---
title: HTTP Method Matching
weight: 40
description: Match requests to routes based on HTTP method
---

You can also create route rules based on the request HTTP method, e.g. GET, POST, DELETE, etc. You can specify one or
more HTTP Methods to match against, and if any one of those method verbs is present, the request will match, that is
Gloo will conditional OR the match for HTTP Method. Note: since Gloo/Envoy is based on HTTP/2, this gets translated
into a header value match against the HTTP/2 `:method` header, which [by spec](https://http2.github.io/http2-spec/#HttpRequest)
includes all of the HTTP/1 verbs.

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

Let's create a virtual service with an http method match on `POST`:

{{< tabs >}}
{{< tab name="kubectl" codelang="yaml">}}
{{< readfile file="gloo_routing/virtual_services/routes/matching_rules/http_method_matching/test-post-vs.yaml">}}
{{< /tab >}}
{{< /tabs >}} 

Let's POST to that route and make sure it works:

```shell
curl -H "Host: foo" -XPOST $(glooctl proxy url)/posts
```

returns

```
{
  "id": 101
}
```

Let's delete that virtual service. 

{{< tabs >}}
{{< tab name="kubectl" codelang="yaml">}}
kubectl delete vs -n gloo-system test-post
{{< /tab >}}
{{< tab name="glooctl" codelang="shell" >}}
glooctl delete vs --name test-post
{{< /tab >}}
{{< /tabs >}}

Now let's create a virtual service that matches on GET:

{{< tabs >}}
{{< tab name="kubectl" codelang="yaml">}}
{{< readfile file="gloo_routing/virtual_services/routes/matching_rules/http_method_matching/test-get-vs.yaml">}}
{{< /tab >}}
{{< /tabs >}} 

Now POST requests will return a 404:

```shell
curl -v -H "Host: foo" -XPOST $(glooctl proxy url)/posts
```

But GET requests succeed:

```shell
curl -H "Host: foo" $(glooctl proxy url)/posts
```

## Summary 

In this guide, we created a virtual service that utilized HTTP method matching and demonstrated it on POST and GET requests. 

Cleanup the resources by running:

{{< tabs >}}
{{< tab name="kubectl" codelang="yaml">}}
kubectl delete vs -n gloo-system test-get
kubectl delete upstream -n gloo-system json-upstream
{{< /tab >}}
{{< tab name="glooctl" codelang="shell" >}}
glooctl delete vs test-get
glooctl delete upstream json-upstream
{{< /tab >}}
{{< /tabs >}}

<br /> 
<br />
