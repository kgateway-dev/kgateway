package translator

import (
	"context"

	v1 "github.com/solo-io/gloo/projects/clusteringress/pkg/api/v1"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/knative/pkg/translator"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	knativev1alpha1 "knative.dev/serving/pkg/apis/networking/v1alpha1"
)

const (
	proxyName = "clusteringress-proxy"
)

func translateProxy(ctx context.Context, namespace string, snap *v1.TranslatorSnapshot) (*gloov1.Proxy, error) {
	// use map of *core.Metadata to support both Ingress and ClusterIngress,
	// which share the same Spec type
	ingresses := make(map[*core.Metadata]knativev1alpha1.IngressSpec)
	for _, ing := range snap.Clusteringresses {
		meta := ing.GetMetadata()
		ingresses[&meta] = ing.Spec
	}
	return translator.TranslateProxyFromSpecs(ctx, proxyName, namespace, ingresses)
}
