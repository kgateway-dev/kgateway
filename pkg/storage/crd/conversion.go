package crd

import (
	"github.com/gogo/protobuf/proto"
	"github.com/pkg/errors"
	"github.com/solo-io/gloo/pkg/api/types/v1"
	"github.com/solo-io/gloo/pkg/protoutil"
	crdv1 "github.com/solo-io/gloo/pkg/storage/crd/solo.io/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func UpstreamToCrd(namespace string, upstream *v1.Upstream) (*crdv1.Upstream, error) {
	name := upstream.Name
	var status *v1.Status
	var ok bool
	if upstream.Status != nil {
		status, ok = proto.Clone(upstream.Status).(*v1.Status)
		if !ok {
			return nil, errors.New("internal error: output of proto.Clone was not expected type")
		}
	}
	var resourceVersion string
	var annotations map[string]string
	if upstream.Metadata != nil {
		resourceVersion = upstream.Metadata.ResourceVersion
		if upstream.Metadata.Namespace != "" {
			namespace = upstream.Metadata.Namespace
		}
		annotations = upstream.Metadata.Annotations
	}

	// clone and remove fields
	upstreamClone, ok := proto.Clone(upstream).(*v1.Upstream)
	if !ok {
		return nil, errors.New("internal error: output of proto.Clone was not expected type")
	}
	upstreamClone.Metadata = nil
	upstreamClone.Name = ""
	upstreamClone.Status = nil

	spec, err := protoutil.MarshalMap(upstreamClone)
	if err != nil {
		return nil, errors.Wrap(err, "failed to convert proto upstream to map[string]interface{}")
	}
	copySpec := crdv1.Spec(spec)

	return &crdv1.Upstream{
		ObjectMeta: metav1.ObjectMeta{
			Name:            name,
			Namespace:       namespace,
			ResourceVersion: resourceVersion,
			Annotations:     annotations,
		},
		Status: status,
		Spec:   &copySpec,
	}, nil
}

func UpstreamFromCrd(upstreamCrd *crdv1.Upstream) (*v1.Upstream, error) {
	var upstream v1.Upstream
	if upstreamCrd.Spec != nil {
		err := protoutil.UnmarshalMap(*upstreamCrd.Spec, &upstream)
		if err != nil {
			return nil, errors.Wrap(err, "failed to convert crd spec to upstream")
		}
	}
	// add removed fields to the internal object
	upstream.Name = upstreamCrd.Name
	upstream.Metadata = &v1.Metadata{
		ResourceVersion: upstreamCrd.ResourceVersion,
		Namespace:       upstreamCrd.Namespace,
		Annotations:     upstreamCrd.Annotations,
	}
	upstream.Status = upstreamCrd.Status
	return &upstream, nil
}

func VirtualServiceToCrd(namespace string, virtualService *v1.VirtualService) (*crdv1.VirtualService, error) {
	name := virtualService.Name
	var status *v1.Status
	var ok bool
	if virtualService.Status != nil {
		status, ok = proto.Clone(virtualService.Status).(*v1.Status)
		if !ok {
			return nil, errors.New("internal error: output of proto.Clone was not expected type")
		}
	}
	var resourceVersion string
	var annotations map[string]string
	if virtualService.Metadata != nil {
		resourceVersion = virtualService.Metadata.ResourceVersion
		if virtualService.Metadata.Namespace != "" {
			namespace = virtualService.Metadata.Namespace
		}
		annotations = virtualService.Metadata.Annotations
	}

	// clone and remove fields
	vServiceClone, ok := proto.Clone(virtualService).(*v1.VirtualService)
	if !ok {
		return nil, errors.New("internal error: output of proto.Clone was not expected type")
	}
	vServiceClone.Metadata = nil
	vServiceClone.Name = ""
	vServiceClone.Status = nil

	spec, err := protoutil.MarshalMap(vServiceClone)
	if err != nil {
		return nil, errors.Wrap(err, "failed to convert proto upstream to map[string]interface{}")
	}
	copySpec := crdv1.Spec(spec)

	return &crdv1.VirtualService{
		ObjectMeta: metav1.ObjectMeta{
			Name:            name,
			Namespace:       namespace,
			ResourceVersion: resourceVersion,
			Annotations:     annotations,
		},
		Status: status,
		Spec:   &copySpec,
	}, nil
}

func VirtualServiceFromCrd(vServiceCrd *crdv1.VirtualService) (*v1.VirtualService, error) {
	var virtualService v1.VirtualService
	if vServiceCrd.Spec != nil {
		err := protoutil.UnmarshalMap(*vServiceCrd.Spec, &virtualService)
		if err != nil {
			return nil, errors.Wrap(err, "failed to convert crd spec to virtualservice")
		}
	}
	// add removed fields to the internal object
	virtualService.Name = vServiceCrd.Name
	virtualService.Metadata = &v1.Metadata{
		ResourceVersion: vServiceCrd.ResourceVersion,
		Namespace:       vServiceCrd.Namespace,
		Annotations:     vServiceCrd.Annotations,
	}
	virtualService.Status = vServiceCrd.Status
	return &virtualService, nil
}
