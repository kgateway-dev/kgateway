package xdsinspection

import (
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/aws/ec2"

	"github.com/solo-io/gloo/projects/gloo/pkg/translator"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
)

func (xd *XdsDump) GetEc2InstancesForUpstream(upstream *core.ResourceRef) []string {
	var out []string
	if xd == nil {
		out = append(out, "use -o wide for instance details")
		return out
	}
	clusterName := translator.UpstreamToClusterName(upstream)
	endpointCount := 0
	for _, clusterEndpoints := range xd.Endpoints {
		if clusterEndpoints.ClusterName == clusterName {
			for _, lEp := range clusterEndpoints.Endpoints {
				for _, ep := range lEp.LbEndpoints {
					if k, ok := ep.Metadata.FilterMetadata[translator.SoloAnnotations]; ok {
						v, ok := k.Fields[ec2.InstanceIdAnnotationKey]
						if ok {
							endpointCount++
							out = append(out, v.GetStringValue())
						}
					}
				}
			}
		}
	}
	if endpointCount == 0 {
		out = append(out, "no endpoints")
	}
	return out
}
