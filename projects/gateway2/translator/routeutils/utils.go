package routeutils

import (
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func AppendSourceToRoute(route *v1.Route, source client.Object) {
	meta := route.GetMetadataStatic()
	if meta == nil {
		meta = &v1.SourceMetadata{}
	}
	sources := meta.GetSources()
	sources = append(sources, &v1.SourceMetadata_SourceRef{
		ResourceRef: &core.ResourceRef{
			Name:      source.GetName(),
			Namespace: source.GetNamespace(),
		},
		ResourceKind:       source.GetObjectKind().GroupVersionKind().Kind,
		ObservedGeneration: source.GetGeneration(),
	})
	route.OpaqueMetadata = &v1.Route_MetadataStatic{
		MetadataStatic: &v1.SourceMetadata{
			Sources: sources,
		},
	}
}
