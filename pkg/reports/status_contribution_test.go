package reports

import (
	"testing"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
)

func TestStatusContributionsFromReportMapSplitsAndTransfersOwnership(t *testing.T) {
	route := &gwv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{Name: "route", Namespace: "default", Generation: 7},
	}
	reportMap := NewReportMap()
	reporter := NewReporter(&reportMap)
	reporter.Route(route).ParentRef(&gwv1.ParentReference{Name: "gateway"})

	contributions := StatusContributionsFromReportMap(StatusSource{Kind: GatewayStatusSource, Name: "default/gateway"}, reportMap)
	require.Len(t, contributions, 1)
	contribution := contributions[0]
	require.Equal(t, wellknown.HTTPRouteGVK, contribution.Target.GroupVersionKind)
	require.Equal(t, types.NamespacedName{Namespace: "default", Name: "route"}, contribution.Target.NamespacedName)
	require.Same(t, reportMap.HTTPRoutes[contribution.Target.NamespacedName], contribution.Route,
		"splitting transfers ownership without cloning every report")
}

func TestStatusKeyIsVersionIndependent(t *testing.T) {
	nn := types.NamespacedName{Namespace: "default", Name: "route"}
	v1 := StatusTarget{
		GroupVersionKind: schema.GroupVersionKind{Group: gwv1.GroupName, Version: "v1", Kind: "HTTPRoute"},
		NamespacedName:   nn,
	}
	v1beta1 := StatusTarget{
		GroupVersionKind: schema.GroupVersionKind{Group: gwv1.GroupName, Version: "v1beta1", Kind: "HTTPRoute"},
		NamespacedName:   nn,
	}
	require.Equal(t, v1.Key(), v1beta1.Key())
}
