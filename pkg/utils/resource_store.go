package utils

import (
	"github.com/solo-io/solo-kit/pkg/api/v1/resources"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
)

type ResourceStore interface {
	Load(resources.ResourceList)

	Find(namespace, name string) resources.Resource
	Has(namespace, name string) bool
}

var _ ResourceStore = new(store)

type store struct {
	resources map[string]resources.Resource
}

func NewResourceStore() *store {
	return &store{
		resources: make(map[string]resources.Resource),
	}
}

func (s *store) Load(resourceList resources.ResourceList) {
	newResources := make(map[string]resources.Resource)
	for _, resource := range resourceList {
		resourceKey := s.resourceKey(resource.GetMetadata().GetNamespace(), resource.GetMetadata().GetName())
		newResources[resourceKey] = resource
	}
	s.resources = newResources
}

func (s *store) resourceKey(namespace, name string) string {
	return core.ResourceRef{
		Name:      name,
		Namespace: namespace,
	}.Key()
}

func (s *store) Find(namespace, name string) resources.Resource {
	return s.resources[s.resourceKey(namespace, name)]
}

func (s *store) Has(namespace, name string) bool {
	return s.Find(namespace, name) != nil
}
