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
	VirtualHostOptionTargetField = "virtualHostOption.targetRef"
)

func IterateIndices(f func(client.Object, string, client.IndexerFunc) error) error {
	return f(&solokubev1.VirtualHostOption{}, VirtualHostOptionTargetField, virtualHostOptionTargetRefIndexer)
}

func virtualHostOptionTargetRefIndexer(obj client.Object) []string {
	vhOpt, ok := obj.(*solokubev1.VirtualHostOption)
	if !ok {
		panic(fmt.Sprintf("wrong type %T provided to indexer. expected gateway.solo.io.VirtualHostOption", obj))
	}

	var res []string
	targetRef := vhOpt.Spec.GetTargetRef()
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
		ns = vhOpt.GetNamespace()
	}
	targetNN := types.NamespacedName{
		Namespace: ns,
		Name:      targetRef.GetName(),
	}
	res = append(res, targetNN.String())
	return res
}
