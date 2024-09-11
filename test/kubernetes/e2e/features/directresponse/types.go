package directresponse

import (
	"path/filepath"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/solo-io/skv2/codegen/util"
)

var (
	setupManifest                                = filepath.Join(util.MustGetThisDir(), "testdata", "setup.yaml")
	gatewayManifest                              = filepath.Join(util.MustGetThisDir(), "testdata", "gateway.yaml")
	basicDirectResposeManifests                  = filepath.Join(util.MustGetThisDir(), "testdata", "basic-direct-response.yaml")
	basicDelegationManifests                     = filepath.Join(util.MustGetThisDir(), "testdata", "basic-delegation-direct-response.yaml")
	invalidDelegationConflictingFiltersManifests = filepath.Join(util.MustGetThisDir(), "testdata", "invalid-delegation-conflicting-filters.yaml")
	invalidMissingRefManifests                   = filepath.Join(util.MustGetThisDir(), "testdata", "invalid-missing-ref-direct-response.yaml")
	invalidOverlappingFiltersManifests           = filepath.Join(util.MustGetThisDir(), "testdata", "invalid-overlapping-filters.yaml")
	invalidMultipleRouteActionsManifests         = filepath.Join(util.MustGetThisDir(), "testdata", "invalid-multiple-route-actions.yaml")
	invalidBackendRefFilterManifests             = filepath.Join(util.MustGetThisDir(), "testdata", "invalid-backendRef-filter.yaml")

	glooProxyObjectMeta = metav1.ObjectMeta{
		Name:      "gloo-proxy-gw",
		Namespace: "default",
	}
	proxyDeployment   = &appsv1.Deployment{ObjectMeta: glooProxyObjectMeta}
	proxyService      = &corev1.Service{ObjectMeta: glooProxyObjectMeta}
	httpbinDeployment = &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "httpbin",
			Namespace: "httpbin",
		},
	}
)
