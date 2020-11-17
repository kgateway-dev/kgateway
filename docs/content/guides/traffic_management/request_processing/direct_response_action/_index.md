---
title: Direct Response Action
weight: 40
description: Respond with a direct response instead of being proxied to any backend
---


Gloo Edge allows you to specify a direct response instead of routing to a destination. 

## Setup 

{{< readfile file="/static/content/setup_notes" markdown="true">}}

## Creating a direct response virtual service

Let's create a virtual service with a direct response action instead of a route action, returning a 200 "Hello, World!" response:

{{< tabs >}}
{{< tab name="kubectl" codelang="yaml">}}
{{< readfile file="guides/traffic_management/request_processing/direct_response_action/test-direct-response-vs.yaml">}}
{{< /tab >}}
{{< /tabs >}} 

Now if we curl the route, we should get the 200 response and see the message: 

```shell
curl -H "Host: foo" $(glooctl proxy url)
```

This will return the following message:

```shell
Hello, world!
```

## Summary

A virtual service route can be configured with a direct response instead of a routing action. 

Let's clean up the virtual service we created:

{{< tabs >}}
{{< tab name="kubectl" codelang="yaml">}}
kubectl delete vs -n gloo-system test-direct-response
{{< /tab >}}
{{< tab name="glooctl" codelang="shell" >}}
glooctl delete vs test-direct-response
{{< /tab >}}
{{< /tabs >}}

<br /> 
<br /> 