package utils

import (
	"sort"

	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	sk_resources "github.com/solo-io/solo-kit/pkg/api/v1/resources"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	"k8s.io/apimachinery/pkg/util/sets"
)

// Merges the modified resources into the existing resources, overwriting any existing values,
// and returns the new list
func MergeResourceLists(existingResources sk_resources.ResourceList, modifiedResources sk_resources.ResourceList) sk_resources.ResourceList {
	// make a map from resource ref key (i.e. "<namespace>.<name>") to resource
	resourceMap := make(sk_resources.ResourcesById)
	for _, resource := range existingResources {
		resourceMap[resource.GetMetadata().Ref().Key()] = resource
	}
	for _, resource := range modifiedResources {
		resourceMap[resource.GetMetadata().Ref().Key()] = resource
	}

	// convert map back into a list
	var resourceList sk_resources.ResourceList
	for _, resource := range resourceMap {
		resourceList = append(resourceList, resource)
	}

	sort.SliceStable(resourceList, func(i, j int) bool {
		return resourceList[i].GetMetadata().Less(resourceList[j].GetMetadata())
	})
	return resourceList
}

// Deletes the resources with the given refs from the list, and returns the updated list.
func DeleteResources(existingResources sk_resources.ResourceList, refsToDelete []*core.ResourceRef) sk_resources.ResourceList {
	// add all the deleted resource ref keys to a set for lookup
	deletedResourceKeys := sets.NewString()
	for _, ref := range refsToDelete {
		deletedResourceKeys.Insert(ref.Key())
	}

	var resourceList sk_resources.ResourceList
	// for each existing resource, only add it to the new list if it doesn't appear in the deleted ref set
	for _, resource := range existingResources {
		if !deletedResourceKeys.Has(resource.GetMetadata().Ref().Key()) {
			resourceList = append(resourceList, resource)
		}
	}

	sort.SliceStable(resourceList, func(i, j int) bool {
		return resourceList[i].GetMetadata().Less(resourceList[j].GetMetadata())
	})
	return resourceList
}

func UpstreamsToResourceList(upstreams []*gloov1.Upstream) sk_resources.ResourceList {
	var upstreamList gloov1.UpstreamList
	for _, upstream := range upstreams {
		upstreamList = append(upstreamList, upstream)
	}
	return upstreamList.AsResources()
}

func ResourceListToUpstreamList(resourceList sk_resources.ResourceList) gloov1.UpstreamList {
	var upstreamList gloov1.UpstreamList
	for _, resource := range resourceList {
		upstreamList = append(upstreamList, resource.(*gloov1.Upstream))
	}
	return upstreamList
}

func ResourceListToSecretList(resourceList sk_resources.ResourceList) gloov1.SecretList {
	var secretList gloov1.SecretList
	for _, resource := range resourceList {
		secretList = append(secretList, resource.(*gloov1.Secret))
	}
	return secretList
}
