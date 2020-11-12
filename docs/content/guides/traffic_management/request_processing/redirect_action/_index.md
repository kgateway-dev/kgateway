---
title: Host Redirect
weight: 30
description: Configure Gloo Edge to redirect requests to a route instead of routing to a destination. 
---

Gloo Edge allows you to specify redirects instead of routes to a destination. 

## Setup 

{{< readfile file="/static/content/setup_notes" markdown="true">}}

## Creating a redirect virtual service

Let's create a virtual service with a redirect action instead of a route action, redirecting "foo" to "google.com":

{{< tabs >}}
{{< tab name="kubectl" codelang="yaml">}}
{{< readfile file="guides/traffic_management/request_processing/redirect_action/test-redirect-vs.yaml">}}
{{< /tab >}}
{{< /tabs >}} 

Now if we curl the route, we should get a 301 Permanently Moved response. 

```shell
curl -v -H "Host: foo" $(glooctl proxy url)
```

This will contain the following message:

```shell
< HTTP/1.1 301 Moved Permanently
< location: http://google.com/
```

## Summary

A virtual service route can be configured with a redirect instead of a routing action. 

Let's clean up the virtual service we created:

{{< tabs >}}
{{< tab name="kubectl" codelang="yaml">}}
kubectl delete vs -n gloo-system test-redirect
{{< /tab >}}
{{< tab name="glooctl" codelang="shell" >}}
glooctl delete vs test-redirect
{{< /tab >}}
{{< /tabs >}}

<br /> 
<br /> 

