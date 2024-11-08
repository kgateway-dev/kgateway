package edsupstream

import (
	cluster_v3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	"github.com/solo-io/gloo/projects/gloo/pkg/xds"
)

// InternalEDSLabel is a marker that synthetic Upstreams can use to indicate
// their clusters should be treated as EDS clusters.
// This label name contains invalid characters so it cannot mistakenly be used in
// an actual Kubernetes resource.
const InternalEDSLabel = "~internal.solo.io/eds-upstream"

var _ plugins.UpstreamPlugin = &seUsPlugin{}

type seUsPlugin struct {
	settings *v1.Settings
}

// NewPlugin will convert our specific type of upstreams to create EDS clusters.
func NewPlugin() plugins.UpstreamPlugin {
	return &seUsPlugin{}
}

func (p *seUsPlugin) Init(params plugins.InitParams) {
	p.settings = params.Settings
}

func (p *seUsPlugin) Name() string {
	return "InternalEDSPlugin"
}

func (p *seUsPlugin) ProcessUpstream(params plugins.Params, in *v1.Upstream, out *cluster_v3.Cluster) error {
	// not ours
	if _, ok := in.GetMetadata().GetLabels()[InternalEDSLabel]; !ok {
		return nil
	}

	// use EDS
	xds.SetEdsOnCluster(out, p.settings)
	// clear a non-EDS CLA
	out.LoadAssignment = nil
	// clear DNS only things
	out.DnsRefreshRate = nil

	return nil
}
