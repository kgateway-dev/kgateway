package translator

import (
	"context"

	knativev1alpha1 "knative.dev/serving/pkg/apis/networking/v1alpha1"
	v1 "github.com/solo-io/gloo/projects/clusteringress/pkg/api/v1"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/knative/pkg/translator"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
)

const (
	proxyName = "clusteringress-proxy"
)

func translateProxy(ctx context.Context, namespace string, snap *v1.TranslatorSnapshot) (*gloov1.Proxy, error) {
	ingresses := make(map[core.ResourceRef]knativev1alpha1.IngressSpec)
	for _, ing := range snap.Clusteringresses {
		ingresses[ing.GetMetadata().Ref()] = ing.Spec
	}
	return translator.TranslateProxyFromSpecs(ctx, proxyName, namespace, ingresses, snap.Secrets)
}
