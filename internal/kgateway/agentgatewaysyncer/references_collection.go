package agentgatewaysyncer

import (
	"fmt"

	creds "istio.io/istio/pilot/pkg/model/credentials"
	"istio.io/istio/pkg/config"
	"istio.io/istio/pkg/kube/krt"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	gwv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"

	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/utils/krtutil"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/wellknown"
)

// Reference stores a reference to a namespaced GVK, as used by ReferencePolicy
type Reference struct {
	Kind      schema.GroupVersionKind
	Namespace gwv1beta1.Namespace
}

func (refs Reference) String() string {
	return refs.Kind.String() + "/" + string(refs.Namespace)
}

type ReferencePair struct {
	To, From Reference
}

func (r ReferencePair) String() string {
	return fmt.Sprintf("%s->%s", r.To, r.From)
}

type ReferenceGrants struct {
	collection krt.Collection[ReferenceGrant]
	index      krt.Index[ReferencePair, ReferenceGrant]
}

func ReferenceGrantsCollection(referenceGrants krt.Collection[*gwv1beta1.ReferenceGrant], krtopts krtutil.KrtOptions) krt.Collection[ReferenceGrant] {
	return krt.NewManyCollection(referenceGrants, func(ctx krt.HandlerContext, obj *gwv1beta1.ReferenceGrant) []ReferenceGrant {
		rp := obj.Spec
		results := make([]ReferenceGrant, 0, len(rp.From)*len(rp.To))
		for _, from := range rp.From {
			fromKey := Reference{
				Namespace: from.Namespace,
			}
			if string(from.Group) == wellknown.GatewayGVK.Group && string(from.Kind) == wellknown.GatewayKind {
				fromKey.Kind = wellknown.GatewayGVK
			} else if string(from.Group) == wellknown.HTTPRouteGVK.Group && string(from.Kind) == wellknown.HTTPRouteKind {
				fromKey.Kind = wellknown.HTTPRouteGVK
			} else if string(from.Group) == wellknown.GRPCRouteGVK.Group && string(from.Kind) == wellknown.GRPCRouteKind {
				fromKey.Kind = wellknown.GRPCRouteGVK
			} else if string(from.Group) == wellknown.TLSRouteGVK.Group && string(from.Kind) == wellknown.TLSRouteKind {
				fromKey.Kind = wellknown.TLSRouteGVK
			} else if string(from.Group) == wellknown.TCPRouteGVK.Group && string(from.Kind) == wellknown.TCPRouteKind {
				fromKey.Kind = wellknown.TCPRouteGVK
			} else {
				// Not supported type. Not an error; may be for another controller
				continue
			}
			for _, to := range rp.To {
				toKey := Reference{
					Namespace: gwv1beta1.Namespace(obj.Namespace),
				}
				if to.Group == "" && string(to.Kind) == wellknown.SecretGVK.Kind {
					toKey.Kind = wellknown.SecretGVK
				} else if to.Group == "" && string(to.Kind) == wellknown.ServiceKind {
					toKey.Kind = wellknown.ServiceGVK
				} else {
					// Not supported type. Not an error; may be for another controller
					continue
				}
				rg := ReferenceGrant{
					Source:      config.NamespacedName(obj),
					From:        fromKey,
					To:          toKey,
					AllowAll:    false,
					AllowedName: "",
				}
				if to.Name != nil {
					rg.AllowedName = string(*to.Name)
				} else {
					rg.AllowAll = true
				}
				results = append(results, rg)
			}
		}
		return results
	}, krtopts.ToOptions("ReferenceGrants")...)
}

func BuildReferenceGrants(collection krt.Collection[ReferenceGrant]) ReferenceGrants {
	idx := krt.NewIndex(collection, "refgrant", func(o ReferenceGrant) []ReferencePair {
		return []ReferencePair{{
			To:   o.To,
			From: o.From,
		}}
	})
	return ReferenceGrants{
		collection: collection,
		index:      idx,
	}
}

type ReferenceGrant struct {
	Source      types.NamespacedName
	From        Reference
	To          Reference
	AllowAll    bool
	AllowedName string
}

func (g ReferenceGrant) ResourceName() string {
	return g.Source.String() + "/" + g.From.String() + "/" + g.To.String()
}

func (refs ReferenceGrants) SecretAllowed(ctx krt.HandlerContext, resourceName string, namespace string) bool {
	p, err := creds.ParseResourceName(resourceName, "", "", "")
	if err != nil {
		logger.Warn("failed to parse resource name", "resource_name", resourceName, "error", err)
		return false
	}
	from := Reference{Kind: wellknown.GatewayGVK, Namespace: gwv1beta1.Namespace(namespace)}
	to := Reference{Kind: wellknown.SecretGVK, Namespace: gwv1beta1.Namespace(p.Namespace)}
	pair := ReferencePair{From: from, To: to}
	grants := krt.Fetch(ctx, refs.collection, krt.FilterIndex(refs.index, pair))
	for _, g := range grants {
		if g.AllowAll || g.AllowedName == p.Name {
			return true
		}
	}
	return false
}

func (refs ReferenceGrants) BackendAllowed(ctx krt.HandlerContext,
	k schema.GroupVersionKind,
	backendName gwv1beta1.ObjectName,
	backendNamespace gwv1beta1.Namespace,
	routeNamespace string,
) bool {
	from := Reference{Kind: k, Namespace: gwv1beta1.Namespace(routeNamespace)}
	to := Reference{Kind: wellknown.SecretGVK, Namespace: backendNamespace}
	pair := ReferencePair{From: from, To: to}
	grants := krt.Fetch(ctx, refs.collection, krt.FilterIndex(refs.index, pair))
	for _, g := range grants {
		if g.AllowAll || g.AllowedName == string(backendName) {
			return true
		}
	}
	return false
}
