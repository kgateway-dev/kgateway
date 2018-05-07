package crd

import (
	"github.com/gogo/protobuf/proto"
	"github.com/pkg/errors"
	"github.com/solo-io/gloo/pkg/api/types/v1"
	"github.com/solo-io/gloo/pkg/protoutil"
	crdv1 "github.com/solo-io/gloo/pkg/storage/crd/solo.io/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func ConfigObjectToCrd(namespace string, item v1.ConfigObject) (metav1.Object, error) {
	name := item.GetName()
	var (
		ok     bool
	)
	var (
		resourceVersion string
		annotations     map[string]string
	)
	if item.GetMetadata() != nil {
		resourceVersion = item.GetMetadata().ResourceVersion
		if item.GetMetadata().Namespace != "" {
			namespace = item.GetMetadata().Namespace
		}
		annotations = item.GetMetadata().Annotations
	}

	// clone and remove fields
	var clone v1.ConfigObject
	switch item.(type) {
	case *v1.Upstream:
		clone, ok = proto.Clone(item).(*v1.Upstream)
		if !ok {
			return nil, errors.New("internal error: output of proto.Clone was not expected type")
		}
	case *v1.VirtualService:
		// clone and remove fields
		clone, ok = proto.Clone(item).(*v1.VirtualService)
		if !ok {
			return nil, errors.New("internal error: output of proto.Clone was not expected type")
		}
	default:
		panic(errors.Errorf("unknown type: %v", item))
	}
	clone.SetMetadata(nil)
	clone.SetName("")

	spec, err := protoutil.MarshalMap(clone)
	if err != nil {
		return nil, errors.Wrap(err, "failed to convert proto config object to map[string]interface{}")
	}
	copySpec := crdv1.Spec(spec)

	meta := metav1.ObjectMeta{
		Name:            name,
		Namespace:       namespace,
		ResourceVersion: resourceVersion,
		Annotations:     annotations,
	}

	var crdObject metav1.Object

	switch item.(type) {
	case *v1.Upstream:
		crdObject = &crdv1.Upstream{
			ObjectMeta: meta,
			Spec:       &copySpec,
		}
	case *v1.VirtualService:
		crdObject = &crdv1.VirtualService{
			ObjectMeta: meta,
			Spec:       &copySpec,
		}
	default:
		panic(errors.Errorf("unknown type: %v", item))
	}

	return crdObject, nil
}

func ConfigObjectFromCrd(objectMeta metav1.ObjectMeta,
	spec *crdv1.Spec,
	item v1.ConfigObject) error {
	if spec != nil {
		err := protoutil.UnmarshalMap(*spec, item)
		if err != nil {
			return errors.Wrap(err, "failed to convert crd spec to config object")
		}
	}
	// add removed fields to the internal object
	item.SetName(objectMeta.Name)
	item.SetMetadata(&v1.Metadata{
		ResourceVersion: objectMeta.ResourceVersion,
		Namespace:       objectMeta.Namespace,
		Annotations:     objectMeta.Annotations,
	})
	return nil
}
