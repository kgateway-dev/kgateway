package controller

import (
	"errors"
	"fmt"

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	api "sigs.k8s.io/gateway-api/apis/v1beta1"
)

const (
	httpRouteTargetField    = "http-route-target"
	referenceGrantFromField = "ref-grant-from"
)

func IterateIndices(f func(client.Object, string, client.IndexerFunc) error) error {
	return errors.Join(
		f(&api.HTTPRoute{}, httpRouteTargetField, httpRouteToTargetIndexer),
		f(&api.ReferenceGrant{}, referenceGrantFromField, refGrantFromIndexer),
	)
}

func httpRouteToTargetIndexer(obj client.Object) []string {
	hr, ok := obj.(*api.HTTPRoute)
	if !ok {
		panic(fmt.Sprintf("wrong type %T provided to indexer. expected HTTPRoute", obj))
	}
	var parents []string
	for _, pRef := range hr.Spec.ParentRefs {
		if pRef.Kind == nil || *pRef.Kind == kind(&api.Gateway{}) {
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
	}
	return parents
}

func refGrantFromIndexer(obj client.Object) []string {
	rg, ok := obj.(*api.ReferenceGrant)
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
