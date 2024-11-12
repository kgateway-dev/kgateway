package query

import (
	"errors"

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwv1a2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	gwv1b1 "sigs.k8s.io/gateway-api/apis/v1beta1"

	"github.com/solo-io/gloo/projects/gateway2/wellknown"
)

const (
	HttpRouteTargetField    = "http-route-target"
	TcpRouteTargetField     = "tcp-route-target"
	ReferenceGrantFromField = "ref-grant-from"
)

// IterateIndices calls the provided function for each indexable object with the appropriate indexer function.
func IterateIndices(f func(client.Object, string, client.IndexerFunc) error) error {
	return errors.Join(
		f(&gwv1.HTTPRoute{}, HttpRouteTargetField, IndexerByObjType),
		f(&gwv1a2.TCPRoute{}, TcpRouteTargetField, IndexerByObjType),
		f(&gwv1b1.ReferenceGrant{}, ReferenceGrantFromField, IndexerByObjType),
	)
}

// IndexerByObjType indexes objects based on the provided object type. The following object types are supported:
//
//   - HTTPRoute
//   - TCPRoute
//   - ReferenceGrant
func IndexerByObjType(obj client.Object) []string {
	var results []string

	switch resource := obj.(type) {
	case *gwv1.HTTPRoute:
		for _, pRef := range resource.Spec.ParentRefs {
			if pRef.Group != nil && *pRef.Group != gwv1.GroupName {
				continue
			}
			if pRef.Kind != nil && *pRef.Kind != wellknown.GatewayKind {
				continue
			}
			ns := resolveNs(pRef.Namespace)
			if ns == "" {
				ns = resource.Namespace
			}
			nns := types.NamespacedName{
				Namespace: ns,
				Name:      string(pRef.Name),
			}
			results = append(results, nns.String())
		}
	case *gwv1a2.TCPRoute:
		for _, pRef := range resource.Spec.ParentRefs {
			if pRef.Group != nil && *pRef.Group != gwv1a2.GroupName {
				continue
			}
			if pRef.Kind != nil && *pRef.Kind != wellknown.GatewayKind {
				continue
			}
			ns := resolveNs(pRef.Namespace)
			if ns == "" {
				ns = resource.Namespace
			}
			nns := types.NamespacedName{
				Namespace: ns,
				Name:      string(pRef.Name),
			}
			results = append(results, nns.String())
		}
	case *gwv1b1.ReferenceGrant:
		for _, from := range resource.Spec.From {
			if from.Namespace != "" {
				results = append(results, string(from.Namespace))
			}
		}
	default:
		// Unsupported route type
		return results
	}

	return results
}

// resolveNs resolves the namespace from an optional Namespace field.
func resolveNs(ns *gwv1.Namespace) string {
	if ns == nil {
		return ""
	}
	return string(*ns)
}
