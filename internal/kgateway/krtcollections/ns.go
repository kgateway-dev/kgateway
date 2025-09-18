package krtcollections

import (
	"context"
	"maps"

	"istio.io/istio/pkg/kube"
	"istio.io/istio/pkg/kube/kclient"
	"istio.io/istio/pkg/kube/krt"
	corev1 "k8s.io/api/core/v1"

	krtinternal "github.com/kgateway-dev/kgateway/v2/internal/kgateway/utils/krtutil"
)

type NamespaceMetadata struct {
	Name   string
	Labels map[string]string
}

func (n NamespaceMetadata) ResourceName() string {
	return n.Name
}

func (n NamespaceMetadata) Equals(in NamespaceMetadata) bool {
	return n.Name == in.Name && maps.Equal(n.Labels, in.Labels)
}

func NewNamespaceCollection(ctx context.Context, istioClient kube.Client, krtOpts krtinternal.KrtOptions) (krt.Collection[NamespaceMetadata], kclient.Client[*corev1.Namespace]) {
	client := kclient.NewFiltered[*corev1.Namespace](istioClient, kclient.Filter{
		// ObjectTransform: ...,
		// NOTE: Do not apply an ObjectFilter to namespaces as the discovery namespace ObjectFilter for other clients
		// requires all namespaces to be watched
	})
	col := krt.WrapClient(client, krtOpts.ToOptions("Namespaces")...)
	return NewNamespaceCollectionFromCol(ctx, col, krtOpts), client
}

func NewNamespaceCollectionFromCol(ctx context.Context, col krt.Collection[*corev1.Namespace], krtOpts krtinternal.KrtOptions) krt.Collection[NamespaceMetadata] {
	return krt.NewCollection(col, func(ctx krt.HandlerContext, ns *corev1.Namespace) *NamespaceMetadata {
		return &NamespaceMetadata{
			Name:   ns.Name,
			Labels: ns.Labels,
		}
	}, krtOpts.ToOptions("NamespacesMetadata")...)
}
