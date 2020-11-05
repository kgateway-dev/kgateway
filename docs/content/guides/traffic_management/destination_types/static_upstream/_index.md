---
title: Static Upstreams
weight: 10
description: Routing to explicitly and statically defined Upstreams
---

Let's configure Gloo Edge to route to a single, static Upstream. In this case, we'll route requests through Gloo Edge to the JSON testing API available at `http://jsonplaceholder.typicode.com/`. 

{{< readfile file="/static/content/setup_notes" markdown="true">}}

## Create Upstream

Let's create a simple upstream for testing called `json-upstream`, that routes to a static site:

{{< tabs >}}
{{< tab name="kubectl" codelang="yaml">}}
{{< readfile file="/static/content/upstream.yaml">}}
{{< /tab >}}
{{< tab name="glooctl" codelang="shell" >}}
glooctl create upstream static --static-hosts jsonplaceholder.typicode.com:80 --name json-upstream
{{< /tab >}}
{{< /tabs >}}

## Create Virtual Service

Now let's create a virtual service that routes all requests to the `foo` domain to that upstream. 

{{< tabs >}}
{{< tab name="kubectl" codelang="yaml">}}
{{< readfile file="guides/traffic_management/destination_types/static_upstream/virtual-service.yaml">}}
{{< /tab >}}
{{< /tabs >}}

## Test routes

Now we can verify that the proxy was updated to support routing to this upstream using curl:

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

## Summary

In this example, we created a static upstream and created a virtual service with a route to it. We showed using curl that the 
proxy was configured with this route. 

Let's cleanup the test upstream and virtual service we created:

{{< tabs >}}
{{< tab name="kubectl" codelang="yaml">}}
kubectl delete vs -n gloo-system test-static
kubectl delete upstream -n gloo-system json-upstream
{{< /tab >}}
{{< tab name="glooctl" codelang="shell" >}}
glooctl delete vs test-static
glooctl delete upstream json-upstream
{{< /tab >}}
{{< /tabs >}}

<br /> 
<br />