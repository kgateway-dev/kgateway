package controller

import (
	"errors"
	"fmt"

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	apiv1 "sigs.k8s.io/gateway-api/apis/v1"
	apiv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"
)

const (
	httpRouteTargetField    = "http-route-target"
	referenceGrantFromField = "ref-grant-from"
)

func IterateIndices(f func(client.Object, string, client.IndexerFunc) error) error {
	return errors.Join(
		f(&apiv1.HTTPRoute{}, httpRouteTargetField, httpRouteToTargetIndexer),
		f(&apiv1beta1.ReferenceGrant{}, referenceGrantFromField, refGrantFromIndexer),
	)
}

func httpRouteToTargetIndexer(obj client.Object) []string {
	hr, ok := obj.(*apiv1.HTTPRoute)
	if !ok {
		panic(fmt.Sprintf("wrong type %T provided to indexer. expected HTTPRoute", obj))
	}
	var parents []string
	for _, pRef := range hr.Spec.ParentRefs {
		if pRef.Group != nil && *pRef.Group != "gateway.networking.k8s.io" {
			continue
		}
		if pRef.Kind != nil && *pRef.Kind != kind(&apiv1.Gateway{}) {
			continue
		}
		ns := resolveNs(pRef.Namespace)
		if ns == "" {
			ns = hr.Namespace
		}
		nns := types.NamespacedName{
			Namespace: ns,
			Name:      string(pRef.Name),
		}
		parents = append(parents, nns.String())
	}
	return parents
}

func refGrantFromIndexer(obj client.Object) []string {
	rg, ok := obj.(*apiv1beta1.ReferenceGrant)
	if !ok {
		panic(fmt.Sprintf("wrong type %T provided to indexer. expected ReferenceGrant", obj))
	}
	var ns []string
	for _, from := range rg.Spec.From {
		if from.Namespace != "" {
			ns = append(ns, string(from.Namespace))
		}
	}
	return ns
}
