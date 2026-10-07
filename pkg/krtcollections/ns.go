package krtcollections

import (
	"context"
	"maps"

	"istio.io/api/label"
	"istio.io/istio/pkg/kube/kclient"
	"istio.io/istio/pkg/kube/krt"
	corev1 "k8s.io/api/core/v1"

	"github.com/kgateway-dev/kgateway/v2/pkg/apiclient"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/krtutil"
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

func NewNamespaceCollection(ctx context.Context, cli apiclient.Client, krtOpts krtutil.KrtOptions) (krt.Collection[NamespaceMetadata], kclient.Client[*corev1.Namespace]) {
	// NOTE: Do not apply an ObjectFilter to namespaces as the discovery namespace ObjectFilter for other clients
	// requires all namespaces to be watched
	client := kclient.New[*corev1.Namespace](cli) //nolint:forbidigo // should not use filtered client
	col := krt.WrapClient(client, krtOpts.ToOptions("Namespaces")...)
	return NewNamespaceCollectionFromCol(ctx, col, krtOpts), client
}

func NewNamespaceCollectionFromCol(ctx context.Context, col krt.Collection[*corev1.Namespace], krtOpts krtutil.KrtOptions) krt.Collection[NamespaceMetadata] {
	return krt.NewCollection(col, func(ctx krt.HandlerContext, ns *corev1.Namespace) *NamespaceMetadata {
		return &NamespaceMetadata{
			Name:   ns.Name,
			Labels: ns.Labels,
		}
	}, krtOpts.ToOptions("NamespacesMetadata")...)
}

// SystemNetwork returns the network Istio assigns to a workload that carries no
// topology.istio.io/network label of its own: the value of that label on the
// Istio system namespace. Istio resolves a workload's network as its own label,
// else this system namespace label (else meshNetworks, which kgateway does not
// read). Returns "" when namespaces is nil or the label is unset.
func SystemNetwork(kctx krt.HandlerContext, namespaces krt.Collection[NamespaceMetadata], systemNamespace string) string {
	if namespaces == nil || systemNamespace == "" {
		return ""
	}
	ns := krt.FetchOne(kctx, namespaces, krt.FilterKey(systemNamespace))
	if ns == nil {
		return ""
	}
	return ns.Labels[label.TopologyNetwork.Name]
}
