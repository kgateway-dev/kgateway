package translator

import (
	errors "github.com/rotisserie/eris"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	usconversions "github.com/solo-io/gloo/projects/gloo/pkg/upstreams"
	"github.com/solo-io/solo-kit/pkg/api/v2/reporter"
)

func (t *translatorInstance) verifyUpstreamGroups(params plugins.Params, reports reporter.ResourceReports) {

	upstreams := params.Snapshot.Upstreams
	upstreamGroups := params.Snapshot.UpstreamGroups

	for _, ug := range upstreamGroups {
		for i, dest := range ug.Destinations {
			if dest.Destination == nil {
				reports.AddError(ug, errors.Errorf("destination # %d: destination is nil", i+1))
				continue
			}

			upRef, err := usconversions.DestinationToUpstreamRef(dest.Destination)
			if err != nil {
				reports.AddError(ug, err)
				continue
			}

			ns := upRef.Namespace
			if upRef.Namespace == "" {
				parentMetadata := ug.GetMetadata()
				ns = parentMetadata.GetNamespace()
			}

			if _, err := upstreams.Find(ns, upRef.Name); err != nil {
				reports.AddError(ug, errors.Wrapf(err, "destination # %d: upstream not found", i+1))
				continue
			}
		}

	}

}

func (t *translatorInstance) fixUpstreamGroups(ug *v1.UpstreamGroup) {
	for _, dest := range ug.Destinations {
		if upstream := dest.GetDestination().GetUpstream(); upstream != nil && upstream.GetNamespace() == "" {
			parentMetadata := ug.GetMetadata()
			upstream.Namespace = parentMetadata.GetNamespace()
		}
	}
}