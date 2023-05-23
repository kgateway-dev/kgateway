package v1

import (
	"sort"

	"github.com/solo-io/gloo/projects/gloo/constants"
	plugins "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options"
)

/*
	These interfaces should be implemented by upstreams that support subset load balancing.
	https://github.com/envoyproxy/envoy/blob/main/source/docs/subset_load_balancer.md
*/

type SubsetSpecGetter interface {
	GetSubsetSpec() *plugins.SubsetSpec
}
type SubsetSpecSetter interface {
	SetSubsetSpec(*plugins.SubsetSpec)
}
type SubsetSpecMutator interface {
	SubsetSpecGetter
	SubsetSpecSetter
}

func (us *Upstream_Kube) GetSubsetSpec() *plugins.SubsetSpec {
	return us.Kube.GetSubsetSpec()
}

func (us *Upstream_Kube) SetSubsetSpec(spec *plugins.SubsetSpec) {
	us.Kube.SubsetSpec = spec
}

func (us *Upstream_Consul) GetSubsetSpec() *plugins.SubsetSpec {
	subsets := &plugins.SubsetSpec{}

	// Add a subset selector for data centers
	// This will cause Envoy to partition the endpoints by their data center
	var dataCenterMetadataKeys []string
	for _, dc := range us.Consul.GetDataCenters() {
		dataCenterMetadataKeys = append(dataCenterMetadataKeys, constants.ConsulDataCenterKeyPrefix+dc)
	}
	sort.Strings(dataCenterMetadataKeys)

	subsets.Selectors = append(subsets.GetSelectors(), &plugins.Selector{
		Keys: dataCenterMetadataKeys,
	})

	tags := us.Consul.GetSubsetTags()
	if len(tags) == 0 {
		tags = us.Consul.GetServiceTags()
	}

	if len(tags) > 0 {

		// If any tags are present, create a subset selector with the tags as key set
		// This will cause Envoy to partition the endpoints (service instances) by their tags
		var tagMetadataKeys []string
		for _, tag := range tags {
			tagMetadataKeys = append(tagMetadataKeys, constants.ConsulTagKeyPrefix+tag)
		}
		sort.Strings(tagMetadataKeys)

		subsets.Selectors = append(subsets.GetSelectors(), &plugins.Selector{
			Keys: tagMetadataKeys,
		})

		// Also create a subset selector with both the data center and the tag keys
		// This will cause Envoy to partition the endpoints by data center and by their tags
		var allKeys []string
		allKeys = append(allKeys, dataCenterMetadataKeys...)
		allKeys = append(allKeys, tagMetadataKeys...)
		subsets.Selectors = append(subsets.GetSelectors(), &plugins.Selector{
			Keys: allKeys,
		})
	}

	return subsets
}
