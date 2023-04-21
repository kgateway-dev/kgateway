---
title: Shadowing
weight: 110
description: Enable traffic shadowing for the route.
---

## Why traffic shadowing?
When releasing changes to a service, you want to finely control how those changes get exposed to users. This "[progressive delivery](https://redmonk.com/jgovernor/2018/08/06/towards-progressive-delivery/)" approach to releasing software allows you to reduce the blast radius when changes may introduce unintended bad behaviors. Approaches for controlling the release of new changes by slowly exposing them include [canary release, blue-green deployment](https://blog.christianposta.com/deploy/blue-green-deployments-a-b-testing-and-canary-releases/), and traffic shadowing. With traffic shadowing, we make a copy of the incoming request and send that request out-of-band (ie, out of the real request path, and asynchronously) to the new version of our software. From there we can simulate taking live production traffic (it is actually production traffic) without affecting real users. We can observe the behavior of the new release, compare it against expected results, and determine whether to proceed with the release (by rolling out to real traffic with % based canary, etc) or roll it back.

## What is traffic shadowing with Gloo Edge?
Traffic shadowing with Gloo Edge makes a copy of an incoming request and proxies the real request to the appropriate backend (the normal request path) and sends the copy to another upstream. The copied message is ignored for failures or responses. In this case, you can deploy `v2` of a service and shadow traffic to it *in production* without affecting user traffic. This ability to shadow is incredibly important because it allows you to begin your release or canary process with zero production impact. 

![Gloo Edge traffic shadowing diagram]({{% versioned_link_path fromRoot="/img/traffic-shadowing.png" %}})

When shadowing traffic, you can use tools like [Open Diffy](https://github.com/opendiffy/diffy), or [Diferencia](https://github.com/lordofthejars/diferencia) to do diff-compares on the responses of the traffic. This way you can verify the response is correct (these tools do a diff between the real response and the new service's response) in a way that can also detect API forward/backward compatibility problems. 


## How to shadow traffic with Gloo Edge

To enable traffic shadowing in Gloo Edge, we need to add a route option to the VirtualService as seen below. We configure the new service to which to shadow as well as how much of the original live traffic to shadow. For example, you may wish to only shadow 5% of all traffic and observe how it behaves. The following fields control those variables:

* `upstream` : Indicates the upstream to which to send the shadowed traffic.
* `percentage` : Percent of traffic to shadow (must be an integer between 0 and 100).

In the example below, all traffic going to `petstore` is also forwarded to `petstore-v2`.
{{< highlight yaml "hl_lines=19-23" >}}
apiVersion: gateway.solo.io/v1
kind: VirtualService
metadata:
  name: 'default'
  namespace: 'gloo-system'
spec:
  virtualHost:
    domains:
    - '*'
    routes:
    - matchers:
       - prefix: '/petstore'
      routeAction:
        single:
          upstream:
            name: 'petstore'
            namespace: 'gloo-system'
      options:
        shadowing:
          upstream:
            name: 'petstore-v2'
            namespace: 'gloo-system'
          percentage: 100
{{< /highlight >}}

## How does your service know it's shadowed traffic?

When your new service gets a copy of a live-traffic message (ie, the copy), how can your service know that this is indeed a copy? This could be valuable information in how your service deals with the message, especially if this is a stateful service. For example, if you can detect this is a shadowed message, you can rollback any stateful transactions that may be associated with the processing of the message. 

With Gloo Edge, since it's based on Envoy, the `Host` or `Authority` header includes a `-shadow` postfix to it. For example, if we're sending traffic to `foo.bar.com` the `Host` value will then be `foo.bar.com-shadow`. From there, the service can detect this and response accordingly. See this blog [on advanced traffic shadowing patterns](https://blog.christianposta.com/microservices/advanced-traffic-shadowing-patterns-for-microservices-with-istio-service-mesh/) for more.
