package deployer

import (
	"context"
	"time"

	"istio.io/istio/pkg/kube/krt/krttest"
	"istio.io/istio/pkg/test"
	"istio.io/istio/pkg/util/smallset"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	apixv1a1 "sigs.k8s.io/gateway-api/apisx/v1alpha1"

	_ "github.com/envoyproxy/go-control-plane/envoy/extensions/upstreams/http/v3"

	apisettings "github.com/kgateway-dev/kgateway/v2/api/settings"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/krtcollections"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/wellknown"
	sdk "github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/collections"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/krtutil"
)

func NewCommonCols(t test.Failer, initObjs ...client.Object) *collections.CommonCollections {
	ctx := context.Background()
	var anys []any
	for _, obj := range initObjs {
		anys = append(anys, obj)
	}
	mock := krttest.NewMock(t, anys)

	policies := krtcollections.NewPolicyIndex(krtutil.KrtOptions{}, sdk.ContributesPolicies{}, apisettings.Settings{})
	kubeRawGateways := krttest.GetMockCollection[*gwv1.Gateway](mock)
	kubeRawListenerSets := krttest.GetMockCollection[*apixv1a1.XListenerSet](mock)
	gatewayClasses := krttest.GetMockCollection[*gwv1.GatewayClass](mock)
	nsCol := krtcollections.NewNamespaceCollectionFromCol(ctx, krttest.GetMockCollection[*corev1.Namespace](mock), krtutil.KrtOptions{})

	krtopts := krtutil.NewKrtOptions(ctx.Done(), nil)

	gateways := krtcollections.NewGatewayIndex(krtopts, smallset.New(wellknown.DefaultGatewayControllerName), wellknown.DefaultGatewayControllerName, policies, kubeRawGateways, kubeRawListenerSets, gatewayClasses, nsCol)

	commonCols := &collections.CommonCollections{
		GatewayIndex: gateways,
	}

	for !kubeRawGateways.HasSynced() || !kubeRawListenerSets.HasSynced() || !gatewayClasses.HasSynced() {
		time.Sleep(time.Second / 10)
	}

	gateways.Gateways.WaitUntilSynced(ctx.Done())
	return commonCols
}
