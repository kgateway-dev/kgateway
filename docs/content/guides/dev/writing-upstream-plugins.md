---
title: "Service discovery plugins for Gloo Edge"

weight: 5
---

## Intro

Gloo Edge uses the [v1.Upstream]({{% versioned_link_path fromRoot="/reference/api/github.com/solo-io/gloo/projects/gloo/api/v1/upstream.proto.sk" %}}) config object to define routable destinations for Gloo Edge. These are converted inside Gloo Edge.

In this tutorial, you learn how to write an Upstream plugin for virtual machines (VM) that are hosted on Google Compute Engine, and to add the plugin to Gloo Edge to enable service discovery. A single endpoint represents a single VM, and the Upstream groups these instances by using the labels that are assigned to the VMs. 

Note that *any* backend store of service addresses can be plugged into Gloo Edge in this way. 

The Upstream plugin connects to the external source of truth (in this case, the Google Compute Engine API) and converts the information into configuration that Gloo Edge can understand and supply to Envoy for routing. 

To see the completed code for this tutorial:

* [gce.proto](../gce.proto): The API definitions for the plugin.
* [plugins.proto](../plugins.proto): The Gloo Edge Core API that includes the plugin API.
* [plugin.go](../plugin.go): The actual code for the plugin.
* [registry.go](../registry.go): The Gloo Edge Plugin Registry that includes the plugin.


{{% notice tip %}}
Want to see and try out a dummy example instead? Check out this [branch](https://github.com/mwieczorek/gloo/tree/dummy) to find sample code that you can use as a basis to create Upstream plugins for your own use case. 
{{% /notice %}}



## Environment Setup

To set up a development environment for Gloo Edge including installing prerequisites to generate code and build docker images, [see the dev setup guide]({{% versioned_link_path fromRoot="/guides/dev/setting-up-dev-environment" %}}). Make sure you 
include the **Enabling Code Generation** section of that tutorial.

## Upstream Plugin

For Gloo Edge, an upstream represents a single service backed by one or more *endpoints* (where each endpoint is an IP or hostname plus port) that accepts TCP or HTTP traffic. Upstreams can provide their endpoints to Gloo Edge hard-coded inside their YAML spec, as with the `static` Upstream type. Alternatively, Upstreams can provide information to Gloo Edge so that a corresponding Gloo Edge plugin can perform Endpoint Discovery (EDS).  

This tutorial shows how to create an EDS-style plugin, where you provide a Google Compute Engine (GCE) Upstream to Gloo Edge, and the plugin retrieves each endpoint for that Upstream.

Let's begin.

## Adding the new Upstream Type to Gloo Edge's API

The first step we'll take will be to add a new {{% protobuf name="gloo.solo.io.Upstream" display="UpstreamType" %}} to Gloo Edge. 

All of Gloo Edge's APIs are defined as protobuf files (`.proto`). The list of Upstream Types live in the {{% protobuf name="gloo.solo.io.UpstreamSpec" %}} file, where Gloo Edge's core API objects (Upstream, Virtual Service, Proxy, Gateway) are bound to plugin-specific configuration.

We'll write a simple `UpstreamSpec` proto for the new `gce` upstream type:

```proto
syntax = "proto3";
package gce.options.gloo.solo.io;

option go_package = "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/plugins/gce";

option (extproto.equal_all) = true;

// Upstream Spec for Google Compute Engine Upstreams
// GCE Upstreams represent a set of one or more addressable VM instances with
// a shared set of tags
message UpstreamSpec {
    // get endpoints from instances whose labels match this selector
    map<string, string> selector = 1;
    // zone in which the instances live
    string zone = 2;
    // the GCP project to which the instances belong
    string project_id = 3;
    // the port on which the instances are listening
    // create multiple upstreams to support multiple ports
    uint32 port = 4;
}
```

Let's follow the established convention and place our proto code into a new `gce` directory in the `api/v1/plugins` API root:

```bash
# cd to the gloo directory
cd ${GOPATH}/src/github.com/solo-io/gloo
# make the new gce plugin directory
mkdir -p projects/gloo/api/v1/plugins/gce
# paste the proto code from above to projects/gloo/api/v1/plugins/gce/gce.proto 
cat > projects/gloo/api/v1/plugins/gce/gce.proto <<EOF
syntax = "proto3";
package gce.options.gloo.solo.io;

option go_package = "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/plugins/gce";

option (extproto.equal_all) = true;

// Upstream Spec for Google Compute Engine Upstreams
// GCE Upstreams represent a set of one or more addressable VM instances with
// a shared set of tags
message UpstreamSpec {
    // get endpoints from instances whose labels match this selector
    map<string, string> selector = 1;
    // zone in which the instances live
    string zone = 2;
    // the GCP project to which the instances belong
    string project_id = 3;
    // the port on which the instances are listening
    // create multiple upstreams to support multiple ports
    uint32 port = 4;
}
EOF

```

You can view the complete `gce.proto` here: [gce.proto](../gce.proto). 


Now we need to add the new GCE `UpstreamSpec` to Gloo Edge's list of Upstream Types. This can be found in 
the {{% protobuf name="gloo.solo.io.UpstreamSpec" %}} file at the API root (projects/gloo/api/v1)/. 

First, we'll add an import to the top of the file

{{< highlight proto "hl_lines=31-32" >}}
syntax = "proto3";
package gloo.solo.io;
option go_package = "github.com/solo-io/gloo/projects/gloo/pkg/api/v1";

import "google/protobuf/struct.proto";

option (extproto.equal_all) = true;

import "github.com/solo-io/gloo/projects/gloo/api/v1/ssl/ssl.proto";
import "github.com/solo-io/gloo/projects/gloo/api/v1/extensions.proto";
import "github.com/solo-io/gloo/projects/gloo/api/v1/circuit_breaker.proto";
import "github.com/solo-io/gloo/projects/gloo/api/v1/load_balancer.proto";
import "github.com/solo-io/gloo/projects/gloo/api/v1/connection.proto";

import "github.com/solo-io/gloo/projects/gloo/api/v1/plugins/aws/aws.proto";
import "github.com/solo-io/gloo/projects/gloo/api/v1/plugins/rest/rest.proto";
import "github.com/solo-io/gloo/projects/gloo/api/v1/plugins/grpc/grpc.proto";
import "github.com/solo-io/gloo/projects/gloo/api/v1/plugins/grpc_web/grpc_web.proto";
import "github.com/solo-io/gloo/projects/gloo/api/v1/plugins/hcm/hcm.proto";
import "github.com/solo-io/gloo/projects/gloo/api/v1/plugins/tcp/tcp.proto";
import "github.com/solo-io/gloo/projects/gloo/api/v1/plugins/azure/azure.proto";
import "github.com/solo-io/gloo/projects/gloo/api/v1/plugins/consul/consul.proto";
import "github.com/solo-io/gloo/projects/gloo/api/v1/plugins/kubernetes/kubernetes.proto";
import "github.com/solo-io/gloo/projects/gloo/api/v1/plugins/retries/retries.proto";
import "github.com/solo-io/gloo/projects/gloo/api/v1/plugins/static/static.proto";
import "github.com/solo-io/gloo/projects/gloo/api/v1/plugins/stats/stats.proto";
import "github.com/solo-io/gloo/projects/gloo/api/v1/plugins/transformation/prefix_rewrite.proto";
import "github.com/solo-io/gloo/projects/gloo/api/v1/plugins/transformation/transformation.proto";
import "github.com/solo-io/gloo/projects/gloo/api/v1/plugins/faultinjection/fault.proto";
// add the following line:
import "github.com/solo-io/gloo/projects/gloo/api/v1/plugins/gce/gce.proto";

{{< /highlight >}}

Next we'll add the new `UpstreamSpec` from our import. Locate the `UpstreamSpec` at the bottom of the `plugins.proto` file. The new `gce` UpstreamSpec must be added to the `upstream_type` oneof, like so:

{{< highlight proto "hl_lines=27-28" >}}

// Each upstream in Gloo Edge has a type. Supported types include `static`, `kubernetes`, `aws`, `consul`, and more.
// Each upstream type is handled by a corresponding Gloo Edge plugin.
message UpstreamSpec {

    UpstreamSslConfig ssl_config = 6;

    // Circuit breakers for this upstream. if not set, the defaults ones from the Gloo Edge settings will be used.
    // if those are not set, [envoy's defaults](https://www.envoyproxy.io/docs/envoy/latest/api-v3/config/cluster/v3/circuit_breaker.proto#envoy-api-msg-cluster-circuitbreakers)
    // will be used.
    CircuitBreakerConfig circuit_breakers = 7;
    LoadBalancerConfig load_balancer_config = 8;
    ConnectionConfig connection_config = 9;

    // Use http2 when communicating with this upstream
    // this field is evaluated `true` for upstreams
    // with a grpc service spec
    bool use_http2 = 10;

    // Note to developers: new Upstream Plugins must be added to this oneof field
    // to be usable by Gloo Edge.
    oneof upstream_type {
        kubernetes.options.gloo.solo.io.UpstreamSpec kube = 1;
        static.options.gloo.solo.io.UpstreamSpec static = 4;
        aws.options.gloo.solo.io.UpstreamSpec aws = 2;
        azure.options.gloo.solo.io.UpstreamSpec azure = 3;
        consul.options.gloo.solo.io.UpstreamSpec consul = 5;
        // add the following line
        gce.plugins.gloo.solo.io.UpstreamSpec gce = 11;
    }
}

{{< /highlight >}}

You can view the complete `plugins.proto` here: [plugins.proto](../plugins.proto). 

Great! We're all set to run code generation on Gloo Edge and begin writing our plugin!

## Running the Code Generation

To regenerate code in the project, we will need `go`, `make`, `dep`, and `protoc` installed. If they aren't already, [see the dev setup guide]({{% versioned_link_path fromRoot="/guides/dev/setting-up-dev-environment" %}}).

To (re)generate code:

```bash
# go to gloo root dir
cd ${GOPATH}/src/github.com/solo-io/gloo
# run code generation 
make generated-code # add -B if you need to re-run 

```

We should be able to see modifications and additions to the generated code in `projects/gloo/pkg/api/v1`. Run `git status` to see what's been changed.

Let's start writing our plugin!

### Plugin code

#### Skeleton

We'll start by creating a new package/directory for our code to live in. Following the convention in Gloo Edge, we'll create our new package at `projects/gloo/pkg/plugins/gce`:

```bash
cd ${GOPATH}/src/github.com/solo-io/gloo
mkdir -p projects/gloo/pkg/plugins/gce
touch projects/gloo/pkg/plugins/gce/plugin.go
```

We'll start writing the code for our plugin in `plugin.go`:

```go
package gce

type plugin struct{}

func NewPlugin() *plugin {
	return &plugin{}
}

```

So far, our plugin is just a plain go struct with no features. In order to provide service discovery for Gloo Edge, our plugin needs to implement two interfaces: the [`plugins.UpstreamPlugin`](https://github.com/solo-io/gloo/blob/main/projects/gloo/pkg/plugins/plugin_interface.go#L43) and [`discovery.DiscoveryPlugin`](https://github.com/solo-io/gloo/blob/main/projects/gloo/pkg/discovery/discovery.go#L21) interfaces.

Let's add the functions necessary to implement these interfaces:

```go
package gce

import (
	"github.com/envoyproxy/go-control-plane/envoy/api/v2"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/discovery"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
)

type plugin struct{}

func NewPlugin() *plugin {
	return &plugin{}
}

func (*plugin) ProcessUpstream(params plugins.Params, in *v1.Upstream, out *v2.Cluster) error {
	// we'll add our implementation here
}
func (*plugin) WatchEndpoints(writeNamespace string, upstreamsToTrack v1.UpstreamList, opts clients.WatchOpts) (<-chan v1.EndpointList, <-chan error, error) {
	// we'll add our implementation here
}

// it is sufficient to return nil here 
func (*plugin) Init(params plugins.InitParams) error {
	return nil
}

// though required by the plugin interface, this function is not necesasary for our plugin 
func (*plugin) DiscoverUpstreams(watchNamespaces []string, writeNamespace string, opts clients.WatchOpts, discOpts discovery.Opts) (chan v1.UpstreamList, chan error, error) {
	return nil, nil, nil
}

// though required by the plugin interface, this function is not necesasary for our plugin 
func (*plugin) UpdateUpstream(original, desired *v1.Upstream) (bool, error) {
	return false, nil
}

```

#### ProcessUpstream

Our plugin now implements the required interfaces and can be plugged into Gloo Edge. For the purpose of this tutorial, 
we will only need the `ProcessUpstream` and `WatchEndpoints` functions to be implemented for our plugin. The rest
can be no-op and will simply be ignored by Gloo Edge.

First, let's handle `ProcessUpstream`. `ProcessUpstream` is called for every **Upstream** known to Gloo Edge in 
each iteration of Gloo Edge's translation loop (in which Gloo Edge config is translated to Envoy config). `ProcessUpstream`
looks at each individual Upstream (the user input object) and modifies, if necessary, the ouptut [Envoy Cluster](https://www.envoyproxy.io/docs/envoy/latest/api-v3/config/cluster/v3/cluster.proto) corresponding to that Upstream. 

Our `ProcessUpstream` function should:

* Check that the user's Upstream is *ours* (of type GCE)
* If so, mark the Cluster to use EDS

Let's implement that in our function right now:

```go
package gce

import (
	//...

	// add these imports to use Envoy's API
	envoyapi "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	envoycore "github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
)

//...

func (*plugin) ProcessUpstream(params plugins.Params, in *v1.Upstream, out *v2.Cluster) error {
	// check that the upstream is our type (GCE)
	if _, ok := in.UpstreamType.(*v1.UpstreamSpec_Gce); !ok {
		// not gce, return early
		return nil
	}
	// tell Envoy to use EDS to get endpoints for this cluster 
	out.ClusterDiscoveryType = &envoyapi.Cluster_Type{
		Type: envoyapi.Cluster_EDS,
	}
	// tell envoy to use ADS to resolve Endpoints  
	out.EdsClusterConfig = &envoyapi.Cluster_EdsClusterConfig{
		EdsConfig: &envoycore.ConfigSource{
			ConfigSourceSpecifier: &envoycore.ConfigSource_Ads{
				Ads: &envoycore.AggregatedConfigSource{},
			},
		},
	}
	return nil
} 

```

All EDS-based plugins must implement the above function. See the `kubernetes` plugin for another example plugin for Envoy EDS. 

#### WatchEndpoints

The last piece our plugin needs is the `WatchEndpoints` function.  Here's where the meat of our plugin will live. 

We need to:

* Poll the GCE API
* Retrieve the list of instances
* Correlate those addresses with the user's GCE Upstreams by their labels 
* Compose a list of Endpoints and send them on a channel to Gloo Edge
* Repeat this at some interval to keep endpoints updated

So let's start writing our function. We'll need to add some imports to interact with the GCE API:

```go
package gce

import (
	//...

	// add these imports to use Google Compute Engine's API
	"golang.org/x/oauth2/google"
	"google.golang.org/api/compute/v1"	
	"google.golang.org/api/option"
)
```

Gloo Edge now uses go modules, so these should automatically be pulled into your IDE. However, 
on older versions of Gloo Edge that use dep, we can download these imports to our project with `dep ensure`:

```bash
cd ${GOPATH}/src/github.com/solo-io/gloo
dep ensure -v
```

Now we can develop our plugin.

Before we can discover our endpoints, we'll need to connect to the Google Compute API for Instances. Let's implement a function to initialize our client for us:

```go

// initialize client for talking to Google Compute Engine API
func initializeClient(ctx context.Context) (*compute.InstancesService, error) {
	// initialize google credentials from a custom environment variable
	// environment variables are not a secure way to share credentials to our application
	// and are only used here for the sake of convenience
	// we will store the content of our Google Developers Console client_credentials.json
	// as the value for GOOGLE_CREDENTIALS_JSON
	credsJson := []byte(os.Getenv("GOOGLE_CREDENTIALS_JSON"))
	creds, err := google.CredentialsFromJSON(ctx, credsJson, compute.ComputeScope)
	if err != nil {
		return nil, err
	}
	token := option.WithTokenSource(creds.TokenSource)
	svc, err := compute.NewService(ctx, token)
	if err != nil {
		return nil, err
	}
	instancesClient := compute.NewInstancesService(svc)

	return instancesClient, nil
}

```

For the purpose of the tutorial, we'll simply pass our Google API Credentials 
as the environment variable `GOOGLE_CREDENTIALS_JSON`. In a real 
production environment we'd want to retrieve
credentials from a secret store such as Kubernetes or Vault.

See https://cloud.google.com/video-intelligence/docs/common/auth on downloading this file. Its contents should be stored to an environment variable on the server running Gloo Edge. We can set this on the deployment template for Gloo Edge once we're ready to deploy to Kube.

Now that we have access to our client, we're ready to set up our polling 
function. It should retrieve the list of 
VMs from GCE and convert them to Endpoints 
for Gloo Edge. Additionally, it should track 
the Upstream each Endpoint belongs to, and 
ignore any endpoints that don't belong to 
an upstream.

The declaration for our function reads as follows:

```go
// one call results in a list of endpoints for our upstreams
func getLatestEndpoints(instancesClient *compute.InstancesService, upstreams v1.UpstreamList, writeNamespace string) (v1.EndpointList, error) {
	//...
}
```

`getLatestEndpoints` will take as inputs the instances client and the for which we're upstreams discovering endpoints. Its outputs will be a list of endpoints and an error (if encountered during polling).

{{< highlight go "hl_lines=9-14 17-24" >}}

// one call results in a list of endpoints for our upstreams
func getLatestEndpoints(instancesClient *compute.InstancesService, upstreams v1.UpstreamList, writeNamespace string) (v1.EndpointList, error) {

	// initialize a new list of endpoints
	var result v1.EndpointList

	// for each upstream, retrieve its endpoints
	for _, us := range upstreams {
	  // check that the upstream uses the GCE Spec
		gceSpec := us.GetGce()
		if gceSpec == nil {
			// skip non-GCE upstreams
			continue
		}
		
		
		// get the Google Compute VM Instances for the project/zone
		instancesForUpstream, err := instancesClient.List(
			gceSpec.ProjectId,
			gceSpec.Zone,
		).Do()
		if err != nil {
			return nil, err
		}

    //...
	}
	
	return result, nil
}

{{< /highlight >}}

We're now listing all the instances in the Google Cloud Project/Zone for the Upstream, but we still need to filter them down to the instances for the specific upstream. 

Let's add a convenience function `shouldSelectInstance` to do our filtering:

```go

// inspect the labels for a match
func shouldSelectInstance(selector, instanceLabels map[string]string) bool {
	if len(instanceLabels) == 0 {
		// only an empty selector can match empty labels
		return len(selector) == 0
	}

	for k, v := range selector {
		instanceVal, ok := instanceLabels[k]
		if !ok {
			// the selector key is missing from the instance labels
			return false
		}
		if v != instanceVal {
			// the label value in the selector does not match
			// the label value from the instance
			return false
		}
	}
	// we didn't catch a mismatch by now, they match
	return true
}
```

We'll use this function to filter instances in `getLatestEndpoints`:

{{< highlight go "hl_lines=26-33" >}}

// one call results in a list of endpoints for our upstreams
func getLatestEndpoints(instancesClient *compute.InstancesService, upstreams v1.UpstreamList, writeNamespace string) (v1.EndpointList, error) {

	// initialize a new list of endpoints
	var result v1.EndpointList

	// for each upstream, retrieve its endpoints
	for _, us := range upstreams {
	  // check that the upstream uses the GCE Spec
		gceSpec := us.GetGce()
		if gceSpec == nil {
			// skip non-GCE upstreams
			continue
		}
		
		
		// get the Google Compute VM Instances for the project/zone
		instancesForUpstream, err := instancesClient.List(
			gceSpec.ProjectId,
			gceSpec.Zone,
		).Do()
		if err != nil {
			return nil, err
		}

		// iterate over each instance
		// add its address as an endpoint if its labels match
		for _, instance := range instancesForUpstream.Items {

			if !shouldSelectInstance(gceSpec.Selector, instance.Labels) {
				// the selector doesn't match this instance, skip it
				continue
			}

			// ...
		}
	}
	
	return result, nil
}

{{< /highlight >}}

We must also filter instances that don't have an IP address:


{{< highlight go "hl_lines=35-38" >}}

// one call results in a list of endpoints for our upstreams
func getLatestEndpoints(instancesClient *compute.InstancesService, upstreams v1.UpstreamList, writeNamespace string) (v1.EndpointList, error) {

	// initialize a new list of endpoints
	var result v1.EndpointList

	// for each upstream, retrieve its endpoints
	for _, us := range upstreams {
	  // check that the upstream uses the GCE Spec
		gceSpec := us.GetGce()
		if gceSpec == nil {
			// skip non-GCE upstreams
			continue
		}
		
		
		// get the Google Compute VM Instances for the project/zone
		instancesForUpstream, err := instancesClient.List(
			gceSpec.ProjectId,
			gceSpec.Zone,
		).Do()
		if err != nil {
			return nil, err
		}

		// iterate over each instance
		// add its address as an endpoint if its labels match
		for _, instance := range instancesForUpstream.Items {

			if !shouldSelectInstance(gceSpec.Selector, instance.Labels) {
				// the selector doesn't match this instance, skip it
				continue
			}

			if len(instance.NetworkInterfaces) == 0 {
				// skip vms that don't have an allocated IP address
				continue
			}
		}
		
		// convert the instance to an endpoint ...
	}
	
	return result, nil
}

{{< /highlight >}}

Now we've filtered out all instances that don't match the upstream for which we're discovering endpoints. 

Finally, we must convert the instance to an endpoint and append it to our list:

{{< highlight go "hl_lines=40-61 63-64" >}}

// one call results in a list of endpoints for our upstreams
func getLatestEndpoints(instancesClient *compute.InstancesService, upstreams v1.UpstreamList, writeNamespace string) (v1.EndpointList, error) {

	// initialize a new list of endpoints
	var result v1.EndpointList

	// for each upstream, retrieve its endpoints
	for _, us := range upstreams {
	  // check that the upstream uses the GCE Spec
		gceSpec := us.GetGce()
		if gceSpec == nil {
			// skip non-GCE upstreams
			continue
		}
		
		
		// get the Google Compute VM Instances for the project/zone
		instancesForUpstream, err := instancesClient.List(
			gceSpec.ProjectId,
			gceSpec.Zone,
		).Do()
		if err != nil {
			return nil, err
		}

		// iterate over each instance
		// add its address as an endpoint if its labels match
		for _, instance := range instancesForUpstream.Items {

			if !shouldSelectInstance(gceSpec.Selector, instance.Labels) {
				// the selector doesn't match this instance, skip it
				continue
			}

			if len(instance.NetworkInterfaces) == 0 {
				// skip vms that don't have an allocated IP address
				continue
			}
		
			// use the first network ip of the vm for our endpoint
			address := instance.NetworkInterfaces[0].NetworkIP

			// get the port from the upstream spec
			port := gceSpec.Port

			// provide a pointer back to the upstream this
			// endpoint was created for
			upstreamRef := us.Metadata.Ref()

			endpointForInstance := &v1.Endpoint{
				Metadata: core.Metadata{
					Namespace: writeNamespace,
					Name:      instance.Name,
					Labels:    instance.Labels,
				},
				Address:   address,
				Port:      port,
				// normally if more than one upstream shares an endpoint
				// we would provide a list here
				Upstreams: []*core.ResourceRef{&upstreamRef},
			}

			// add the endpoint to our list
			result = append(result, endpointForInstance)
	}
	
	return result, nil
}

{{< /highlight >}}

Now that our `getLatestEndpoints` function is finished, we 
can tie everything together in our plugin's `WatchEndpoints`.

Let's get the initializations out of the way:

{{< highlight go "hl_lines=2-3 5-9 11-12 14-15 17-22 24-25" >}}

func (*plugin) WatchEndpoints(writeNamespace string, upstreamsToTrack v1.UpstreamList, opts clients.WatchOpts) (<-chan v1.EndpointList, <-chan error, error) {
	// use the context from the opts we were passed
	ctx := opts.Ctx

	// get the client for interacting with GCE VM Instances
	instancesClient, err := initializeClient(ctx)
	if err != nil {
		return nil, nil, err
	}

	// initialize the channel on which we will send endpoint results to Gloo Edge
	results := make(chan v1.EndpointList)

	// initialize a channel on which we can send polling errors to Gloo Edge
	errorsDuringUpdate := make(chan error)

	// in a goroutine, continue updating endpoints at an interval
	// until the context is done
	go func() {
		// poll endpoints here...
		}
	}()

	// return the channels to Gloo Edge
	return results, errorsDuringUpdate, nil
}

{{< /highlight >}}

The last step is to fill in our new goroutine. Let's have it poll on an interval of 10 seconds, sending updated `v1.EndpointList`s down the `results` channel:

{{< highlight go "hl_lines=20-43" >}}

func (*plugin) WatchEndpoints(writeNamespace string, upstreamsToTrack v1.UpstreamList, opts clients.WatchOpts) (<-chan v1.EndpointList, <-chan error, error) {
	// use the context from the opts we were passed
	ctx := opts.Ctx

	// get the client for interacting with GCE VM Instances
	instancesClient, err := initializeClient(ctx)
	if err != nil {
		return nil, nil, err
	}

	// initialize the channel on which we will send endpoint results to Gloo Edge
	results := make(chan v1.EndpointList)

	// initialize a channel on which we can send polling errors to Gloo Edge
	errorsDuringUpdate := make(chan error)

	// in a goroutine, continue updating endpoints at an interval
	// until the context is done
	go func() {
		// once this goroutine exits, we should close our output channels
		defer close(results)
		defer close(errorsDuringUpdate)

		// poll indefinitely
		for {
			select {
			case <-ctx.Done():
				// context was cancelled, stop polling
				return
			default:
				endpoints, err := getLatestEndpoints(instancesClient, upstreamsToTrack, writeNamespace)
				if err != nil {
					// send the error to Gloo Edge for logging
					errorsDuringUpdate <- err
				} else {
					// send the latest set of endpoints to Gloo Edge
					results <- endpoints
				}

				// sleep 10s between polling
				time.Sleep(time.Second * 10)
			}
		}
	}()

	// return the channels to Gloo Edge
	return results, errorsDuringUpdate, nil
}

{{< /highlight >}}

Our `WatchEndpoints` is now finished, along with our plugin!

We are not finished, however. The task remains to wire our plugin 
into the Gloo Edge core, then rebuild Gloo Edge and deploy to Kubernetes!

All Gloo Edge plugins are registered inside of a `registry` subpackage within the `plugins` directory. See [the registry.go file on GitHub here](https://github.com/solo-io/gloo/blob/main/projects/gloo/pkg/plugins/registry/registry.go).

We need to add our plugin (and its import) to `registry.go`:


{{< highlight go "hl_lines=12-13 52-53" >}}
package registry

import (
	"github.com/solo-io/gloo/projects/gloo/pkg/bootstrap"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/aws"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/azure"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/basicroute"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/consul"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/cors"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/faultinjection"
	// add our plugin's import here:
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/gce"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/grpc"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/hcm"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/kubernetes"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/linkerd"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/loadbalancer"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/rest"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/static"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/stats"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/transformation"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/upstreamconn"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/upstreamssl"
)

type registry struct {
	plugins []plugins.Plugin
}

var globalRegistry = func(opts bootstrap.Opts, pluginExtensions ...plugins.Plugin) *registry {
	transformationPlugin := transformation.NewPlugin()
	reg := &registry{}
	// plugins should be added here
	reg.plugins = append(reg.plugins,
		loadbalancer.NewPlugin(),
		upstreamconn.NewPlugin(),
		upstreamssl.NewPlugin(),
		azure.NewPlugin(&transformationPlugin.RequireTransformationFilter),
		aws.NewPlugin(&transformationPlugin.RequireTransformationFilter),
		rest.NewPlugin(&transformationPlugin.RequireTransformationFilter),
		hcm.NewPlugin(),
		static.NewPlugin(),
		transformationPlugin,
		consul.NewPlugin(),
		grpc.NewPlugin(&transformationPlugin.RequireTransformationFilter),
		faultinjection.NewPlugin(),
		basicroute.NewPlugin(),
		cors.NewPlugin(),
		linkerd.NewPlugin(),
		stats.NewPlugin(),
		// and our plugin goes here
		gce.NewPlugin(),
	)
	if opts.KubeClient != nil {
		reg.plugins = append(reg.plugins, kubernetes.NewPlugin(opts.KubeClient))
	}
	for _, pluginExtension := range pluginExtensions {
		reg.plugins = append(reg.plugins, pluginExtension)
	}

	return reg
}

func Plugins(opts bootstrap.Opts, pluginExtensions ...plugins.Plugin) []plugins.Plugin {
	return globalRegistry(opts, pluginExtensions...).plugins
}

{{< /highlight >}}

Code changes are now complete. You can view the all of the code here:

* [gce.proto](../gce.proto): API definitions for our plugin.
* [plugins.proto](../plugins.proto): The Gloo Edge Core API with our plugin API added to it.
* [plugin.go](../plugin.go): The actual code for the plugin.
* [registry.go](../registry.go): The Gloo Edge Plugin Registry with our plugin added to it.

## Build and Deploy from Source

To see our new and improved Gloo Edge in action, follow the 
[building and deploying Gloo Edge from source tutorial]({{% versioned_link_path fromRoot="/guides/dev/building-and-deploying-gloo" %}}).

## Conclusions

We've just seen how to extend Gloo Edge's service discovery 
mechanism via the use of a plugin. While this plugin 
focused on the discovery of VMs from a hosted cloud 
provider, Gloo Edge Upstream Plugins can be used to import 
endpoint data from any conceivable source of truth, as 
long as those endpoints represent TCP/HTTP services 
listening on some port. 

There are many other places where Gloo Edge supports 
extensibility through plugins, including leveraging new 
(or previously unused) Envoy filters. 

Hopefully you're now more familiar with how Gloo Edge plugins 
work. Maybe you're even ready to start writing your own 
plugins. At the very least, you now have a look inside 
how Gloo Edge Plugins connect external sources of truth to Envoy.

We encourage you to check out our other dev tutorials to discover other ways of extending Gloo Edge!
