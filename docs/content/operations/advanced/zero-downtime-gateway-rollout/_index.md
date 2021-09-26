---
title: Zero-downtime Gateway rollout
weight: 25
description: Properly configure Gloo Edge and your Load-Balancer to minimize the downtime when bouncing Envoy proxies.
---


## Principles

With distributed systems come reliability patterns that are best to implement.

As services cannot guess the state of their neighborhood, they must implement some mechanisms like health-checks, retries, failover and more.

If you want to know more about theses principles, please watch out this video:
<p style="text-align: center">
<iframe width="560" height="315" src="https://www.youtube.com/embed/xYFx0a0W9_E" title="YouTube video player" frameborder="0" allow="accelerometer; autoplay; clipboard-write; encrypted-media; gyroscope; picture-in-picture" allowfullscreen></iframe>
</p>

To implement these principles, you might want to configure your load-balancer to do accurate health-checks but also the Kubernetes service representing the Envoy proxy and, of course, Envoy itself. 

![Overview](/img/0dt-overview.png)

From the right to the left:
- **B** - Envoy is not _immediately_ aware of the state of the Kubernetes liveness & readiness probes that are set on an upstream API. So, here are two recommendations:
  - the API should start failing health-checks once it receives a SIGTERM signal, and also it should start draining connections gracefully
  - Envoy should be configured with health-checks, retries and outlier detection on these upstreams
- **A** - Depending on your load balancer and network setup, the health-check can reach either the Kubernetes nodes or the Kubernetes pods. Keep in mind these rules of thumb:
  - Cloud LB health-checks to the same node should end in the same pod. You can use either a **DaemonSet** with host port - or you use Kubernetes **affinity** policies to have at most one Envoy proxy on each node + `ExternalTrafficPolicy: local`
  - configure the Health-check filter on Envoy. More details below and also in the dedicated [documentation page](/guides/traffic_management/request_processing/health_checks/). Configure the readiness probe accordingly
  - enable the shutdown hook on the Envoy pods. Configure this hook to fail LB health-checks once it gets a termination signal

This guide shows how to configure these different elements and demonstrates the benefits during a gateway rollout.


## Configuring the Gloo Edge Proxies

### Upstream options

As explained above, it's best having your upstream API to start failing health checks once it receives a termination signal. Talking Envoy side, you can add retries, health-checks and outlier detection as shown below:

```yaml
apiVersion: gloo.solo.io/v1
kind: Upstream
metadata:
  name: default-httpbin-8000
  namespace: gloo-system
spec:
  ...
  # ----- Health Check (a.k.a. active health-checks) -------
  healthChecks:
    - healthyThreshold: 1
      httpHealthCheck:
        path: /status/200
      interval: 2s
      noTrafficInterval: 2s
      timeout: 1s
      unhealthyThreshold: 2
      reuseConnection: false
  # ----- Outlier Detection  (a.k.a. passive health-checks) ------
  outlierDetection:
    consecutive5xx: 3
    maxEjectionPercent: 100
    interval: 10s
  # ----- Help with consistency between the Kubernetes control-plane and the Gloo control-plane ------
  ignoreHealthOnHostRemoval: true
```

Reties are set at the route level:

```yaml
apiVersion: gateway.solo.io/v1
kind: VirtualService
metadata:
  name: httpbin
  namespace: gloo-system
spec:
  virtualHost:
    domains:
      ...
    routes:
    - matchers:
        ...
      routeAction:
        ...
    options:
      # -------- Retries --------
      retries:
        retryOn: 'connect-failure,5xx'
        numRetries: 3
        perTryTimeout: '3s'
```

### Envoy Listener options

First, you want to know when exactly Gloo Edge is ready to route client requests. They are several conditions and the most important one is to have a `VirtualService` correctly configured.

While `glooctl check` will help you to check some fundamentals, this command will not show if the _gateway-proxy_ is actually listening to new connections. Only its internal engine - Envoy - knows about that.

It's fair to quickly remember here that Envoy can be listening to multiple hosts and ports at the same time. For that to happen, you need to define different `Gateways` and `VirtualServices`. If you want to better understand how these objects work together, please check out this [article]({{% versioned_link_path fromRoot="/installation/advanced_configuration/multi-gw-deployment/" %}}).

Once you have these `Gateways` and `VirtualServices` configured, Gloo Edge will generate `Proxy` _Custom Resources_ that will, in turn, generate Envoy **Listeners**, **Routes**, and more. From this point, Envoy is ready to accept new connections. 

The goal here is to know when these [Envoy Listener](https://www.envoyproxy.io/docs/envoy/latest/configuration/listeners/listeners) are actually ready. Luckily, Envoy comes with a handy [Health Check filter](/guides/traffic_management/request_processing/health_checks/) which helps with that.

Example with Helm values:

```yaml
gloo:
  gatewayProxies:
    gatewayProxy:
      gatewaySettings:
        customHttpsGateway:
          options:
            healthCheck:
              # define a custom path that is available when the Gateway (Envoy listener) is actually listening
              path: /envoy-hc
```

## Configuring the Kubernetes probes

As explained above, you need have Envoy to handle gracefully shutdown signals

```yaml
gloo:
  gatewayProxies:
    gatewayProxy:
      podTemplate:
        # graceful shutdown: Envoy will fail health checks but only stop after 7 seconds
        terminationGracePeriodSeconds: 7
        gracefulShutdown:
          enabled: true
          sleepTimeSeconds: 5
        probes: true
        # the gateway-proxy pod is ready only when a Gateway (Envoy listener) is listening
        customReadinessProbe:
          httpGet:
            scheme: HTTPS
            port: 8443
            path: /envoy-hc
          failureThreshold: 2
          initialDelaySeconds: 5
          periodSeconds: 5
```



## Configuring a NLB

In this guide, you will configure an AWS **N**etwork **L**oad **B**alancer. You will need the **AWS Load Balancer Controller** which brings the annotations driven configuration to the next level. More rationales are exposed in this article: [Integration with AWS ELBs]({{% versioned_link_path fromRoot="/guides/integrations/aws/" %}})

The first goal is to correctly configure the LB health-checks:

```yaml
  # Health checks
  service.beta.kubernetes.io/aws-load-balancer-healthcheck-healthy-threshold: "2" # 2-20
  service.beta.kubernetes.io/aws-load-balancer-healthcheck-unhealthy-threshold: "2" # 2-10
  service.beta.kubernetes.io/aws-load-balancer-healthcheck-interval: "10" # 10 or 30
  service.beta.kubernetes.io/aws-load-balancer-healthcheck-path: "/envoy-hc" # Envoy HC filter
  service.beta.kubernetes.io/aws-load-balancer-healthcheck-protocol: "HTTPS"
  service.beta.kubernetes.io/aws-load-balancer-healthcheck-port: "traffic-port"
  service.beta.kubernetes.io/aws-load-balancer-healthcheck-timeout: "6" # 6 is the minimum
```



