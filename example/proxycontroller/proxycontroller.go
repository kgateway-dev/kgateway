// +build ignore

package main

// all the import's we'll need for this controller
import (
	"context"
	"log"
	"os"
	"time"

	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	matchers "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/core/matchers"
	"github.com/solo-io/k8s-utils/kubeutils"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/factory"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/kube"
	core "github.com/solo-io/solo-kit/pkg/api/v1/resources/core"

	// import for GKE
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
)

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

func initGlooClients(ctx context.Context) (v1.UpstreamClient, v1.ProxyClient) {
	// root rest config
	restConfig, err := kubeutils.GetConfig(
		os.Getenv("KUBERNETES_MASTER_URL"),
		os.Getenv("KUBECONFIG"))
	must(err)

	// wrapper for kubernetes shared informer factory
	cache := kube.NewKubeCache(ctx)

	// initialize the CRD client for Gloo Upstreams
	upstreamClient, err := v1.NewUpstreamClient(ctx, &factory.KubeResourceClientFactory{
		Crd:         v1.UpstreamCrd,
		Cfg:         restConfig,
		SharedCache: cache,
	})
	must(err)

	// registering the client registers the type with the client cache
	err = upstreamClient.Register()
	must(err)

	// initialize the CRD client for Gloo Proxies
	proxyClient, err := v1.NewProxyClient(ctx, &factory.KubeResourceClientFactory{
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

// in this function we'll generate an opinionated
// proxy object with a routes for each of our upstreams
func makeDesiredProxy(upstreams v1.UpstreamList) *v1.Proxy {

	// each virtual host represents the table of routes for a given
	// domain or set of domains.
	// in this example, we'll create one virtual host
	// for each upstream.
	var virtualHosts []*v1.VirtualHost

	for _, upstream := range upstreams {
		upstreamRef := upstream.Metadata.Ref()
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
				Matchers: []*matchers.Matcher{
					&matchers.Matcher{
						PathSpecifier: &matchers.Matcher_Prefix{
							Prefix: "/",
						},
					},
				},

				// tell Gloo where to send the requests
				Action: &v1.Route_RouteAction{
					RouteAction: &v1.RouteAction{
						Destination: &v1.RouteAction_Single{
							// single destination
							Single: &v1.Destination{
								DestinationType: &v1.Destination_Upstream{
									// a "reference" to the upstream, which is a Namespace/Name tuple
									Upstream: &upstreamRef,
								},
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
