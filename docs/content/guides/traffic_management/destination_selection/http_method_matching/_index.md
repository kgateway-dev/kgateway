---
title: HTTP Method Matching
weight: 40
description: Match requests to routes based on HTTP method
---

The route rules in a *Virtual Service* can evaluate based on the request HTTP method, e.g. GET, POST, DELETE, etc. You can specify one or more HTTP Methods to match against, and if any one of those method verbs is present, the request will match, that is Gloo Edge will conditional OR the match for HTTP Method. 

{{% notice note %}}
Gloo Edge/Envoy is based on HTTP/2, the HTTP method gets translated into a header value match against the HTTP/2 `:method` header, which [by spec](https://http2.github.io/http2-spec/#HttpRequest) includes all of the HTTP/1 verbs.
{{% /notice %}}

In this guide, we're going to take a closer look at different method matches by creating an *Upstream* and then creating a Virtual Service to route requests to that Upstream based on the method used in the request.

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

Our next step is to create a series of Virtual Services to test the `POST` and `GET` method matchers. We will first create a new Virtual Service for the `POST` method. Then delete that Virtual Service and create a second one for the `GET` method.

### Create the POST Virtual Service

Let's create a Virtual Service with an http method match on `POST` on the prefix `/`. This route rule will match on requests that use the `POST` method and reference any path that starts with `/`, i.e. all paths on the Upstream.

<video controls loop>
  <source src="https://solo-docs.s3.us-east-2.amazonaws.com/gloo/videos/httpmatch_createvs1.mp4" type="video/mp4">
</video>

{{< tabs >}}
{{< tab name="kubectl" codelang="yaml">}}
{{< readfile file="guides/traffic_management/destination_selection/http_method_matching/test-post-vs.yaml">}}
{{< /tab >}}
{{< /tabs >}} 

Now let's send a POST request to that route and make sure we receive a valid response.

<video controls loop>
  <source src="https://solo-docs.s3.us-east-2.amazonaws.com/gloo/videos/httpmatch_test1.mp4" type="video/mp4">
</video>

```shell
curl -H "Host: foo" -XPOST $(glooctl proxy url)/posts
```

returns

```
{
  "id": 101
}
```

The response confirms that a new blog post was created on the Upstream.

Now let's delete that Virtual Service to prepare for the next method.

{{< tabs >}}
{{< tab name="kubectl" codelang="yaml">}}
kubectl delete vs -n gloo-system test-post
{{< /tab >}}
{{< tab name="glooctl" codelang="shell" >}}
glooctl delete vs --name test-post
{{< /tab >}}
{{< /tabs >}}

### Create the GET Virtual Service

We've seen how the `POST` method matches, now let's create a Virtual Service that matches on `GET` instead.

<video controls loop>
  <source src="https://solo-docs.s3.us-east-2.amazonaws.com/gloo/videos/httpmatch_createvs2.mp4" type="video/mp4">
</video>

{{< tabs >}}
{{< tab name="kubectl" codelang="yaml">}}
{{< readfile file="guides/traffic_management/destination_selection/http_method_matching/test-get-vs.yaml">}}
{{< /tab >}}
{{< /tabs >}} 

Because we removed the `POST` method matcher, sending a request using the `POST` method will now result in a 404 response from the Virtual Service. A `GET` request will successfully return a list of blog posts from the Upstream.

<video controls loop>
  <source src="https://solo-docs.s3.us-east-2.amazonaws.com/gloo/videos/httpmatch_test2.mp4" type="video/mp4">
</video>

`POST` request:

```shell
curl -v -H "Host: foo" -XPOST $(glooctl proxy url)/posts
```

`GET` request:

```shell
curl -H "Host: foo" $(glooctl proxy url)/posts
```

---

## Summary 

In this guide, we created a Virtual Service that utilized HTTP method matching and demonstrated it on POST and GET requests. 

Let's cleanup the Virtual Service and Upstream we used.

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

### Next Steps

HTTP method matching rules are not the only rules available for routing decisions. We recommend checking out any of the following guides next:

* [Header Matching]({{< versioned_link_path fromRoot="/guides/traffic_management/destination_selection/header_matching/" >}})
* [Path Matching]({{< versioned_link_path fromRoot="/guides/traffic_management/destination_selection/path_matching/" >}})
* [Query Parameter Matching]({{< versioned_link_path fromRoot="/guides/traffic_management/destination_selection/query_parameter_matching/" >}})
