package translator

import (
	structpb "github.com/golang/protobuf/ptypes/struct"
	errors "github.com/rotisserie/eris"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	"github.com/solo-io/solo-kit/pkg/utils/protoutils"
)

type SourceMetadata struct {
	Sources []SourceRef `json:"sources"`
}

type SourceRef struct {
	*core.ResourceRef
	ResourceKind       string `json:"kind"`
	ObservedGeneration int64  `json:"observedGeneration"`
}

type ObjectWithMetadata interface {
	GetMetadata() *structpb.Struct
	GetMetadataStatic() *v1.SourceMetadata
}

func sourceMetaFromStruct(s *structpb.Struct) (*SourceMetadata, error) {
	if s == nil {
		return nil, nil
	}
	var m SourceMetadata
	if err := protoutils.UnmarshalStruct(s, &m); err != nil {
		return nil, err
	}
	return &m, nil
}

func sourceMetaFromProto(s *v1.SourceMetadata) *SourceMetadata {
	if s == nil {
		return nil
	}
	result := &SourceMetadata{}
	for _, source := range s.GetSources() {
		result.Sources = append(result.Sources, SourceRef{
			ResourceRef:        source.GetResourceRef(),
			ResourceKind:       source.GetResourceKind(),
			ObservedGeneration: source.GetObservedGeneration(),
		})
	}
	return result
}

func objMetaToSourceMeta(meta *SourceMetadata) *v1.SourceMetadata {
	if meta == nil {
		return nil
	}
	result := &v1.SourceMetadata{}
	for _, source := range meta.Sources {
		result.Sources = append(result.GetSources(), &v1.SourceMetadata_SourceRef{
			ResourceRef:        source.ResourceRef,
			ResourceKind:       source.ResourceKind,
			ObservedGeneration: source.ObservedGeneration,
		})
	}
	return result
}

func setObjMeta(obj ObjectWithMetadata, meta *SourceMetadata) error {
	switch typedObj := obj.(type) {
	case *v1.Route:
		typedObj.OpaqueMetadata = &v1.Route_MetadataStatic{
			MetadataStatic: objMetaToSourceMeta(meta),
		}
	case *v1.VirtualHost:
		typedObj.OpaqueMetadata = &v1.VirtualHost_MetadataStatic{
			MetadataStatic: objMetaToSourceMeta(meta),
		}
	case *v1.Listener:
		typedObj.OpaqueMetadata = &v1.Listener_MetadataStatic{
			MetadataStatic: objMetaToSourceMeta(meta),
		}
	default:
		return errors.Errorf("unimplemented object type: %T", obj)
	}
	return nil
}

func GetSourceMeta(obj ObjectWithMetadata) (*SourceMetadata, error) {
	if meta := obj.GetMetadataStatic(); meta != nil {
		return sourceMetaFromProto(meta), nil
	} else if metaDeprecated := obj.GetMetadata(); metaDeprecated != nil {
		return sourceMetaFromStruct(obj.GetMetadata())
	}
	return &SourceMetadata{}, nil
}

func appendSource(obj ObjectWithMetadata, source resources.InputResource) error {
	meta, err := GetSourceMeta(obj)
	if err != nil {
		return errors.Wrapf(err, "getting obj metadata")
	}
	meta.Sources = append(meta.Sources, makeSourceRef(source))
	return setObjMeta(obj, meta)
}

func ForEachSource(obj ObjectWithMetadata, fn func(source SourceRef) error) error {
	meta, err := GetSourceMeta(obj)
	if err != nil {
		return errors.Wrapf(err, "getting obj metadata")
	}
	for _, src := range meta.Sources {
		if err := fn(src); err != nil {
			return err
		}
	}
	return nil
}

func makeSourceRef(source resources.InputResource) SourceRef {
	return SourceRef{
		ResourceRef:        source.GetMetadata().Ref(),
		ResourceKind:       resources.Kind(source),
		ObservedGeneration: source.GetMetadata().GetGeneration(),
	}
}
