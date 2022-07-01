//go:build ignore
// +build ignore

package docs_demo

// package gce

import (
	"context"
	"os"
	"time"

	cluster_v3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/discovery"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"

	// add these imports to use Envoy's API
	envoycore "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"

	// add these imports to use Google Compute Engine's API
	"golang.org/x/oauth2/google"
	"google.golang.org/api/compute/v1"
	"google.golang.org/api/option"
)

type plugin struct{}

func NewPlugin() *plugin {
	return &plugin{}
}

func (*plugin) ProcessUpstream(params plugins.Params, in *v1.Upstream, out *cluster_v3.Cluster) error {
	// check that the upstream is our type (GCE)
	if _, ok := in.UpstreamType.(*v1.UpstreamSpec_Gce); !ok {
		// not gce, return early
		return nil
	}
	// tell Envoy to use EDS to get endpoints for this cluster
	out.ClusterDiscoveryType = &cluster_v3.Cluster_Type{
		Type: cluster_v3.Cluster_EDS,
	}
	// tell envoy to use ADS to resolve Endpoints
	out.EdsClusterConfig = &cluster_v3.Cluster_EdsClusterConfig{
		EdsConfig: &envoycore.ConfigSource{
			ConfigSourceSpecifier: &envoycore.ConfigSource_Ads{
				Ads: &envoycore.AggregatedConfigSource{},
			},
		},
	}
	return nil
}

func (*plugin) WatchEndpoints(writeNamespace string, upstreamsToTrack v1.UpstreamList, opts clients.WatchOpts) (<-chan v1.EndpointList, <-chan error, error) {
	// use the context from the opts we were passed
	ctx := opts.Ctx

	// get the client for interacting with GCE VM Instances
	instancesClient, err := initializeClient(ctx)
	if err != nil {
		return nil, nil, err
	}

	// initialize the channel on which we will send endpoint results to Gloo
	results := make(chan v1.EndpointList)

	// initialize a channel on which we can send polling errors to Gloo
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
				endpoints, err := getLatestEndpoints(instancesClient, upstreamsToTrack)
				if err != nil {
					// send the error to Gloo for logging
					errorsDuringUpdate <- err
				} else {
					// send the latest set of endpoints to Gloo
					results <- endpoints
				}

				// sleep 10s between polling
				time.Sleep(time.Second * 10)
			}
		}
	}()

	// return the channels to Gloo
	return results, errorsDuringUpdate, nil
}

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

// one call results in a list of endpoints for our upstreams
func getLatestEndpoints(instancesClient *compute.InstancesService, upstreams v1.UpstreamList) (v1.EndpointList, error) {

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
			if len(instance.NetworkInterfaces) == 0 {
				// skip vms that don't have an allocated IP address
				continue
			}

			if !shouldSelectInstance(gceSpec.Selector, instance.Labels) {
				// the selector doesn't match this instance, skip it
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
					Namespace: us.Metadata.Namespace,
					Name:      instance.Name,
					Labels:    instance.Labels,
				},
				Address: address,
				Port:    port,
				// normally if more than one upstream shares an endpoint
				// we would provide a list here
				Upstreams: []*core.ResourceRef{&upstreamRef},
			}

			// add the endpoint to our list
			result = append(result, endpointForInstance)
		}
	}
	return result, nil
}

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
