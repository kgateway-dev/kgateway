package trafficpolicy

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/kgateway"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/reports"
)

func gatewayExtensionForStatusTest(name string, generation int64) *kgateway.GatewayExtension {
	return &kgateway.GatewayExtension{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:  "default",
			Name:       name,
			Generation: generation,
		},
	}
}

// A contribution is the target/identity the writer keys on, so it must point at the
// GatewayExtension itself and the source must be scoped to it.
func TestGatewayExtensionStatusContributionTargetsTheExtension(t *testing.T) {
	ext := gatewayExtensionForStatusTest("my-ext", 4)

	contrib := gatewayExtensionStatusContribution(ext, nil)
	require.NotNil(t, contrib)
	require.Equal(t, wellknown.GatewayExtensionGVK.GroupKind(), contrib.Target.GroupKind)
	require.Equal(t, types.NamespacedName{Namespace: "default", Name: "my-ext"}, contrib.Target.NamespacedName)
	require.Equal(t, reports.GatewayExtensionStatusSource, contrib.Source.Kind)
	require.Equal(t, "default/my-ext", contrib.Source.Name)
}

func TestGatewayExtensionStatusContributionAccepted(t *testing.T) {
	ext := gatewayExtensionForStatusTest("my-ext", 4)

	contrib := gatewayExtensionStatusContribution(ext, nil)
	require.NotNil(t, contrib.GatewayExtension)
	require.Len(t, contrib.GatewayExtension.Conditions, 1)

	cond := contrib.GatewayExtension.Conditions[0]
	require.Equal(t, string(kgateway.GatewayExtensionConditionAccepted), cond.Type)
	require.Equal(t, metav1.ConditionTrue, cond.Status)
	require.Equal(t, string(kgateway.GatewayExtensionReasonAccepted), cond.Reason)
}

func TestGatewayExtensionStatusContributionInvalidCarriesTheError(t *testing.T) {
	ext := gatewayExtensionForStatusTest("my-ext", 4)

	contrib := gatewayExtensionStatusContribution(ext, errors.New("unknown extension type"))
	require.NotNil(t, contrib.GatewayExtension)
	require.Len(t, contrib.GatewayExtension.Conditions, 1)

	cond := contrib.GatewayExtension.Conditions[0]
	require.Equal(t, string(kgateway.GatewayExtensionConditionAccepted), cond.Type)
	require.Equal(t, metav1.ConditionFalse, cond.Status)
	require.Equal(t, string(kgateway.GatewayExtensionReasonInvalid), cond.Reason)
	require.Contains(t, cond.Message, "unknown extension type")
}
