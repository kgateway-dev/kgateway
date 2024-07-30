package iosnapshot

import (
	"cmp"
	"encoding/json"
	"slices"

	v1snap "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/gloosnapshot"

	crdv1 "github.com/solo-io/solo-kit/pkg/api/v1/clients/kube/crd/solo.io/v1"
)

// sortResources sorts resources by gvk, namespace, and name
func sortResources(resources []crdv1.Resource) {
	slices.SortStableFunc(resources, func(a, b crdv1.Resource) int {
		return cmp.Or(
			cmp.Compare(a.APIVersion, b.APIVersion),
			cmp.Compare(a.Kind, b.Kind),
			cmp.Compare(a.GetNamespace(), b.GetNamespace()),
			cmp.Compare(a.GetName(), b.GetName()),
		)
	})
}

// apiSnapshotToGenericMap converts an ApiSnapshot into a generic map
// Since maps do not guarantee ordering, we do not attempt to sort these resources, as we do four []crdv1.Resource
func apiSnapshotToGenericMap(snap *v1snap.ApiSnapshot) (map[string]interface{}, error) {
	genericMap := map[string]interface{}{}

	if snap == nil {
		return genericMap, nil
	}

	jsn, err := json.Marshal(snap)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(jsn, &genericMap); err != nil {
		return nil, err
	}
	return genericMap, nil
}
