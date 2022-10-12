package nsselect

import (
	"context"
	"fmt"

	"github.com/solo-io/go-utils/contextutils"

	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
)

// If we are selecting resources by their name and the namespace in which they
// are installed, displayName and displayNamespace are identical to the
// resourceRef. However, meshes are selected by the ns in which they were
// installed, so we need both representations
// NOTE: if we add select helper utils for other resources we should make a
// general "select by resource ref" util
type ResSelect struct {
	displayName      string
	displayNamespace string
	resourceRef      core.ResourceRef
}

type ResMap map[string]ResSelect

func generateCommonResourceSelectOptions(typeName string, nsrMap NsResourceMap) ([]string, ResMap) {

	var resOptions []string
	// map the key to the res select object
	// key is namespace, name
	resMap := make(ResMap)

	for namespace, nsr := range nsrMap {
		var resArray []string
		switch typeName {
		case "secret":
			resArray = nsr.Secrets
		case "upstream":
			resArray = nsr.Upstreams
		default:
			contextutils.LoggerFrom(context.Background()).DPanic(fmt.Errorf("resource type %v not recognized", typeName))
			return nil, nil
		}
		for _, res := range resArray {
			selectMenuString := fmt.Sprintf("%v, %v", namespace, res)
			resOptions = append(resOptions, selectMenuString)
			resMap[selectMenuString] = ResSelect{
				displayName:      res,
				displayNamespace: namespace,
				resourceRef: core.ResourceRef{
					Name:      res,
					Namespace: namespace,
				},
			}
		}
	}
	return resOptions, resMap
}
