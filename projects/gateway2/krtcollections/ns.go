package krtcollections

import (
	"context"
	"maps"

	"istio.io/istio/pkg/kube"
	"istio.io/istio/pkg/kube/kclient"
	"istio.io/istio/pkg/kube/krt"
	corev1 "k8s.io/api/core/v1"
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

func NewNamespaceCollection(ctx context.Context, istioClient kube.Client, dbg *krt.DebugHandler) krt.Collection[NamespaceMetadata] {
	client := kclient.NewFiltered[*corev1.Namespace](istioClient, kclient.Filter{
		ObjectTransform: kube.StripPodUnusedFields,
	})
	col := krt.WrapClient(client, krt.WithName("Namespaces"), krt.WithDebugging(dbg))
	return krt.NewCollection(col, func(ctx krt.HandlerContext, ns *corev1.Namespace) *NamespaceMetadata {
		return &NamespaceMetadata{
			Name:   ns.Name,
			Labels: ns.Labels,
		}
	})
}
