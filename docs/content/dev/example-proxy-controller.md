---
title: "Building a Proxy Controller for Gloo"
weight: 2
---

In this tutorial, we're going to show how to use Gloo's Proxy API to build a router which automatically creates 
routes for every existing kubernetes service,

## Writing the Code

You can view the complete code written in this section here: [example-proxy-controller.go](../example-proxy-controller.go).

### Initial code

First, we'll start with a `main.go`. We'll use the main function to connect to 
Kubernetes and start an event loop. Start by creating a new `main.go` file in a new directory:

```go
package main

// all the import's we'll need for this controller
import (
	"context"
	"log"
	"os"
	"time"

	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/go-utils/kubeutils"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/factory"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/kube"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"

	// import for GKE
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
)


func main() {}


// make our lives easy
func must(err error) {
	if err != nil {
		panic(err)
	}
}

```

### Gloo API Clients

Then we'll want to use Gloo's libraries to initialize a client for Proxies and Upstreams. Add the following function to your code:

```go

func initGlooClients(ctx context.Context) (v1.UpstreamClient, v1.ProxyClient) {
	// root rest config
	restConfig, err := kubeutils.GetConfig(
		os.Getenv("KUBERNETES_MASTER_URL"),
		os.Getenv("KUBECONFIG"))
	must(err)

	// wrapper for kubernetes shared informer factory
	cache := kube.NewKubeCache(ctx)

	// initialize the CRD client for Gloo Upstreams
	upstreamClient, err := v1.NewUpstreamClient(&factory.KubeResourceClientFactory{
		Crd:         v1.UpstreamCrd,
		Cfg:         restConfig,
		SharedCache: cache,
	})
	must(err)

	// registering the client registers the type with the client cache
	err = upstreamClient.Register()
	must(err)

	// initialize the CRD client for Gloo Proxies
	proxyClient, err := v1.NewProxyClient(&factory.KubeResourceClientFactory{
		Crd:         v1.ProxyCrd,
		Cfg:         restConfig,
		SharedCache: cache,
	})
	must(err)

	// registering the client registers the type with the client cache
	err = proxyClient.Register()
	must(err)

	return upstreamClient, proxyClient
}

```

This function will initialize clients for interacting with Gloo's Upstream and Proxy APIs. 

### Proxy Configuration

Next, we'll define the algorithm for generating Proxy CRDs from a given list of upstreams. In this example, our 
proxy will serve traffic to every service in our cluster. 

Paste the following function into your code. Feel free to modify if you want to get experimental, here's where the 
"opinionated" piece of our controller is defined:

```go

// in this function we'll generate an opinionated
// proxy object with a routes for each of our upstreams
func makeDesiredProxy(upstreams v1.UpstreamList) *v1.Proxy {

	// each virtual host represents the table of routes for a given
	// domain or set of domains.
	// in this example, we'll create one virtual host
	// for each upstream.
	var virtualHosts []*v1.VirtualHost

	for _, upstream := range upstreams {

		// create a virtual host for each upstream
		vHostForUpstream := &v1.VirtualHost{
			// logical name of the virtual host, should be unique across vhosts
			Name: upstream.Metadata.Name,

			// the domain will be our "matcher".
			// requests with the Host header equal to the upstream name
			// will be routed to this upstream
			Domains: []string{upstream.Metadata.Name},

			// we'll create just one route designed to match any request
			// and send it to the upstream for this domain
			Routes: []*v1.Route{{
				// use a basic catch-all matcher
				Matcher: &matchers.Matcher{
					PathSpecifier: &matchers.Matcher_Prefix{
						Prefix: "/",
					},
				},

				// tell Gloo where to send the requests
				Action: &v1.Route_RouteAction{
					RouteAction: &v1.RouteAction{
						Destination: &v1.RouteAction_Single{
							// single destination
							Single: &v1.Destination{
								// a "reference" to the upstream, which is a Namespace/Name tuple
								Upstream: upstream.Metadata.Ref(),
							},
						},
					},
				},
			}},
		}

		virtualHosts = append(virtualHosts, vHostForUpstream)
	}

	desiredProxy := &v1.Proxy{
		// metadata will be translated to Kubernetes ObjectMeta
		Metadata: core.Metadata{Namespace: "gloo-system", Name: "my-cool-proxy"},

		// we have the option of creating multiple listeners,
		// but for the purpose of this example we'll just use one
		Listeners: []*v1.Listener{{
			// logical name for the listener
			Name: "my-amazing-listener",

			// instruct envoy to bind to all interfaces on port 8080
			BindAddress: "::", BindPort: 8080,

			// at this point you determine what type of listener
			// to use. here we'll be using the HTTP Listener
			// other listener types are currently unsupported,
			// but future
			ListenerType: &v1.Listener_HttpListener{
				HttpListener: &v1.HttpListener{
					// insert our list of virtual hosts here
					VirtualHosts: virtualHosts,
				},
			}},
		},
	}

	return desiredProxy
}


``` 

### Event Loop

Now we'll define a `resync` function to be called whenever we receive a new list of upstreams:

```go

// we received a new list of upstreams! regenerate the desired proxy
// and write it as a CRD to Kubernetes
func resync(ctx context.Context, upstreams v1.UpstreamList, client v1.ProxyClient) {
	desiredProxy := makeDesiredProxy(upstreams)

	// see if the proxy exists. if yes, update; if no, create
	existingProxy, err := client.Read(
		desiredProxy.Metadata.Namespace,
		desiredProxy.Metadata.Name,
		clients.ReadOpts{Ctx: ctx})


	// proxy exists! this is an update, not a create
	if err == nil {

		// sleep for 1s as Gloo may be re-validating our proxy, which can cause resource version to change
		time.Sleep(time.Second)

		// ensure resource version is the latest
		existingProxy, err = client.Read(
			desiredProxy.Metadata.Namespace,
			desiredProxy.Metadata.Name,
			clients.ReadOpts{Ctx: ctx})
		must(err)

		// update the resource version on our desired proxy
		desiredProxy.Metadata.ResourceVersion = existingProxy.Metadata.ResourceVersion
	}

	// write!
	written, err := client.Write(desiredProxy,
		clients.WriteOpts{Ctx: ctx, OverwriteExisting: true})

	must(err)

	log.Printf("wrote proxy object: %+v\n", written)
}


```

### Main Function

Now that we have our clients and a function defining the proxies we'll want to create, all we need to do is tie it 
all together.

Let's set up a loop to watch Upstreams in our main function. Add the following to your `main()` func:

```go

func main() {
	// root context for the whole thing
	ctx := context.Background()

	// initialize Gloo API clients
	upstreamClient, proxyClient := initGlooClients(ctx)

	// start a watch on upstreams. we'll use this as our trigger
	// whenever upstreams are modified, we'll trigger our sync function
	upstreamWatch, watchErrors, initError := upstreamClient.Watch("gloo-system",
		clients.WatchOpts{Ctx: ctx})
	must(initError)

	// our "event loop". an event occurs whenever the list of upstreams has been updated
	for {
		select {
		// if we error during watch, just exit
		case err := <-watchErrors:
			must(err)
		// process a new upstream list
		case newUpstreamList := <-upstreamWatch:
			// we received a new list of upstreams from our watch, 
			resync(ctx, newUpstreamList, proxyClient)
		}
	}
}

```   


### Finished Code

Great! Here's what our completed main file should look like:


```go
package main

import (
	"context"
	"log"
	"os"
	"time"

	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/go-utils/kubeutils"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/factory"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/kube"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"

	// import for GKE
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
)

func main() {
	// root context for the whole thing
	ctx := context.Background()

	// initialize Gloo API clients, built on top of CRDs
	upstreamClient, proxyClient := initGlooClients(ctx)

	// start a watch on upstreams. we'll use this as our trigger
	// whenever upstreams are modified, we'll trigger our sync function
	upstreamWatch, watchErrors, initError := upstreamClient.Watch("gloo-system",
		clients.WatchOpts{Ctx: ctx})
	must(initError)

	// our "event loop". an event occurs whenever the list of upstreams has been updated
	for {
		select {
		// if we error during watch, just exit
		case err := <-watchErrors:
			must(err)
		// process a new upstream list
		case newUpstreamList := <-upstreamWatch:
			resync(ctx, newUpstreamList, proxyClient)
		}
	}
}

func initGlooClients(ctx context.Context) (v1.UpstreamClient, v1.ProxyClient) {
	// root rest config
	restConfig, err := kubeutils.GetConfig(
		os.Getenv("KUBERNETES_MASTER_URL"),
		os.Getenv("KUBECONFIG"))
	must(err)

	// wrapper for kubernetes shared informer factory
	cache := kube.NewKubeCache(ctx)

	// initialize the CRD client for Gloo Upstreams
	upstreamClient, err := v1.NewUpstreamClient(&factory.KubeResourceClientFactory{
		Crd:         v1.UpstreamCrd,
		Cfg:         restConfig,
		SharedCache: cache,
	})
	must(err)

	// registering the client registers the type with the client cache
	err = upstreamClient.Register()
	must(err)

	// initialize the CRD client for Gloo Proxies
	proxyClient, err := v1.NewProxyClient(&factory.KubeResourceClientFactory{
		Crd:         v1.ProxyCrd,
		Cfg:         restConfig,
		SharedCache: cache,
	})
	must(err)

	// registering the client registers the type with the client cache
	err = proxyClient.Register()
	must(err)

	return upstreamClient, proxyClient
}

// we received a new list of upstreams! regenerate the desired proxy
// and write it as a CRD to Kubernetes
func resync(ctx context.Context, upstreams v1.UpstreamList, client v1.ProxyClient) {
	desiredProxy := makeDesiredProxy(upstreams)

	// see if the proxy exists. if yes, update; if no, create
	existingProxy, err := client.Read(
		desiredProxy.Metadata.Namespace,
		desiredProxy.Metadata.Name,
		clients.ReadOpts{Ctx: ctx})


	// proxy exists! this is an update, not a create
	if err == nil {

		// sleep for 1s as Gloo may be re-validating our proxy, which can cause resource version to change
		time.Sleep(time.Second)

		// ensure resource version is the latest
		existingProxy, err = client.Read(
			desiredProxy.Metadata.Namespace,
			desiredProxy.Metadata.Name,
			clients.ReadOpts{Ctx: ctx})
		must(err)

		// update the resource version on our desired proxy
		desiredProxy.Metadata.ResourceVersion = existingProxy.Metadata.ResourceVersion
	}

	// write!
	written, err := client.Write(desiredProxy,
		clients.WriteOpts{Ctx: ctx, OverwriteExisting: true})

	must(err)

	log.Printf("wrote proxy object: %+v\n", written)
}

// in this function we'll generate an opinionated
// proxy object with a routes for each of our upstreams
func makeDesiredProxy(upstreams v1.UpstreamList) *v1.Proxy {

	// each virtual host represents the table of routes for a given
	// domain or set of domains.
	// in this example, we'll create one virtual host
	// for each upstream.
	var virtualHosts []*v1.VirtualHost

	for _, upstream := range upstreams {

		// create a virtual host for each upstream
		vHostForUpstream := &v1.VirtualHost{
			// logical name of the virtual host, should be unique across vhosts
			Name: upstream.Metadata.Name,

			// the domain will be our "matcher".
			// requests with the Host header equal to the upstream name
			// will be routed to this upstream
			Domains: []string{upstream.Metadata.Name},

			// we'll create just one route designed to match any request
			// and send it to the upstream for this domain
			Routes: []*v1.Route{{
				// use a basic catch-all matcher
				Matcher: &matchers.Matcher{
					PathSpecifier: &matchers.Matcher_Prefix{
						Prefix: "/",
					},
				},

				// tell Gloo where to send the requests
				Action: &v1.Route_RouteAction{
					RouteAction: &v1.RouteAction{
						Destination: &v1.RouteAction_Single{
							// single destination
							Single: &v1.Destination{
								// a "reference" to the upstream, which is a Namespace/Name tuple
								Upstream: upstream.Metadata.Ref(),
							},
						},
					},
				},
			}},
		}

		virtualHosts = append(virtualHosts, vHostForUpstream)
	}

	desiredProxy := &v1.Proxy{
		// metadata will be translated to Kubernetes ObjectMeta
		Metadata: core.Metadata{Namespace: "gloo-system", Name: "my-cool-proxy"},

		// we have the option of creating multiple listeners,
		// but for the purpose of this example we'll just use one
		Listeners: []*v1.Listener{{
			// logical name for the listener
			Name: "my-amazing-listener",

			// instruct envoy to bind to all interfaces on port 8080
			BindAddress: "::", BindPort: 8080,

			// at this point you determine what type of listener
			// to use. here we'll be using the HTTP Listener
			// other listener types are currently unsupported,
			// but future
			ListenerType: &v1.Listener_HttpListener{
				HttpListener: &v1.HttpListener{
					// insert our list of virtual hosts here
					VirtualHosts: virtualHosts,
				},
			}},
		},
	}

	return desiredProxy
}

// make our lives easy
func must(err error) {
	if err != nil {
		panic(err)
	}
}


```

### Run

In order to run this file, you'll need to pull the dependencies into your local workspace. `go get -v ./...` from your 
working dir, or `dep init` (if you're comfortable using [`dep`](https://github.com/golang/dep)) should work fine.

```bash
dep init -v

# or

go get -v ./..

```

While it's possible to package this application in a Docker container and deploy it as a pod inside of Kubernetes, let's 
just try running it locally. [Make sure you have Gloo installed](/installation) in your cluster so 
that Discovery will create some Upstreams for us.

Once that's done, to see our code in action, simply run `go run main.go` !

```bash
go run main.go

2019/02/11 11:27:30 wrote proxy object: listeners:<name:"my-amazing-listener" bind_address:"::" bind_port:8080 http_listener:<virtual_hosts:<name:"default-kubernetes-443" domains:"default-kubernetes-443" routes:<matcher:<prefix:"/" > route_action:<single:<upstream:<name:"default-kubernetes-443" namespace:"gloo-system" > > > > > virtual_hosts:<name:"gloo-system-gateway-proxy-8080" domains:"gloo-system-gateway-proxy-8080" routes:<matcher:<prefix:"/" > route_action:<single:<upstream:<name:"gloo-system-gateway-proxy-8080" namespace:"gloo-system" > > > > > virtual_hosts:<name:"gloo-system-gloo-9977" domains:"gloo-system-gloo-9977" routes:<matcher:<prefix:"/" > route_action:<single:<upstream:<name:"gloo-system-gloo-9977" namespace:"gloo-system" > > > > > virtual_hosts:<name:"kube-system-kube-dns-53" domains:"kube-system-kube-dns-53" routes:<matcher:<prefix:"/" > route_action:<single:<upstream:<name:"kube-system-kube-dns-53" namespace:"gloo-system" > > > > > virtual_hosts:<name:"kube-system-tiller-deploy-44134" domains:"kube-system-tiller-deploy-44134" routes:<matcher:<prefix:"/" > route_action:<single:<upstream:<name:"kube-system-tiller-deploy-44134" namespace:"gloo-system" > > > > > > > status:<> metadata:<name:"my-cool-proxy" namespace:"gloo-system" resource_version:"455073" > 

```

Neat! Our proxy got created. We can view it with `kubectl`:

```bash
kubectl get proxy -n gloo-system -o yaml


apiVersion: v1
items:
- apiVersion: gloo.solo.io/v1
  kind: Proxy
  metadata:
    creationTimestamp: 2019-02-11T16:27:30Z
    generation: 1
    name: my-cool-proxy
    namespace: gloo-system
    resourceVersion: "455074"
    selfLink: /apis/gloo.solo.io/v1/namespaces/gloo-system/proxies/my-cool-proxy
    uid: eda0ba6f-2e19-11e9-b401-c075ea19232f
  spec:
    listeners:
    - bindAddress: '::'
      bindPort: 8080
      httpListener:
        virtualHosts:
        - domains:
          - default-kubernetes-443
          name: default-kubernetes-443
          routes:
          - matcher:
              prefix: /
            routeAction:
              single:
                upstream:
                  name: default-kubernetes-443
                  namespace: gloo-system
        - domains:
          - gloo-system-gateway-proxy-8080
          name: gloo-system-gateway-proxy-8080
          routes:
          - matcher:
              prefix: /
            routeAction:
              single:
                upstream:
                  name: gloo-system-gateway-proxy-8080
                  namespace: gloo-system
        - domains:
          - gloo-system-gloo-9977
          name: gloo-system-gloo-9977
          routes:
          - matcher:
              prefix: /
            routeAction:
              single:
                upstream:
                  name: gloo-system-gloo-9977
                  namespace: gloo-system
        - domains:
          - kube-system-kube-dns-53
          name: kube-system-kube-dns-53
          routes:
          - matcher:
              prefix: /
            routeAction:
              single:
                upstream:
                  name: kube-system-kube-dns-53
                  namespace: gloo-system
        - domains:
          - kube-system-tiller-deploy-44134
          name: kube-system-tiller-deploy-44134
          routes:
          - matcher:
              prefix: /
            routeAction:
              single:
                upstream:
                  name: kube-system-tiller-deploy-44134
                  namespace: gloo-system
      name: my-amazing-listener
  status:
    reported_by: gloo
    state: 1
kind: List
metadata:
  resourceVersion: ""
  selfLink: ""


```

Cool. Let's leave our controller running and watch it dynamically respond when we add a service to our cluster:

```bash
kubectl apply -f \
  https://raw.githubusercontent.com/solo-io/gloo/v1.2.9/example/petstore/petstore.yaml
```

See the service and pod:

```bash
kubectl get pod -n default && kubectl get svc -n default

NAME                      READY     STATUS    RESTARTS   AGE
petstore-6fd84bc9-zdskz   1/1       Running   0          5s
NAME         TYPE        CLUSTER-IP      EXTERNAL-IP   PORT(S)    AGE
kubernetes   ClusterIP   10.96.0.1       <none>        443/TCP    6d
petstore     ClusterIP   10.109.34.250   <none>        8080/TCP   5s

```

The upstream that was created:

```bash
kubectl get upstream

NAME                              AGE
default-kubernetes-443            2m
default-petstore-8080             46s # <- this one's new
gloo-system-gateway-proxy-8080    2m
gloo-system-gloo-9977             2m
kube-system-kube-dns-53           2m
kube-system-tiller-deploy-44134   2m
```

And check that our proxy object was updated:

```bash
kubectl get proxy -n gloo-system -o yaml

apiVersion: v1
items:
- apiVersion: gloo.solo.io/v1
  kind: Proxy
  metadata:
    creationTimestamp: 2019-02-11T19:03:48Z
    generation: 1
    name: my-cool-proxy
    namespace: gloo-system
    resourceVersion: "470446"
    selfLink: /apis/gloo.solo.io/v1/namespaces/gloo-system/proxies/my-cool-proxy
    uid: c2f058fb-2e2f-11e9-b401-c075ea19232f
  spec:
    listeners:
    - bindAddress: '::'
      bindPort: 8080
      httpListener:
        virtualHosts:
        ...
        - domains:
          - default-petstore-8080
          name: default-petstore-8080
          routes:
          - matcher:
              prefix: /
            routeAction:
              single:
                upstream:
                  name: default-petstore-8080
                  namespace: gloo-system
        ...
      name: my-amazing-listener
  status:
    reported_by: gloo
    state: 1
kind: List
metadata:
  resourceVersion: ""
  selfLink: ""

```

The proxy should have been create with the `default-petstore-8080` virtualHost.

Now that we have a proxy called `my-cool-proxy`, Gloo will be serving xDS configuration that matches this proxy CRD.
However, we don't actually have an Envoy instance deployed that will receive this config. In the next section, 
we'll walk through the steps to deploy an Envoy pod wired to receive config from Gloo, identifying itself as 
`my-cool-proxy`.  


## Deploying Envoy to Kubernetes

Gloo comes pre-installed with at least one proxy depending on your setup: the `gateway-proxy`. This proxy is configured 
by the `gateway` proxy controller. It's not very different from the controller we just wrote!

We'll need to deploy another proxy that will register to Gloo with it's `role` configured to match the name of our proxy 
CRD, `my-cool-proxy`. Let's do it!

### Creating the ConfigMap

Envoy needs a ConfigMap which points it at Gloo as its configuration server. Run the following command to create 
the configmap you'll need:


```bash
cat << EOF | kubectl apply -f -
apiVersion: v1
kind: ConfigMap
metadata:
  name: my-cool-envoy-config
  namespace: default
data:
  envoy.yaml: |
    node:
      cluster: "1"
      id: "1"
      metadata:

        # this line is what connects this envoy instance to our Proxy crd
        role: "gloo-system~my-cool-proxy"

    static_resources:
      clusters:
      - name: xds_cluster
        connect_timeout: 5.000s
        load_assignment:
          cluster_name: xds_cluster
          endpoints:
          - lb_endpoints:
            - endpoint:
                address:
                  socket_address:

                    # here's where we provide the hostname of the gloo service
                    address: gloo.gloo-system.svc.cluster.local

                    port_value: 9977
        http2_protocol_options: {}
        type: STRICT_DNS
    dynamic_resources:
      ads_config:
        api_type: GRPC
        grpc_services:
        - envoy_grpc: {cluster_name: xds_cluster}
      cds_config:
        ads: {}
      lds_config:
        ads: {}
    admin:
      access_log_path: /dev/null
      address:
        socket_address:
          address: 127.0.0.1
          port_value: 19000
EOF
```

Note that this will create the configmap in the `default` namespace, but you can run it anywhere. Just make sure 
the proxy deployment and service all go to the same namespace.


### Creating the Service and Deployment

We need to create a `LoadBalancer` service for our proxy so we can connect to it from the outside. Note that 
if you're using a Kubernetes Cluster without an external load balancer (e.g. minikube), we'll be using the service's
`NodePort` to connect.

Run the following command to create the service:

```bash
cat << EOF | kubectl apply -f -
apiVersion: v1
kind: Service
metadata:
  labels:
    gloo: my-cool-proxy
  name: my-cool-proxy
  namespace: default
spec:
  ports:
  - port: 8080 # <- this port should match the port for the HttpListener in our Proxy CRD
    protocol: TCP
    name: http
  selector:
    gloo: my-cool-proxy
  type: LoadBalancer
EOF

```

Finally we'll want to create the deployment itself which will launch a pod with Envoy running inside.

```bash
cat << EOF | kubectl apply -f -
apiVersion: extensions/v1beta1
kind: Deployment
metadata:
  labels:
    gloo: my-cool-proxy
  name: my-cool-proxy
  namespace: default
spec:
  replicas: 
  selector:
    matchLabels:
      gloo: my-cool-proxy
  template:
    metadata:
      labels:
        gloo: my-cool-proxy
    spec:
      containers:
      - args: ["--disable-hot-restart"]
        env:
        - name: POD_NAMESPACE
          valueFrom:
            fieldRef:
              fieldPath: metadata.namespace
        - name: POD_NAME
          valueFrom:
            fieldRef:
              fieldPath: metadata.name
        image: soloio/gloo-envoy-wrapper:0.6.19
        imagePullPolicy: Always
        name: my-cool-proxy
        ports:
        - containerPort: 8080 # <- this port should match the port for the HttpListener in our Proxy CRD
          name: http
          protocol: TCP
        volumeMounts:
        - mountPath: /etc/envoy
          name: envoy-config
      volumes:
      - configMap:
          name: my-cool-envoy-config
        name: envoy-config
EOF
```

If all went well, we should see our pod starting successfully in `default` (or whichever namespace you picked):

```bash
kubectl get pod -n default

NAME                             READY     STATUS    RESTARTS   AGE
my-cool-proxy-7bcb58c87d-h4292   1/1       Running   0          3s
petstore-6fd84bc9-zdskz          1/1       Running   0          48m
```

## Testing the Proxy

If you have `glooctl` installed, we can grab the HTTP endpoint of the proxy with the following command: 

```bash
glooctl proxy url -n default -p my-cool-proxy

http://192.168.99.150:30751
```

Using `curl`, we can connect to any service in our cluster by using the correct `Host` header:

```bash
curl $(glooctl proxy url -n default -p my-cool-proxy)/api/pets -H "Host: default-petstore-8080"
```

returns

```json
[{"id":1,"name":"Dog","status":"available"},{"id":2,"name":"Cat","status":"pending"}]
```

Try any `Host` header for any upstream name: 

```bash
kubectl get upstream

NAME                              AGE
default-kubernetes-443            55m
default-my-cool-proxy-8080        5m
default-petstore-8080             53m
gloo-system-gateway-proxy-8080    55m
gloo-system-gloo-9977             54m
kube-system-kube-dns-53           54m
kube-system-tiller-deploy-44134   54m

```

Sweet! You're an official Gloo developer! You've just seen how easy it is to extend Gloo to service one of many 
potential use cases. Take a look at our 
{{< protobuf name="gloo.solo.io.Proxy" display="API Reference Documentation">}} to learn about the 
wide range of configuration options Proxies expose such as request transformation, SSL termination, serverless computing, 
and much more.
