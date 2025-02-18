//go:build ignore

package directresponse

import (
	"path/filepath"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/fsutils"
)

var (
	setupManifest                                = filepath.Join(fsutils.MustGetThisDir(), "testdata", "setup.yaml")
	gatewayManifest                              = filepath.Join(fsutils.MustGetThisDir(), "testdata", "gateway.yaml")
	basicDirectResposeManifests                  = filepath.Join(fsutils.MustGetThisDir(), "testdata", "basic-direct-response.yaml")
	basicDelegationManifests                     = filepath.Join(fsutils.MustGetThisDir(), "testdata", "basic-delegation-direct-response.yaml")
	invalidDelegationConflictingFiltersManifests = filepath.Join(fsutils.MustGetThisDir(), "testdata", "invalid-delegation-conflicting-filters.yaml")
	invalidMissingRefManifests                   = filepath.Join(fsutils.MustGetThisDir(), "testdata", "invalid-missing-ref-direct-response.yaml")
	invalidOverlappingFiltersManifests           = filepath.Join(fsutils.MustGetThisDir(), "testdata", "invalid-overlapping-filters.yaml")
	invalidMultipleRouteActionsManifests         = filepath.Join(fsutils.MustGetThisDir(), "testdata", "invalid-multiple-route-actions.yaml")
	invalidBackendRefFilterManifests             = filepath.Join(fsutils.MustGetThisDir(), "testdata", "invalid-backendRef-filter.yaml")

	glooProxyObjectMeta = metav1.ObjectMeta{
		Name:      "gw",
		Namespace: "default",
	}
	gwRouteMeta = metav1.ObjectMeta{
		Name:      "gateway",
		Namespace: "default",
	}
	httpbinMeta = metav1.ObjectMeta{
		Name:      "httpbin",
		Namespace: "httpbin",
	}
	proxyDeployment   = &appsv1.Deployment{ObjectMeta: glooProxyObjectMeta}
	proxyService      = &corev1.Service{ObjectMeta: glooProxyObjectMeta}
	httpbinDeployment = &appsv1.Deployment{ObjectMeta: httpbinMeta}

	gwRoute = &gwv1.HTTPRoute{ObjectMeta: gwRouteMeta}
)
