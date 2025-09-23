package backendtlspolicy

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	gwv1alpha3 "sigs.k8s.io/gateway-api/apis/v1alpha3"

	sdk "github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk"
)

func getPolicyStatusFn(
	cl client.Client,
) sdk.GetPolicyStatusFn {
	return func(ctx context.Context, nn types.NamespacedName) (gwv1alpha2.PolicyStatus, error) {
		res := gwv1alpha3.BackendTLSPolicy{}
		err := cl.Get(ctx, nn, &res)
		if err != nil {
			return gwv1alpha2.PolicyStatus{}, err
		}
		return res.Status, nil
	}
}

func patchPolicyStatusFn(
	cl client.Client,
) sdk.PatchPolicyStatusFn {
	return func(ctx context.Context, nn types.NamespacedName, policyStatus gwv1alpha2.PolicyStatus) error {
		res := gwv1alpha3.BackendTLSPolicy{}
		err := cl.Get(ctx, nn, &res)
		if err != nil {
			return err
		}

		res.Status = policyStatus
		if err := cl.Status().Patch(ctx, &res, client.Merge); err != nil {
			return fmt.Errorf("error updating status for TrafficPolicy %s: %w", nn.String(), err)
		}
		return nil
	}
}
