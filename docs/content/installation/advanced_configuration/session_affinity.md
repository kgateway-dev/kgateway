---
title: Session Affinity
weight: 48
description: Configure Gloo Edge session affinity (sticky sessions)
---

For certain applications deployed across multiple replicas, it may be desirable to route all traffic from a single client session to the same instance of the application. This can help reduce latency through better use of caches. This load balancer behavior is referred to as Session Affinity or Sticky Sessions. Gloo Edge exposes Envoy's full session affinity capabilities, as described below.

---

## Configuration overview

There are two steps to configuring session affinity:

1. Set a hashing load balancer on the upstream specification.
  - This can be either Envoy's Ring Hash or Maglev load balancer.
1. Define the hash key parameters on the desired routes.
  - This can include any combination of headers, cookies, and source IP address.


Below, we show how to configure Gloo Edge to use hashing load balancers and demonstrate a common cookie-based hashing strategy using a Ring Hash load balancer.

---

## Upstream Plugin Configuration

- Whether an upstream was discovered by Gloo Edge or created manually, just add the `loadBalancerConfig` spec to your upstream.
- Either a `ringHash` or `maglev` load balancer must be specified to achieve session affinity. Some examples are shown below.
  - To determine whether a Ring Hash or Maglev load balancer is best for your use case, please review
the details in Envoy's [load balancer selection docs](https://www.envoyproxy.io/docs/envoy/latest/intro/arch_overview/upstream/load_balancing/load_balancers#ring-hash).
  - In many cases, either load balancer will work.

### Configure a Ring Hash Load Balancer on an Upstream

- Full reference specification:

{{< highlight yaml "hl_lines=17-21" >}}
apiVersion: gloo.solo.io/v1
kind: Upstream
metadata:
  annotations:
  labels:
    discovered_by: kubernetesplugin
  name: default-session-affinity-app-80
  namespace: gloo-system
spec:
  kube:
    selector:
      name: session-affinity-app
    serviceName: session-affinity-app
    serviceNamespace: default
    servicePort: 80
  loadBalancerConfig:
    ringHash:
      ringHashConfig:
        maximumRingSize: "200"
        minimumRingSize: "10"
{{< /highlight >}}

- Optional fields omitted:

{{< highlight yaml "hl_lines=17-18" >}}
apiVersion: gloo.solo.io/v1
kind: Upstream
metadata:
  annotations:
  labels:
    discovered_by: kubernetesplugin
  name: default-session-affinity-app-80
  namespace: gloo-system
spec:
  kube:
    selector:
      name: session-affinity-app
    serviceName: session-affinity-app
    serviceNamespace: default
    servicePort: 80
  loadBalancerConfig:
    ringHash: {}
{{< /highlight >}}

### Configure a Maglev Load Balancer on an Upstream

- There are no configurable parameters for Maglev load balancers:

{{< highlight yaml "hl_lines=2-2" >}}
    loadBalancerConfig:
      maglev: {}
{{< /highlight >}}

### Route Plugin Configuration

- Full reference specification:

{{< highlight yaml "hl_lines=20-29" >}}
apiVersion: gateway.solo.io/v1
kind: VirtualService
metadata:
  name: default
  namespace: gloo-system
spec:
  virtualHost:
    domains:
    - '*'
    routes:
    - matchers:
       - exact: /route1
      routeAction:
        single:
          upstream:
            name: default-session-affinity-app-80
            namespace: gloo-system
      options:
        lbHash:
          hashPolicies: # (1)
          - header: x-test-affinity
            terminal: true # (2)
          - header: origin # (3)
          - sourceIp: true # (4)
          - cookie: # (5)
              name: gloo
              path: /abc
              ttl: 1s # (6)
        prefixRewrite: /count
{{< /highlight >}}

### Notes on hash policies

1. One or more `hashPolicies` may be specified.
2. Ordering of hash policies matters in that any hash policy can be `terminal`, meaning that if Envoy is able to create a hash key with the policies that it has processed up to and including that policy, it will ignore the subsequent policies. This can be used for implementing a content-contingent hashing policy optimization. For example, if a "x-unique-id" header is available, Envoy can save time by ignoring the later identifiers.
  - Optional, default: `false`
3. `header` policies indicate headers that should be included in the hash key.
4. The `sourceIp` policy indicates that the request's sourece IP address should be included in the hash key.
5. `cookie` policies indicate that the specified cookie should be included in the hash key.
  - `name`, required, identifies the cookie
  - `path`, optional, cookie path
  - `ttl`, optional, if set, Envoy will create the specified cookie, if it is not present on the request
6. Envoy can be configured to create cookies by setting the `ttl` parameter. If the specified cookie is not available on the request, Envoy will create it and add it to the response.

For additional insights, please refer to Envoy's [route hash policy documentation](https://www.envoyproxy.io/docs/envoy/latest/api-v3/config/route/v3/route.proto#route-routeaction-hashpolicy).

---

## Tutorial: Cookie-based route hashing

The following tutorial walks through the steps involved in configuring and verifying session affinity.

### Summary

- Before enabling session affinity, each instance of our "Counter" app will service our requests in turn (Round Robin).
  - This will result in non-incrementing responses, such as [1,1,1,2,2,2,3,3,...].
- After enabling cookie-based session affinity, a single instance of our "Counter" app will service all requests.
  - This will produce incremeting responses, such as [4,5,6,...].

### Requirements

- Kubernetes cluster with Gloo Edge installed
- At least two nodes in the cluster.
- Permission to deploy a DaemonSet and edit Gloo Edge resources.

### Deploy a sample app in a DaemonSet

DaemonSets are one type of resource that may benefit from session affinity. A DaemonSet ensures that all (or some) nodes run a given Pod. Depending on your architecture, you may have node-local caches that you want to associate with segments of your traffic. Session affinity can help steer requests from a given client to a consistent node.

### Overview of the "Counter" application

We will use a very simple "counter" app to demonstrate session affinity configuration. The counter simply reports how many requests have been made to the `/count` endpoint. Without session affinity, subsequent requests will return a non-monotonically increasing response. For example, on a fresh deployment, your first request will be handled by node 1, and return a count of 1. Your second request will by handled by node 2, and also return a count of 1. After you enable session affinity, repeat requests will return a strictly increasing count response.

The source code for the session affinity app is available [here](https://github.com/solo-io/gloo/blob/v1.2.12/docs/examples/session-affinity/main.go).

The core logic is shown below.

{{< highlight golang "hl_lines=24-29" >}}
package main

import (
	"fmt"
	"net/http"
	"os"
)

func main() {
	if err := App(); err != nil {
		os.Exit(1)
	}
}

var (
	countUrl = "/count"
	helpMsg  = fmt.Sprintf(`Simple counter app for testing Gloo Edge

%v - reports number of times the %v path was queried`, countUrl, countUrl)
)

func App() error {
	count := 0
	http.HandleFunc(countUrl, func(w http.ResponseWriter, r *http.Request) {
		count++
		if _, err := fmt.Fprint(w, count); err != nil {
			fmt.Printf("error with request: %v\n", err)
		}
	})
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if _, err := fmt.Fprint(w, helpMsg); err != nil {
			fmt.Printf("error with request: %v\n", err)
		}
	})
	return http.ListenAndServe("0.0.0.0:8080", nil)

}
{{< /highlight >}}

### Apply the DaemonSet

The following command will create our DaemonSet and a matching Service.

```yaml
cat << EOF | kubectl apply -f -
apiVersion: apps/v1
kind: DaemonSet
metadata:
  name: session-affinity-app
spec:
  selector:
    matchLabels:
      name: session-affinity-app
      app: session-affinity-app
  template:
    metadata:
      labels:
        name: session-affinity-app
        app: session-affinity-app
    spec:
      containers:
      - name: session-affinity-app
        image: soloio/session-affinity-app:0.0.3
        resources:
          limits:
            memory: 10Mi
          requests:
            cpu: 10m
            memory: 10Mi
---
apiVersion: v1
kind: Service
metadata:
  name: session-affinity-app
spec:
  selector:
    name: session-affinity-app
  ports:
    - protocol: TCP
      port: 80
      targetPort: 8080
EOF
```

{{% notice note %}}
If you deployed the app to a namespace other than the default namespace you will need to adjust the following commands accordingly.
{{% /notice %}}

Gloo Edge will have discovered the `session-affinity-app` service and created an Upstream from it.

Now create a route to the app with `glooctl`:

```
glooctl add route --path-exact /route1 --dest-name default-session-affinity-app-80 --prefix-rewrite /count --name default
```

In a browser, navigate to this route, `/route1`, on your gateway's URL (you can find this with `glooctl proxy url`). If you refresh the page, you should observe a non-incrementing count. For example, in cluster with three nodes, you should see something like the sequence:

```
1,1,1,2,2,2,3,3,3,4,4,4,...
```

### Apply the session affinity configuration

#### Configure the upstream

Use `kubectl edit upstream -n gloo-system default-session-affinity-app-80` and apply the changes shown below to set a hashing load balancer on the app's upstream.

{{< highlight yaml "hl_lines=17-21" >}}
apiVersion: gloo.solo.io/v1
kind: Upstream
metadata:
  annotations:
  labels:
    discovered_by: kubernetesplugin
  name: default-session-affinity-app-80
  namespace: gloo-system
spec:
  kube:
    selector:
      name: session-affinity-app
    serviceName: session-affinity-app
    serviceNamespace: default
    servicePort: 80
  loadBalancerConfig:
    ringHash:
      ringHashConfig:
        maximumRingSize: "200"
        minimumRingSize: "10"
{{< /highlight >}}

#### Configure the route

Now configure your route to produce hash keys based on a cookie. Update the route with `kubectl edit virtualservice -n gloo-system default` and apply the changes shown below.

{{< highlight yaml "hl_lines=20-25" >}}
apiVersion: gateway.solo.io/v1
kind: VirtualService
metadata:
  name: default
  namespace: gloo-system
spec:
  virtualHost:
    domains:
    - '*'
    routes:
    - matchers:
       - exact: /route1
      routeAction:
        single:
          upstream:
            name: default-session-affinity-app-80
            namespace: gloo-system
      options:
        lbHash:
          hashPolicies:
          - cookie:
              name: gloo
              path: /abc
              ttl: 10s
        prefixRewrite: /count
{{< /highlight >}}

Return to the app in your browser and refresh the page a few times. You should see an increasing count similar to this:

```
5,6,7,8,...
```

Now that you have configured cookie-based sticky sessions, web requests from your browser will be served by the same instance of the counter app (unless you delete the cookie).
