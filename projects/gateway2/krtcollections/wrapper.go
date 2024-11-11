package krtcollections

import (
	"google.golang.org/protobuf/proto"

	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	core "github.com/solo-io/solo-kit/pkg/api/v1/resources/core"

	"istio.io/istio/pkg/kube/krt"
)

type (
	// UpstreamWrapper provides a keying function for Gloo's `v1.Upstream`
	UpstreamWrapper = ResourceWrapper[*gloov1.Upstream]
)

type GlooResource interface {
	proto.Message
	interface {
		GetMetadata() *core.Metadata
	}
}

type ResourceWrapper[T GlooResource] struct {
	Inner T
}

func (us ResourceWrapper[T]) ResourceName() string {
	return krt.Named{
		Name:      us.Inner.GetMetadata().GetName(),
		Namespace: us.Inner.GetMetadata().GetNamespace(),
	}.ResourceName()
}

func (us ResourceWrapper[T]) String() string {
	return us.ResourceName()
}

func (us ResourceWrapper[T]) Equals(in ResourceWrapper[T]) bool {
	return proto.Equal(us.Inner, in.Inner)
}

func (us ResourceWrapper[T]) GetMetadata() *core.Metadata {
	return us.Inner.GetMetadata()
}

// equivalent of var _ Interface = Struct{} for generics
func _genericTypeAssert[T GlooResource]() (krt.ResourceNamer, krt.Equaler[ResourceWrapper[T]]) {
	return ResourceWrapper[T]{}, ResourceWrapper[T]{}
}
