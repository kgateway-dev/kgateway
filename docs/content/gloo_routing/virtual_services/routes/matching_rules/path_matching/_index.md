---
title: Path Matching
weight: 10
description: Match requests to routes based on the context path
---


There are three options to do HTTP path matching. You can specify only one of the following three options within any given
route matcher spec:

* [`prefix`](#prefix) - match if the beginning path of request path matches specified path.
* [`exact`](#exact) - match if request path matches specified path exactly.
* [`regex`](#regex) - match if the specified regular expression matches. 

---

## Setup

{{< readfile file="/static/content/setup_notes" markdown="true">}}

If you have not yet deployed Gloo, you can start by following the directions contained within the guide [Installing Gloo Gateway on Kubernetes]({{% versioned_link_path fromRoot="/installation/gateway/kubernetes/" %}}).

---

Let's create a simple upstream for testing called `json-upstream`, that routes to a static site:

{{< tabs >}}
{{< tab name="kubectl" codelang="yaml">}}
{{< readfile file="/static/content/upstream.yaml">}}
{{< /tab >}}
{{< tab name="glooctl" codelang="shell" >}}
glooctl create upstream static --static-hosts jsonplaceholder.typicode.com:80 --name json-upstream
{{< /tab >}}
{{< /tabs >}}

## Prefix Matching {#prefix}

To see how prefix matching is configured, let's create a virtual service and route to that upstream:

{{< tabs >}}
{{< tab name="kubectl" codelang="yaml">}}
{{< readfile file="gloo_routing/virtual_services/routes/matching_rules/path_matching/prefix_vs.yaml">}}
{{< /tab >}}
{{< /tabs >}}

Since we are using the `foo` domain, if we make a curl request and don't provide the `Host` header, it will 404. 

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

A request to the `/path` matches on the prefix `/` and is routed to the upstream at `jsonplaceholder.typicode.com:80`.

Let's clean up this virtual service and look at exact matchers next. 

{{< tabs >}}
{{< tab name="kubectl" codelang="yaml">}}
kubectl delete vs -n gloo-system test-prefix
{{< /tab >}}
{{< tab name="glooctl" codelang="shell" >}}
glooctl delete vs --name test-prefix
{{< /tab >}}
{{< /tabs >}}

## Exact matching {#exact}

Now let's configure an exact match virtual service to route to our test upstream, first matching on `/`:

{{< tabs >}}
{{< tab name="kubectl" codelang="yaml">}}
{{< readfile file="gloo_routing/virtual_services/routes/matching_rules/path_matching/exact_vs_1.yaml">}}
{{< /tab >}}
{{< tab name="glooctl" codelang="shell" >}}
glooctl create vs --name test-exact-1 --namespace gloo-system --domains foo
glooctl add route --name test-exact-1 --path-exact / --dest-name json-upstream
{{< /tab >}}
{{< /tabs >}}

Now a request to path `/posts` will not match and should return a 404:

```shell
curl -v -H "Host: foo" $(glooctl proxy url)/posts
```

Let's delete that virtual service: 

{{< tabs >}}
{{< tab name="kubectl" codelang="yaml">}}
kubectl delete vs -n gloo-system test-exact-1
{{< /tab >}}
{{< tab name="glooctl" codelang="shell">}}
glooctl delete vs --name test-exact-1
{{< /tab >}}
{{< /tabs >}}

Now let's create a virtual service with a route that has an exact match on `/posts`:

{{< tabs >}}
{{< tab name="kubectl" codelang="yaml">}}
{{< readfile file="gloo_routing/virtual_services/routes/matching_rules/path_matching/exact_vs_2.yaml">}}
{{< /tab >}}
{{< tab name="glooctl" codelang="shell" >}}
glooctl create vs --name test-exact-2 --namespace gloo-system --domains foo
glooctl add route --name test-exact-2 --path-exact /posts --dest-name json-upstream
{{< /tab >}}
{{< /tabs >}}

```shell
curl -v -H "Host: foo" $(glooctl proxy url)/posts
```

Now returns results.

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

Let's delete this virtual service: 

{{< tabs >}}
{{< tab name="kubectl" codelang="yaml">}}
kubectl delete vs -n gloo-system test-exact-2
{{< /tab >}}
{{< tab name="glooctl" codelang="shell" >}}
glooctl delete vs --name test-exact-2
{{< /tab >}}
{{< /tabs >}}

## Regex Matching {#regex}

Finally, let's create a route that uses a regex matcher, in this case matching on any 5-character path: 

{{< tabs >}}
{{< tab name="kubectl" codelang="yaml">}}
{{< readfile file="gloo_routing/virtual_services/routes/matching_rules/path_matching/regex_vs.yaml">}}
{{< /tab >}}
{{< tab name="glooctl" codelang="shell" >}}
glooctl create vs --name test-regex --namespace gloo-system --domains foo
glooctl add route --name test-regex --path-regex /[a-z]{5} --dest-name json-upstream
{{< /tab >}}
{{< /tabs >}}

This request will return a 404 because the path `/comments` is more than 5 characters:

```shell
curl -v -H "Host: foo" $(glooctl proxy url)/comments
```

However, the following requests will both succeed:
```shell
curl -v -H "Host: foo" $(glooctl proxy url)/posts
```

```shell
curl -v -H "Host: foo" $(glooctl proxy url)/todos
```

Let's delete this virtual service: 

{{< tabs >}}
{{< tab name="kubectl" codelang="yaml">}}
kubectl delete vs -n gloo-system test-regex
{{< /tab >}}
{{< tab name="glooctl" codelang="shell" >}}
glooctl delete vs --name test-regex
{{< /tab >}}
{{< /tabs >}}

## Summary

In this tutorial, we created a static upstream and added a route on a virtual service to point to it. We learned how to 
use all 3 types of matchers allowed by Gloo when determining if a route configuration matches a request path: 
prefix, exact, and regex. 

Let's cleanup the test upstream we used:

{{< tabs >}}
{{< tab name="kubectl" codelang="yaml">}}
kubectl delete upstream -n gloo-system json-upstream
{{< /tab >}}
{{< tab name="glooctl" codelang="shell" >}}
glooctl delete upstream json-upstream
{{< /tab >}}
{{< /tabs >}}

<br /> 
<br />

