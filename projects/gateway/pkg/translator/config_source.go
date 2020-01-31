package translator

import (
	"encoding/json"

	"github.com/gogo/protobuf/jsonpb"
	"github.com/gogo/protobuf/types"
	errors "github.com/rotisserie/eris"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/go-utils/protoutils"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
)

type SourceMetadata struct {
	Sources []SourceRef `json:"sources"`
}

type SourceRef struct {
	core.ResourceRef
	ResourceKind       string `json:"kind"`
	ObservedGeneration int64  `json:"observedGeneration"`
}

type ObjectWithMetadata interface {
	GetMetadata() *types.Struct
}

func SourceMetaFromStruct(s *types.Struct) (*SourceMetadata, error) {
	if s == nil {
		return nil, nil
	}
	var m SourceMetadata
	if err := protoutils.UnmarshalStruct(s, &m); err != nil {
		return nil, err
	}
	return &m, nil
}

func objMetaToStruct(meta *SourceMetadata) (*types.Struct, error) {
	data, err := json.Marshal(meta)
	var pb types.Struct
	err = jsonpb.UnmarshalString(string(data), &pb)
	return &pb, err
}

func setObjMeta(obj ObjectWithMetadata, meta *SourceMetadata) error {
	metaStruct, err := objMetaToStruct(meta)
	if err != nil {
		return err
	}
	switch obj := obj.(type) {
	case *v1.Route:
		obj.Metadata = metaStruct
	case *v1.VirtualHost:
		obj.Metadata = metaStruct
	case *v1.Listener:
		obj.Metadata = metaStruct
	default:
		return errors.Errorf("unimplemented object type: %T", obj)
	}
	return nil
}

func GetSourceMeta(obj ObjectWithMetadata) (*SourceMetadata, error) {
	if obj.GetMetadata() == nil {
		return &SourceMetadata{}, nil
	}
	return SourceMetaFromStruct(obj.GetMetadata())
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
		ObservedGeneration: source.GetMetadata().Generation,
	}
}
