package kubeconverters

import (
	"context"
	"fmt"

	"k8s.io/client-go/tools/leaderelection/resourcelock"

	errors "github.com/rotisserie/eris"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"

	"github.com/solo-io/solo-kit/pkg/api/v1/resources"

	skcfgmap "github.com/solo-io/solo-kit/pkg/api/v1/clients/configmap"
	kubev1 "k8s.io/api/core/v1"

	skkubeutils "github.com/solo-io/solo-kit/pkg/utils/kubeutils"
	skprotoutils "github.com/solo-io/solo-kit/pkg/utils/protoutils"
)

func NewArtifactConverter() skcfgmap.ConfigMapConverter {
	return &converter{}
}

type converter struct{}

func (c *converter) FromKubeConfigMap(_ context.Context, rc *skcfgmap.ResourceClient, configMap *kubev1.ConfigMap) (resources.Resource, error) {
	if _, ok := rc.NewResource().(*v1.Artifact); !ok {
		// should never happen
		return nil, errors.Errorf("expected [artifact] resource client, got: [%s]", rc.Kind())
	}

	if _, ok := configMap.GetAnnotations()[resourcelock.LeaderElectionRecordAnnotationKey]; ok {
		// This is a temporary (hacky) solution
		// ConfigMaps are used by our Kubernetes LeaderElection implementation to store that state of the
		// current leader. This means that every election (2s interval) the ConfigMap is updated.
		// Gloo Edge will recognize this change, and re-process the state of the world.
		// The result is that the system does not become consistent.
		//
		// A preferred solution would be to filter out this resouce (either by annotation or label)
		// Solo-Kit only supports inclusive label selection, not exclusive label selection.
		// In a follow-up, we will extend the filtering of solo-kit to enable filtering out certain
		// labels. But for now, we return nil, meaning that this resource will be skipped by our Watch.
		return nil, nil
	}

	return KubeConfigMapToArtifact(configMap), nil
}

func KubeConfigMapToArtifact(configMap *kubev1.ConfigMap) *v1.Artifact {
	artifact := new(v1.Artifact)
	artifact.Data = configMap.Data
	artifact.SetMetadata(skkubeutils.FromKubeMeta(configMap.ObjectMeta, true))

	return artifact
}

func (c *converter) ToKubeConfigMap(_ context.Context, rc *skcfgmap.ResourceClient, resource resources.Resource) (*kubev1.ConfigMap, error) {
	return ArtifactToKubeConfigMap(resource)
}

func ArtifactToKubeConfigMap(resource resources.Resource) (*kubev1.ConfigMap, error) {

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
