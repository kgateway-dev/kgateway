package kubeconverters

import (
	"context"
	"fmt"

	errors "github.com/rotisserie/eris"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"

	"github.com/solo-io/solo-kit/pkg/api/v1/resources"

	skcfgmap "github.com/solo-io/solo-kit/pkg/api/v1/clients/configmap"
	kubev1 "k8s.io/api/core/v1"

	skkubeutils "github.com/solo-io/solo-kit/pkg/utils/kubeutils"
	skprotoutils "github.com/solo-io/solo-kit/pkg/utils/protoutils"
)

func NewKubeConfigMapConverter() *kubeConverter {
	return &kubeConverter{}
}

type kubeConverter struct{}

func (cc *kubeConverter) FromKubeConfigMap(_ context.Context, rc *skcfgmap.ResourceClient, configMap *kubev1.ConfigMap) (resources.Resource, error) {
	return cc.FromKubeConfigMapWithResource(rc.NewResource(), rc.Kind(), configMap)
}

func (cc *kubeConverter) FromKubeConfigMapWithResource(resource resources.Resource, kind string, configMap *kubev1.ConfigMap) (resources.Resource, error) {
	artifact, ok := resource.(*v1.Artifact)
	if !ok {
		// should never happen
		return nil, errors.Errorf("expected [artifact] resource, got: [%s]", kind)
	}

	artifact.Data = configMap.Data
	artifact.SetMetadata(skkubeutils.FromKubeMeta(configMap.ObjectMeta))

	return artifact, nil
}

func (cc *kubeConverter) ToKubeConfigMap(ctx context.Context, rc *skcfgmap.ResourceClient, resource resources.Resource) (*kubev1.ConfigMap, error) {
	return cc.ToKubeConfigMapSimple(ctx, resource)
}

func (cc *kubeConverter) ToKubeConfigMapSimple(ctx context.Context, resource resources.Resource) (*kubev1.ConfigMap, error) {

	resourceMap, err := skprotoutils.MarshalMapEmitZeroValues(resource)
	if err != nil {
		return nil, errors.Wrapf(err, "marshalling resource as map")
	}
	configMapData := make(map[string]string)
	if dataObj, ok := resourceMap["data"]; ok {
		if data, ok := dataObj.(map[string]interface{}); ok {
			for k, v := range data {
				if stringV, ok := v.(string); ok {
					configMapData[k] = stringV
				} else {
					return nil, fmt.Errorf("resource data value %s of type %T is not a string", k, v)
				}
			}
		} else {
			return nil, fmt.Errorf("resource data is not map[string]interface{}")
		}
	} else {
		return nil, fmt.Errorf("resource has no data field")
	}

	meta := skkubeutils.ToKubeMeta(resource.GetMetadata())
	return &kubev1.ConfigMap{
		ObjectMeta: meta,
		Data:       configMapData,
	}, nil
}
