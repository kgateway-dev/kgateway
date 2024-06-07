package query

import (
	"fmt"

	solokubev1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1/kube/apis/gateway.solo.io/v1"
	"github.com/solo-io/gloo/projects/gateway2/wellknown"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

const (
	ListenerOptionTargetField = "listenerOption.targetRef"
)

func IterateIndices(f func(client.Object, string, client.IndexerFunc) error) error {
	return f(&solokubev1.ListenerOption{}, ListenerOptionTargetField, listenerOptionTargetRefIndexer)
}

func listenerOptionTargetRefIndexer(obj client.Object) []string {
	lisOpt, ok := obj.(*solokubev1.ListenerOption)
	if !ok {
		panic(fmt.Sprintf("wrong type %T provided to indexer. expected gateway.solo.io.ListenerOption", obj))
	}

	var res []string
	targetRefs := lisOpt.Spec.GetTargetRefs()
	if len(targetRefs) == 0 {
		return res
	}

	// use the first targetRef in the list as we only support one ref but have multiple in API for future-compatbility
	// TODO: fix this as part of https://github.com/solo-io/solo-projects/issues/6286
	targetRef := targetRefs[0]

	if targetRef == nil {
		return res
	}
	if targetRef.GetGroup() != gwv1.GroupName {
		return res
	}
	if targetRef.GetKind() != wellknown.GatewayKind {
		return res
	}

	ns := targetRef.GetNamespace().GetValue()
	if ns == "" {
		ns = lisOpt.GetNamespace()
	}
	targetNN := types.NamespacedName{
		Namespace: ns,
		Name:      targetRef.GetName(),
	}
	res = append(res, targetNN.String())
	return res
}
