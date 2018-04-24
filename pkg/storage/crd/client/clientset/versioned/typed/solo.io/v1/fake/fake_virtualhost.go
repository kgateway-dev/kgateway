/*
Copyright 2018 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package fake

import (
	solo_io_v1 "github.com/solo-io/gloo/pkg/storage/crd/solo.io/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	labels "k8s.io/apimachinery/pkg/labels"
	schema "k8s.io/apimachinery/pkg/runtime/schema"
	types "k8s.io/apimachinery/pkg/types"
	watch "k8s.io/apimachinery/pkg/watch"
	testing "k8s.io/client-go/testing"
)

// FakeVirtualServices implements VirtualServiceInterface
type FakeVirtualServices struct {
	Fake *FakeGlooV1
	ns   string
}

var virtualServicesResource = schema.GroupVersionResource{Group: "gloo.solo.io", Version: "v1", Resource: "virtualservices"}

var virtualServicesKind = schema.GroupVersionKind{Group: "gloo.solo.io", Version: "v1", Kind: "VirtualService"}

// Get takes name of the virtualService, and returns the corresponding virtualService object, and an error if there is any.
func (c *FakeVirtualServices) Get(name string, options v1.GetOptions) (result *solo_io_v1.VirtualService, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewGetAction(virtualServicesResource, c.ns, name), &solo_io_v1.VirtualService{})

	if obj == nil {
		return nil, err
	}
	return obj.(*solo_io_v1.VirtualService), err
}

// List takes label and field selectors, and returns the list of VirtualServices that match those selectors.
func (c *FakeVirtualServices) List(opts v1.ListOptions) (result *solo_io_v1.VirtualServiceList, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewListAction(virtualServicesResource, virtualServicesKind, c.ns, opts), &solo_io_v1.VirtualServiceList{})

	if obj == nil {
		return nil, err
	}

	label, _, _ := testing.ExtractFromListOptions(opts)
	if label == nil {
		label = labels.Everything()
	}
	list := &solo_io_v1.VirtualServiceList{}
	for _, item := range obj.(*solo_io_v1.VirtualServiceList).Items {
		if label.Matches(labels.Set(item.Labels)) {
			list.Items = append(list.Items, item)
		}
	}
	return list, err
}

// Watch returns a watch.Interface that watches the requested virtualServices.
func (c *FakeVirtualServices) Watch(opts v1.ListOptions) (watch.Interface, error) {
	return c.Fake.
		InvokesWatch(testing.NewWatchAction(virtualServicesResource, c.ns, opts))

}

// Create takes the representation of a virtualService and creates it.  Returns the server's representation of the virtualService, and an error, if there is any.
func (c *FakeVirtualServices) Create(virtualService *solo_io_v1.VirtualService) (result *solo_io_v1.VirtualService, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewCreateAction(virtualServicesResource, c.ns, virtualService), &solo_io_v1.VirtualService{})

	if obj == nil {
		return nil, err
	}
	return obj.(*solo_io_v1.VirtualService), err
}

// Update takes the representation of a virtualService and updates it. Returns the server's representation of the virtualService, and an error, if there is any.
func (c *FakeVirtualServices) Update(virtualService *solo_io_v1.VirtualService) (result *solo_io_v1.VirtualService, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewUpdateAction(virtualServicesResource, c.ns, virtualService), &solo_io_v1.VirtualService{})

	if obj == nil {
		return nil, err
	}
	return obj.(*solo_io_v1.VirtualService), err
}

// Delete takes name of the virtualService and deletes it. Returns an error if one occurs.
func (c *FakeVirtualServices) Delete(name string, options *v1.DeleteOptions) error {
	_, err := c.Fake.
		Invokes(testing.NewDeleteAction(virtualServicesResource, c.ns, name), &solo_io_v1.VirtualService{})

	return err
}

// DeleteCollection deletes a collection of objects.
func (c *FakeVirtualServices) DeleteCollection(options *v1.DeleteOptions, listOptions v1.ListOptions) error {
	action := testing.NewDeleteCollectionAction(virtualServicesResource, c.ns, listOptions)

	_, err := c.Fake.Invokes(action, &solo_io_v1.VirtualServiceList{})
	return err
}

// Patch applies the patch and returns the patched virtualService.
func (c *FakeVirtualServices) Patch(name string, pt types.PatchType, data []byte, subresources ...string) (result *solo_io_v1.VirtualService, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewPatchSubresourceAction(virtualServicesResource, c.ns, name, data, subresources...), &solo_io_v1.VirtualService{})

	if obj == nil {
		return nil, err
	}
	return obj.(*solo_io_v1.VirtualService), err
}
