---
title: Path Matching
weight: 10
description: Match requests to routes based on the context path
---

The route rules in a *Virtual Service* can use path matching rules to match requests to routes based on the context path. There are three options that can be used for HTTP path matching. You can specify only one of the following three options within any given route matcher spec:

* [`prefix`](#prefix) - match if the beginning path of request path matches specified path.
* [`exact`](#exact) - match if request path matches specified path exactly.
* [`regex`](#regex) - match if the specified regular expression matches.

In this guide, we're going to take a closer look at each matching type by creating an *Upstream* and then creating a Virtual Service to route requests to that Upstream based on the path submitted as part of the request.

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

## Prefix Matching {#prefix}

To see how prefix matching is configured, let's create a Virtual Service and route to that Upstream.

<video controls loop>
  <source src="https://solo-docs.s3.us-east-2.amazonaws.com/gloo/videos/pathmatch_prefixcreate.mp4" type="video/mp4">
</video>

{{< tabs >}}
{{< tab name="kubectl" codelang="yaml">}}
{{< readfile file="guides/traffic_management/destination_selection/path_matching/prefix_vs.yaml">}}
{{< /tab >}}
{{< /tabs >}}

The prefix specified is `/posts`, meaning that any requests starting with `/posts` in the path will match this routing rule. 

In the domains portion of the `virtualHost` spec we are specifying the `foo` domain, meaning that this Virtual Service will only answer requests made against the host `foo`. This is useful in case there are other existing Virtual Services using the wildcard (`*`) domain for matching. If we make a curl request and don't provide the `Host` header with the value `foo`, the response will be a 404 as shown by the request below.

<video controls loop>
  <source src="https://solo-docs.s3.us-east-2.amazonaws.com/gloo/videos/pathmatch_prefixtest.mp4"  type="video/mp4">
</video>

```shell
curl -v $(glooctl proxy url)/posts
```

This will print a 404 response containing:
```
< HTTP/1.1 404 Not Found
< date: Wed, 31 Jul 2019 17:20:23 GMT
< server: envoy
< content-length: 0
```

If we pass the `Host` header, we will successfully get results. 

```shell
curl -H "Host: foo" $(glooctl proxy url)/posts
```

```json
[
  {
    "userId": 1,
    "id": 1,
    "title": "sunt aut facere repellat provident occaecati excepturi optio reprehenderit",
    "body": "quia et suscipit\nsuscipit recusandae consequuntur expedita et cum\nreprehenderit molestiae ut ut quas totam\nnostrum rerum est autem sunt rem eveniet architecto"
  },
  {
    "userId": 1,
    "id": 2,
    "title": "qui est esse",
    "body": "est rerum tempore vitae\nsequi sint nihil reprehenderit dolor beatae ea dolores neque\nfugiat blanditiis voluptate porro vel nihil molestiae ut reiciendis\nqui aperiam non debitis possimus qui neque nisi nulla"
  },
  {
    "userId": 1,
    "id": 3,
    "title": "ea molestias quasi exercitationem repellat qui ipsa sit aut",
    "body": "et iusto sed quo iure\nvoluptatem occaecati omnis eligendi aut ad\nvoluptatem doloribus vel accusantium quis pariatur\nmolestiae porro eius odio et labore et velit aut"
  },
  ...
]
```

A request to `/posts` matches on the prefix `/posts` and is routed to the Upstream at `jsonplaceholder.typicode.com:80`. A request sent to `/` does not match the prefix `/posts`.

```shell
curl -v -H "Host: foo" $(glooctl proxy url)/
```

A 404 is generated because there is no match for the host `foo` and the path `/`.

```console
> GET / HTTP/1.1
> Host: foo
> User-Agent: curl/7.58.0
> Accept: */*
>
< HTTP/1.1 404 Not Found
< date: Tue, 07 Jan 2020 20:10:37 GMT
< server: envoy
< content-length: 0
```

Let's clean up this Virtual Service and look at exact matchers next. 

<video controls loop>
  <source src="https://solo-docs.s3.us-east-2.amazonaws.com/gloo/videos/pathmatch_prefixdelete.mp4"  type="video/mp4">
</video>

{{< tabs >}}
{{< tab name="kubectl" codelang="yaml">}}
kubectl delete vs -n gloo-system test-prefix
{{< /tab >}}
{{< tab name="glooctl" codelang="shell" >}}
glooctl delete vs --name test-prefix
{{< /tab >}}
{{< /tabs >}}

---

## Exact matching {#exact}

Now let's configure a Virtual Service using the exact match option to route to our test Upstream. In this first example we are once again using the host `foo` and matching on the exact path `/`.

<video controls loop>
  <source src="https://solo-docs.s3.us-east-2.amazonaws.com/gloo/videos/pathmatch_exactcreate.mp4"  type="video/mp4">
</video>

{{< tabs >}}
{{< tab name="kubectl" codelang="yaml">}}
{{< readfile file="guides/traffic_management/destination_selection/path_matching/exact_vs_1.yaml">}}
{{< /tab >}}
{{< tab name="glooctl" codelang="shell" >}}
glooctl create vs --name test-exact-1 --namespace gloo-system --domains foo
glooctl add route --name test-exact-1 --path-exact / --dest-name json-upstream
{{< /tab >}}
{{< /tabs >}}

A request to the path `/posts` is not an exact match to `/`, and should return a 404.

<video controls loop>
  <source src="https://solo-docs.s3.us-east-2.amazonaws.com/gloo/videos/pathmatch_exacttest.mp4"  type="video/mp4">
</video>

```shell
curl -v -H "Host: foo" $(glooctl proxy url)/posts
```

Let's delete that Virtual Service and create a new one that works with the `/posts` path.

<video controls loop>
  <source src="https://solo-docs.s3.us-east-2.amazonaws.com/gloo/videos/pathmatch_exactdelete.mp4"  type="video/mp4">
</video>

{{< tabs >}}
{{< tab name="kubectl" codelang="yaml">}}
kubectl delete vs -n gloo-system test-exact-1
{{< /tab >}}
{{< tab name="glooctl" codelang="shell">}}
glooctl delete vs --name test-exact-1
{{< /tab >}}
{{< /tabs >}}

We're going to create a Virtual Service with a route that has an exact match on the path `/posts`.

<video controls loop>
  <source src="https://solo-docs.s3.us-east-2.amazonaws.com/gloo/videos/pathmatch_exactcreate_2.mp4"  type="video/mp4">
</video>

{{< tabs >}}
{{< tab name="kubectl" codelang="yaml">}}
{{< readfile file="guides/traffic_management/destination_selection/path_matching/exact_vs_2.yaml">}}
{{< /tab >}}
{{< tab name="glooctl" codelang="shell" >}}
glooctl create vs --name test-exact-2 --namespace gloo-system --domains foo
glooctl add route --name test-exact-2 --path-exact /posts --dest-name json-upstream
{{< /tab >}}
{{< /tabs >}}

Let's test the new Virtual Service by sending a request to the `/posts` path.

<video controls loop>
  <source src="https://solo-docs.s3.us-east-2.amazonaws.com/gloo/videos/pathmatch_exacttest_2.mp4"  type="video/mp4">
</video>

```shell
curl -v -H "Host: foo" $(glooctl proxy url)/posts
```

It will now return results.

```json
[
  {
    "userId": 1,
    "id": 1,
    "title": "sunt aut facere repellat provident occaecati excepturi optio reprehenderit",
    "body": "quia et suscipit\nsuscipit recusandae consequuntur expedita et cum\nreprehenderit molestiae ut ut quas totam\nnostrum rerum est autem sunt rem eveniet architecto"
  },
  {
    "userId": 1,
    "id": 2,
    "title": "qui est esse",
    "body": "est rerum tempore vitae\nsequi sint nihil reprehenderit dolor beatae ea dolores neque\nfugiat blanditiis voluptate porro vel nihil molestiae ut reiciendis\nqui aperiam non debitis possimus qui neque nisi nulla"
  },
  {
    "userId": 1,
    "id": 3,
    "title": "ea molestias quasi exercitationem repellat qui ipsa sit aut",
    "body": "et iusto sed quo iure\nvoluptatem occaecati omnis eligendi aut ad\nvoluptatem doloribus vel accusantium quis pariatur\nmolestiae porro eius odio et labore et velit aut"
  },
  ...
]
```

You can try any number of different combinations to see how the exact match option works. When you're done, let's clean up the exact match Virtual Service and check out the regex matcher.

<video controls loop>
  <source src="https://solo-docs.s3.us-east-2.amazonaws.com/gloo/videos/pathmatch_exactdelete_2.mp4"  type="video/mp4">
</video>

{{< tabs >}}
{{< tab name="kubectl" codelang="yaml">}}
kubectl delete vs -n gloo-system test-exact-2
{{< /tab >}}
{{< tab name="glooctl" codelang="shell" >}}
glooctl delete vs --name test-exact-2
{{< /tab >}}
{{< /tabs >}}

---

## Regex Matching {#regex}

Regex matching provides the most flexibility when using path matching, but it also adds complexity. Be sure to fully test your regex expressions before using them in production. 

{{% notice note %}}
The complexity of the regex is constrained by the regex engine's "program size" setting. If your regex is too complex, you may need to adjust the `regexMaxProgramSize` field
in the [GlooOptions section of your Settings resource]({{% versioned_link_path fromRoot="/reference/api/github.com/solo-io/gloo/projects/gloo/api/v1/settings.proto.sk/#gloooptions" %}}).
{{% /notice %}}

### Regex Example 1: Match Precise Count of Characters

Let's create a route that uses a regex matcher to match any path of five characters in the set `[a-z]`.

<video controls loop>
  <source src="https://solo-docs.s3.us-east-2.amazonaws.com/gloo/videos/pathmatch_regexcreate.mp4"  type="video/mp4">
</video>

{{< tabs >}}
{{< tab name="kubectl" codelang="yaml">}}
{{< readfile file="guides/traffic_management/destination_selection/path_matching/regex_vs.yaml">}}
{{< /tab >}}
{{< tab name="glooctl" codelang="shell" >}}
glooctl create vs --name test-regex --namespace gloo-system --domains foo
glooctl add route --name test-regex --path-regex /[a-z]{5} --dest-name json-upstream
{{< /tab >}}
{{< /tabs >}}

The regex matcher should work on the path `/posts`, but not on the path `/comments` or `/list`. The path `/comments` is over five characters and the path `/list` is less than five characters. Let's test out the path `/comments`.

<video controls loop>
  <source src="https://solo-docs.s3.us-east-2.amazonaws.com/gloo/videos/pathmatch_regextest.mp4"  type="video/mp4">
</video>

```shell
curl -v -H "Host: foo" $(glooctl proxy url)/comments
```

The result is a 404 response.

```console
> GET /comments HTTP/1.1
> Host: foo
> User-Agent: curl/7.58.0
> Accept: */*
>
< HTTP/1.1 404 Not Found
```

However, the following requests will both succeed since they have exactly five characters in them:
```shell
curl -v -H "Host: foo" $(glooctl proxy url)/posts
```

```json
[
  {
    "userId": 1,
    "id": 1,
    "title": "sunt aut facere repellat provident occaecati excepturi optio reprehenderit",
    "body": "quia et suscipit\nsuscipit recusandae consequuntur expedita et cum\nreprehenderit molestiae ut ut quas totam\nnostrum rerum est autem sunt rem eveniet architecto"
  },
  {
    "userId": 1,
    "id": 2,
    "title": "qui est esse",
    "body": "est rerum tempore vitae\nsequi sint nihil reprehenderit dolor beatae ea dolores neque\nfugiat blanditiis voluptate porro vel nihil molestiae ut reiciendis\nqui aperiam non debitis possimus qui neque nisi nulla"
  },
  {
    "userId": 1,
    "id": 3,
    "title": "ea molestias quasi exercitationem repellat qui ipsa sit aut",
    "body": "et iusto sed quo iure\nvoluptatem occaecati omnis eligendi aut ad\nvoluptatem doloribus vel accusantium quis pariatur\nmolestiae porro eius odio et labore et velit aut"
  },
  ...
]
```

```shell
curl -v -H "Host: foo" $(glooctl proxy url)/todos
```

```json
[
  {
    "userId": 1,
    "id": 1,
    "title": "delectus aut autem",
    "completed": false
  },
  {
    "userId": 1,
    "id": 2,
    "title": "quis ut nam facilis et officia qui",
    "completed": false
  },
  {
    "userId": 1,
    "id": 3,
    "title": "fugiat veniam minus",
    "completed": false
  },
  {
    "userId": 1,
    "id": 4,
    "title": "et porro tempora",
    "completed": true
  },
  ...
```

You can replace this Virtual Service with other regex expressions to see how they react. 

### Regex Example 2: Match Path Containing a UUID

Let's consider an example where we need to match on a path containing an 8-4-4-4-12 formatted UUID. Let's say we have a requirement to match on paths that contain a UUID, like this:
* `/api/v1/<uuid>/good-path`
* `/api/v1/<uuid>`
But we want to disallow any other paths that don't precisely match the patterns above, for example:
* `/api/v1/<uuid>/bad-path`

We will accomplish this using a Virtual Service that incorporates a regex to match the UUID:

{{< tabs >}}
{{< tab name="kubectl" codelang="yaml">}}
{{< readfile file="guides/traffic_management/destination_selection/path_matching/regex_vs_uuid.yaml">}}
{{< /tab >}}
{{< /tabs >}}

Apply the modified Virtual Service spec above and test it with the following scenarios.

#### Test 1: Match full UUID path

```
curl -i -H "Host: foo" $(glooctl proxy url)/api/v1/123e4567-e89b-12d3-a456-426614174000/good-path
```

Note that the request contains a valid UUID followed by `good-path` in the request. So the response indicates that it matched a UUID and good-path.
```
HTTP/1.1 200 OK
content-length: 21
content-type: text/plain
date: Tue, 08 Feb 2022 22:12:22 GMT
server: envoy

api v1 uuid good-path
```

#### Test 2: Match shorter UUID path

```
curl -i -H "Host: foo" $(glooctl proxy url)/api/v1/123e4567-e89b-12d3-a456-426614174000
```

Note that the request contains a valid UUID at the end of the path. So the response indicates that it matched the path containing only a UUID.
```
HTTP/1.1 200 OK
content-length: 11
content-type: text/plain
date: Tue, 08 Feb 2022 22:13:41 GMT
server: envoy

api v1 uuid
```

#### Test 3: Mismatch on invalid UUID format

```
curl -i -H "Host: foo" $(glooctl proxy url)/api/v1/123e4567-e89b-12d3-a456-bad-uuid/good-path
```

Note that the request contains a malformed UUID. So the response is an HTTP 404, because no matcher was found.
```
HTTP/1.1 404 Not Found
date: Tue, 08 Feb 2022 22:13:22 GMT
server: envoy
content-length: 0
```

#### Test 4: Mismatch on invalid path request

```
curl -i -H "Host: foo" $(glooctl proxy url)/api/123e4567-e89b-12d3-a456-426614174000/bad-path
```

Note that the request contains a `bad-path` at the end of the path, which is not supported by any of the matchers. So the response is an HTTP 404, because no matcher was found.
```
HTTP/1.1 404 Not Found
date: Tue, 08 Feb 2022 22:13:34 GMT
server: envoy
content-length: 0
```

## Cleanup

When you are finished, you can simply delete the `test-regex` virtual service.

<video controls loop>
  <source src="https://solo-docs.s3.us-east-2.amazonaws.com/gloo/videos/pathmatch_regexdelete.mp4"  type="video/mp4">
</video>

{{< tabs >}}
{{< tab name="kubectl" codelang="yaml">}}
kubectl delete vs -n gloo-system test-regex
{{< /tab >}}
{{< tab name="glooctl" codelang="shell" >}}
glooctl delete vs --name test-regex
{{< /tab >}}
{{< /tabs >}}

{{% notice note %}}
Envoy uses the [Google RE2](https://github.com/google/re2) regular expression engine internally.  A more complete description of the grammar available to Gloo Edge `regex` matchers is provided [here](https://github.com/google/re2/wiki/Syntax).
{{% /notice %}}

---

## Summary

In this tutorial, we created a static Upstream and added a route on a Virtual Service to point to it. We learned how to use all 3 types of matchers allowed by Gloo Edge when determining if a route configuration matches a request path: prefix, exact, and regex. 

Let's cleanup the test Upstream we used.

<video controls loop>
  <source src="https://solo-docs.s3.us-east-2.amazonaws.com/gloo/videos/pathmatch_deleteupstream.mp4"  type="video/mp4">
</video>

{{< tabs >}}
{{< tab name="kubectl" codelang="yaml">}}
kubectl delete upstream -n gloo-system json-upstream
{{< /tab >}}
{{< tab name="glooctl" codelang="shell" >}}
glooctl delete upstream json-upstream
{{< /tab >}}
{{< /tabs >}}

### Next Steps

Path matching rules are not the only rules available for routing decisions. We recommend checking out any of the following guides next:

* [Header Matching]({{< versioned_link_path fromRoot="/guides/traffic_management/destination_selection/header_matching/" >}})
* [Query Parameter Matching]({{< versioned_link_path fromRoot="/guides/traffic_management/destination_selection/query_parameter_matching/" >}})
* [HTTP Method Matching]({{< versioned_link_path fromRoot="/guides/traffic_management/destination_selection/http_method_matching/" >}})

