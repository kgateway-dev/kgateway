package setup

import (
	"context"

	"github.com/solo-io/go-utils/contextutils"
	"github.com/solo-io/go-utils/kubeutils"
	"go.uber.org/zap"
	"k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	apiext "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	kubev1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
)

func MustKubeConfig(ctx context.Context) *rest.Config {
	kubecfg, err := kubeutils.GetConfig("", "")
	if err != nil {
		contextutils.LoggerFrom(ctx).Fatalw("Failed to get kubernetes config.", zap.Error(err))
	}
	return kubecfg
}

func MustSetV1ToServed(ctx context.Context) *v1beta1.CustomResourceDefinition {
	apiExts, err := apiext.NewForConfig(MustKubeConfig(ctx))
	if err != nil {
		contextutils.LoggerFrom(ctx).Fatalw("Failed to get kubernetes clientset.", zap.Error(err))
	}

	crd, err := apiExts.ApiextensionsV1beta1().CustomResourceDefinitions().Get("gateways.gateway.solo.io", kubev1.GetOptions{})
	if err != nil {
		contextutils.LoggerFrom(ctx).Fatalw("Failed to read gateway CRD", zap.Error(err))
	}

	foundV1 := false
	for i, v := range crd.Spec.Versions {
		if v.Name == "v1" {
			v.Served = true
			crd.Spec.Versions[i] = v
			foundV1 = true
			break
		}
	}
	if !foundV1 {
		newV1 := v1beta1.CustomResourceDefinitionVersion{
			Name:   "v1",
			Served: true,
		}
		crd.Spec.Versions = append(crd.Spec.Versions, newV1)
	}

	written, err := apiExts.ApiextensionsV1beta1().CustomResourceDefinitions().Update(crd)
	if err != nil {
		contextutils.LoggerFrom(ctx).Fatalw("Failed to set gateway CRD v1 to served", zap.Error(err))
	}
	return written
}

func MustSetV1ToNotServed(ctx context.Context, crd *v1beta1.CustomResourceDefinition) {
	apiExts, err := apiext.NewForConfig(MustKubeConfig(ctx))
	if err != nil {
		contextutils.LoggerFrom(ctx).Fatalw("Failed to get kubernetes clientset.", zap.Error(err))
	}

	for i, v := range crd.Spec.Versions {
		if v.Name == "v1" {
			v.Served = false
			crd.Spec.Versions[i] = v
			break
		}
	}
	_, err = apiExts.ApiextensionsV1beta1().CustomResourceDefinitions().Update(crd)
	if err != nil {
		contextutils.LoggerFrom(ctx).Fatalw("Failed to set gateway CRD v1 to not served.", zap.Error(err))
	}
}
