---
title: Service Failover
description: Creating a failover service in Gloo Federation
weight: 30
---

[INSERT INTRO TEXT]

## Prerequisites

To successfully follow this Service Failover guide, you will need the following software available and configured on your system.

Docker - Runs the containers for kind and all pods inside the clusters
Kubectl - Used to execute commands against the clusters
Kind - Deploys two Kubernetes clusters using containers running on Docker
Helm - Used to deploy the Gloo Federation and Gloo charts
Glooctl - Used to register the Kubernetes clusters with Gloo Federation
openssl  - Generates certificates to enable mTLS between multiple Gloo instances

In addition you will also need to have completed the steps in the Getting Started guide or the Cluster Registration guide, as it is assumed that certain components have already been installed and configured.

## Configure Gloo for Failover

The first step to enabling failover is security. As failover allows communication between multiple clusters, it is crucial that the traffic be encrypted. Therefore certificates need to be provisioned and placed in the clusters to allow for mTLS between the Gloo instances running on separate clusters. 

{{< notice note >}}
If you deployed your Gloo Federation installation using the `glooctl federation demo` command, these certificates and secrets have already been configured and the necessary gateway and service have been created. You can skip to the [next section](#deploy-our-sample-application) where we deploy an application to demonstrate the failover.
{{< /notice >}}

### Create the certificates and secrets for mTLS

The following two commands will generate all of the certs necessary.

```
# Generate downstream cert and key
openssl req -x509 -nodes -days 365 -newkey rsa:2048 \
  -keyout tls.key -out tls.crt -subj "/CN=solo.io"

# Generate upstream ca cert and key
openssl req -x509 -nodes -days 365 -newkey rsa:2048 \
  -keyout mtls.key -out mtls.crt -subj "/CN=solo.io"
```

Once the certificates have been generated, we can place them in the cluster as Secrets so that Gloo can access them.

```
# Set the name of the local and remote cluster contexts
REMOTE_CLUSTER_CONTEXT=kind-remote
LOCAL_CLUSTER_CONTEXT=kind-local

# Set context to remote cluster
kubectl config use-context $REMOTE_CLUSTER_CONTEXT

# Create the secret
glooctl create secret tls --name failover-downstream \
--certchain tls.crt --privatekey tls.key --rootca mtls.crt

# Set the context to the local cluster
kubectl config use-context $LOCAL_CLUSTER_CONTEXT

# Create the secret
glooctl create secret tls --name failover-upstream \
--certchain mtls.crt --privatekey mtls.key
```

### Create the failover gateway

In order to use a Gloo Instance as a failover target it first needs to be configured with an additional listener to route incoming failover requests.

The Gateway resource below sets up a TCP proxy which is configured to terminate mTLS traffic from the primary gloo instance, and forward the traffic based on the SNI name. The SNI name and routing are automatically handled by Gloo Federation, but the certificates are the ones created in the previous step.

The service creates an externally addressable way of communicating with the Gloo instance in question. This service may look different for different setups, but in the local KinD setup it needs to be a NodePort service on the specified port. Gloo Federation will automatically discover all external addresses for any Gloo instance.

```yaml
# Apply failover gateway and service
kubectl apply -f - <<EOF
apiVersion: gateway.solo.io/v1
kind: Gateway
metadata:
 name: failover-gateway
 namespace: gloo-system
 labels:
   app: gloo
spec:
 bindAddress: "::"
 bindPort: 15443
 tcpGateway:
   tcpHosts:
   - name: failover
     sslConfig:
       secretRef:
         name: failover-downstream
         namespace: gloo-system
     destination:
       forwardSniClusterName: {}

---
apiVersion: v1
kind: Service
metadata:
 labels:
   app: gloo
   gateway-proxy-id: gateway-proxy
   gloo: gateway-proxy
 name: failover
 namespace: gloo-system
spec:
 ports:
 - name: failover
   nodePort: 32000
   port: 15443
   protocol: TCP
   targetPort: 15443
 selector:
   gateway-proxy: live
   gateway-proxy-id: gateway-proxy
 sessionAffinity: None
 type: NodePort
EOF
```

## Deploy our Sample Application

In order to demonstrate Gloo multi cluster failover features we will create a relatively contrived example which can very easily show off the power of the feature.

This example assumes that the `glooctl federation demo init` command has already been run, and the two kind clusters it produces are already configured.

This application consists of two simple workloads which just return a color. The workload in the local cluster returns the color “blue”, and the workload in the remote cluster returns the color “green”. Each workload also has a healthcheck endpoint running at “/health” which can be manually made to fail for demonstration purposes.

The first step is too apply the application by running the two following commands:

Create Local Color App (BLUE)

```shell script
# Set the LOCAL_CLUSTER_CONTEXT value if you haven't already
LOCAL_CLUSTER_CONTEXT=kind-local

kubectl apply --context $LOCAL_CLUSTER_CONTEXT -f - <<EOF
apiVersion: v1
kind: Service
metadata:
 labels:
   app: bluegreen
   text: blue
 name: service-blue
 namespace: default
spec:
 ports:
 - name: color
   port: 10000
   protocol: TCP
   targetPort: 10000
 selector:
   app: bluegreen
   text: blue
 sessionAffinity: None
 type: ClusterIP
---
apiVersion: apps/v1
kind: Deployment
metadata:
 labels:
   app: bluegreen
   text: blue
 name: echo-blue
 namespace: default
spec:
 progressDeadlineSeconds: 600
 replicas: 1
 revisionHistoryLimit: 10
 selector:
   matchLabels:
     app: bluegreen
     text: blue
 strategy:
   rollingUpdate:
     maxSurge: 25%
     maxUnavailable: 25%
   type: RollingUpdate
 template:
   metadata:
     creationTimestamp: null
     labels:
       app: bluegreen
       text: blue
   spec:
     containers:
     - args:
       - -text="blue-pod"
       image: hashicorp/http-echo@sha256:ba27d460cd1f22a1a4331bdf74f4fccbc025552357e8a3249c40ae216275de96
       imagePullPolicy: IfNotPresent
       name: echo
       resources: {}
       terminationMessagePath: /dev/termination-log
       terminationMessagePolicy: File
     - args:
       - --config-yaml
       - |2

         node:
          cluster: ingress
          id: "ingress~for-testing"
          metadata:
           role: "default~proxy"
         static_resources:
           listeners:
           - name: listener_0
             address:
               socket_address: { address: 0.0.0.0, port_value: 10000 }
             filter_chains:
             - filters:
               - name: envoy.filters.network.http_connection_manager
                 typed_config:
                   "@type": type.googleapis.com/envoy.extensions.filters.network.http_connection_manager.v3.HttpConnectionManager
                   stat_prefix: ingress_http
                   codec_type: AUTO
                   route_config:
                     name: local_route
                     virtual_hosts:
                     - name: local_service
                       domains: ["*"]
                       routes:
                       - match: { prefix: "/" }
                         route: { cluster: some_service }
                   http_filters:
                   - name: envoy.filters.http.health_check
                     typed_config:
                       "@type": type.googleapis.com/envoy.extensions.filters.http.health_check.v3.HealthCheck
                       pass_through_mode: true
                   - name: envoy.filters.http.router
           clusters:
           - name: some_service
             connect_timeout: 0.25s
             type: STATIC
             lb_policy: ROUND_ROBIN
             load_assignment:
               cluster_name: some_service
               endpoints:
               - lb_endpoints:
                 - endpoint:
                     address:
                       socket_address:
                         address: 0.0.0.0
                         port_value: 5678
         admin:
           access_log_path: /dev/null
           address:
             socket_address:
               address: 0.0.0.0
               port_value: 19000
       - --disable-hot-restart
       - --log-level
       - debug
       - --concurrency
       - "1"
       - --file-flush-interval-msec
       - "10"
       image: envoyproxy/envoy:v1.14.2
       imagePullPolicy: IfNotPresent
       name: envoy
       resources: {}
       terminationMessagePath: /dev/termination-log
       terminationMessagePolicy: File
     dnsPolicy: ClusterFirst
     restartPolicy: Always
     schedulerName: default-scheduler
     securityContext: {}
     terminationGracePeriodSeconds: 0
EOF
```


```shell script
# Set the REMOTE_CLUSTER_CONTEXT value if you haven't already
REMOTE_CLUSTER_CONTEXT=kind-remote

kubectl apply --context $REMOTE_CLUSTER_CONTEXT -f - <<EOF
apiVersion: v1
kind: Service
metadata:
 labels:
   app: bluegreen
 name: service-green
 namespace: default
spec:
 ports:
 - name: color
   port: 10000
   protocol: TCP
   targetPort: 10000
 selector:
   app: bluegreen
   text: green
 sessionAffinity: None
 type: ClusterIP

---
apiVersion: apps/v1
kind: Deployment
metadata:
 labels:
   app: bluegreen
   text: green
 name: echo-green
 namespace: default
spec:
 progressDeadlineSeconds: 600
 replicas: 1
 revisionHistoryLimit: 10
 selector:
   matchLabels:
     app: bluegreen
     text: green
 strategy:
   rollingUpdate:
     maxSurge: 25%
     maxUnavailable: 25%
   type: RollingUpdate
 template:
   metadata:
     creationTimestamp: null
     labels:
       app: bluegreen
       text: green
   spec:
     containers:
     - args:
       - -text="green-pod"
       image: hashicorp/http-echo@sha256:ba27d460cd1f22a1a4331bdf74f4fccbc025552357e8a3249c40ae216275de96
       imagePullPolicy: IfNotPresent
       name: echo
       resources: {}
       terminationMessagePath: /dev/termination-log
       terminationMessagePolicy: File
     - args:
       - --config-yaml
       - |2

         node:
          cluster: ingress
          id: "ingress~for-testing"
          metadata:
           role: "default~proxy"
         static_resources:
           listeners:
           - name: listener_0
             address:
               socket_address: { address: 0.0.0.0, port_value: 10000 }
             filter_chains:
             - filters:
               - name: envoy.filters.network.http_connection_manager
                 typed_config:
                   "@type": type.googleapis.com/envoy.extensions.filters.network.http_connection_manager.v3.HttpConnectionManager
                   stat_prefix: ingress_http
                   codec_type: AUTO
                   route_config:
                     name: local_route
                     virtual_hosts:
                     - name: local_service
                       domains: ["*"]
                       routes:
                       - match: { prefix: "/" }
                         route: { cluster: some_service }
                   http_filters:
                   - name: envoy.filters.http.health_check
                     typed_config:
                       "@type": type.googleapis.com/envoy.extensions.filters.http.health_check.v3.HealthCheck
                       pass_through_mode: true
                   - name: envoy.filters.http.router
           clusters:
           - name: some_service
             connect_timeout: 0.25s
             type: STATIC
             lb_policy: ROUND_ROBIN
             load_assignment:
               cluster_name: some_service
               endpoints:
               - lb_endpoints:
                 - endpoint:
                     address:
                       socket_address:
                         address: 0.0.0.0
                         port_value: 5678
         admin:
           access_log_path: /dev/null
           address:
             socket_address:
               address: 0.0.0.0
               port_value: 19000
       - --disable-hot-restart
       - --log-level
       - debug
       - --concurrency
       - "1"
       - --file-flush-interval-msec
       - "10"
       image: envoyproxy/envoy:v1.14.2
       imagePullPolicy: IfNotPresent
       name: envoy
       resources: {}
       terminationMessagePath: /dev/termination-log
       terminationMessagePolicy: File
     dnsPolicy: ClusterFirst
     restartPolicy: Always
     schedulerName: default-scheduler
     securityContext: {}
     terminationGracePeriodSeconds: 0
EOF
```

Now that we have our two applications up and running, we can configure health checks and the failover resource.

## Configure Failover Through Gloo-Fed

A major part of failover is health checking. In order for Envoy to determine the state of the primary of failover endpoints, health checking must be enabled. In this section we will specify a health check for the Blue instance of the application and create the failover configuration.

First, let's add health checks to the blue Upstream:

```shell script
kubectl patch --context $LOCAL_CLUSTER_CONTEXT upstream -n gloo-system default-service-blue-10000 --type=merge -p "
spec:
 healthChecks:
 - timeout: 1s
   interval: 1s
   unhealthyThreshold: 1
   healthyThreshold: 1
   httpHealthCheck:
     path: /health
"
```

Once health checking has been enabled we can go ahead and actually create our FailoverScheme resource. This is the Gloo Federation resource which will dynamically configure failover from one root Upstream, to a set prioritized Upstreams.

We will create the FailoverScheme resource in the `gloo-fed` namespace:

```shell script
kubectl apply --context $LOCAL_CLUSTER_CONTEXT -f - <<EOF
apiVersion: fed.solo.io/v1
kind: FailoverScheme
metadata:
 name: failover-scheme
 namespace: gloo-fed
spec:
 failoverGroups:
 - priorityGroup:
   - cluster: $REMOTE_CLUSTER_CONTEXT
     upstreams:
     - name: default-service-green-10000
       namespace: gloo-system
 primary:
   clusterName: $LOCAL_CLUSTER_CONTEXT
   name: default-service-blue-10000
   namespace: gloo-system
EOF
```


Create simple route

```shell script
# Make sure the context is set to the local cluster
kubectl config use-context $LOCAL_CLUSTER_CONTEXT

glooctl add route \
     --path-prefix / \
     --dest-name default-service-blue-10000
```


## Test That Everything Works

Test that local endpoint is in use while healthy

```shell script
curl -v http://localhost:8080

*   Trying ::1...
* TCP_NODELAY set
* Connected to localhost (::1) port 8080 (#0)
> GET / HTTP/1.1
> Host: localhost:8080
> User-Agent: curl/7.64.1
> Accept: */*
>
Handling connection for 8080
< HTTP/1.1 200 OK
< x-app-name: http-echo
< x-app-version: 0.2.3
< date: Wed, 08 Jul 2020 17:35:53 GMT
< content-length: 11
< content-type: text/plain; charset=utf-8
< x-envoy-upstream-service-time: 1
< x-envoy-upstream-healthchecked-cluster: ingress
< server: envoy
<
"blue-pod"
* Connection #0 to host localhost left intact
* Closing connection 0
```


Fail Health Checks in the local service to begin failing over to remote service

```shell script
# optionally run in bakground using &
kubectl port-forward deploy/echo-blue 19000
# switch to new shell
curl -v -X POST  http://localhost:19000/healthcheck/fail
```

Test gloo route again to ensure it is now serving failover endpoint
```shell script
$ curl -v http://localhost:8080
*   Trying ::1...
* TCP_NODELAY set
* Connected to localhost (::1) port 8080 (#0)
> GET / HTTP/1.1
> Host: localhost:8080
> User-Agent: curl/7.64.1
> Accept: */*
>
Handling connection for 8080
< HTTP/1.1 200 OK
< x-app-name: http-echo
< x-app-version: 0.2.3
< date: Wed, 08 Jul 2020 18:19:20 GMT
< content-length: 12
< content-type: text/plain; charset=utf-8
< x-envoy-upstream-service-time: 1
< x-envoy-upstream-healthchecked-cluster: ingress
< server: envoy
<
"green-pod"
* Connection #0 to host localhost left intact
* Closing connection 0
```

## Cleanup
TODO
