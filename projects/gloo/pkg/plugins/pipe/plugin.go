package pipe

import (
	"errors"

	envoyapi "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	envoycore "github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
	envoyendpoint "github.com/envoyproxy/go-control-plane/envoy/api/v2/endpoint"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
)

type Plugin struct{}

func NewPlugin() *Plugin {
	return &Plugin{}
}

func (p *Plugin) Init(params plugins.InitParams) error {
	return nil
}

func (p *Plugin) ProcessUpstream(params plugins.Params, in *v1.Upstream, out *envoyapi.Cluster) error {
	// not ours
	pipeSpec, ok := in.UpstreamType.(*v1.Upstream_Pipe)
	if !ok {
		return nil
	}

	spec := pipeSpec.Pipe
	out.ClusterDiscoveryType = &envoyapi.Cluster_Type{
		Type: envoyapi.Cluster_STATIC,
	}
	if spec.Path == "" {
		return errors.New("no path provided")
	}

	out.LoadAssignment = &envoyapi.ClusterLoadAssignment{
		ClusterName: out.Name,
		Endpoints:   []envoyendpoint.LocalityLbEndpoints{{}},
	}

	out.LoadAssignment.Endpoints[0].LbEndpoints = append(out.LoadAssignment.Endpoints[0].LbEndpoints,
		envoyendpoint.LbEndpoint{
			HostIdentifier: &envoyendpoint.LbEndpoint_Endpoint{
				Endpoint: &envoyendpoint.Endpoint{
					Address: &envoycore.Address{
						Address: &envoycore.Address_Pipe{
							Pipe: &envoycore.Pipe{
								Path: spec.Path,
							},
						},
					},
				},
			},
		})

	return nil
}
