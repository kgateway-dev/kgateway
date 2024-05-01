package listenerutils

import (
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func AppendSourceToListener(listener *v1.Listener, source client.Object) {
	meta := listener.GetMetadataStatic()
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
	listener.OpaqueMetadata = &v1.Listener_MetadataStatic{
		MetadataStatic: &v1.SourceMetadata{
			Sources: sources,
		},
	}
}
