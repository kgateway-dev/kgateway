package route_delegation

import (
	"fmt"
	"path/filepath"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/fsutils"
)

const (
	gatewayPort = 8080
)

// ref: common.yaml
var (
	commonManifest = filepath.Join(fsutils.MustGetThisDir(), "testdata", "common.yaml")

	// resources from common manifest
	httpbinTeam1Service = &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "svc1",
			Namespace: "team1",
		},
	}
	httpbinTeam1Deployment = &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "httpbin",
			Namespace: "team1",
		},
	}
	httpbinTeam2Service = &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "svc2",
			Namespace: "team2",
		},
	}
	httpbinTeam2Deployment = &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "httpbin",
			Namespace: "team2",
		},
	}
	gateway = &gwv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "http-gateway",
			Namespace: "infra",
		},
	}

	// resources produced by deployer when Gateway is applied
	proxyMeta = metav1.ObjectMeta{
		Name:      "http-gateway",
		Namespace: "infra",
	}
	proxyDeployment = &appsv1.Deployment{ObjectMeta: proxyMeta}
	proxyService    = &corev1.Service{ObjectMeta: proxyMeta}
	proxyHostPort   = fmt.Sprintf("%s.%s.svc:%d", proxyService.Name, proxyService.Namespace, gatewayPort)
)

// ref: basic.yaml
var (
	routeRoot = &gwv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "root",
			Namespace: "infra",
		},
	}
	routeTeam1 = &gwv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "svc1",
			Namespace: "team1",
		},
	}
	routeTeam2 = &gwv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "svc2",
			Namespace: "team2",
		},
	}
	routeParent1 = &gwv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "parent1",
			Namespace: "infra",
		},
	}
	routeParent1Host = "parent1.com"
	routeParent2Host = "parent2.com"
	routeParent2     = &gwv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "parent2",
			Namespace: "infra",
		},
	}
	pathTeam1 = "anything/team1/foo"
	pathTeam2 = "anything/team2/foo"
)

// ref: invalid_child_valid_standalone.yaml
var (
	gatewayTestPort = 8090

	proxyTestMeta = metav1.ObjectMeta{
		Name:      "http-gateway-test",
		Namespace: "infra",
	}
	proxyTestDeployment = &appsv1.Deployment{ObjectMeta: proxyTestMeta}
	proxyTestService    = &corev1.Service{ObjectMeta: proxyTestMeta}

	proxyTestHostPort = fmt.Sprintf("%s.%s.svc:%d", proxyTestService.Name, proxyTestService.Namespace, gatewayTestPort)

	routeParentHost = "parent.com"
	routeTeam2Host  = "team2.com"
)

var (
	basicRoutesManifest                 = filepath.Join(fsutils.MustGetThisDir(), "testdata", "basic.yaml")
	recursiveRoutesManifest             = filepath.Join(fsutils.MustGetThisDir(), "testdata", "recursive.yaml")
	cyclicRoutesManifest                = filepath.Join(fsutils.MustGetThisDir(), "testdata", "cyclic.yaml")
	invalidChildRoutesManifest          = filepath.Join(fsutils.MustGetThisDir(), "testdata", "invalid_child.yaml")
	headerQueryMatchRoutesManifest      = filepath.Join(fsutils.MustGetThisDir(), "testdata", "header_query_match.yaml")
	multipleParentsManifest             = filepath.Join(fsutils.MustGetThisDir(), "testdata", "multiple_parents.yaml")
	invalidChildValidStandaloneManifest = filepath.Join(fsutils.MustGetThisDir(), "testdata", "invalid_child_valid_standalone.yaml")
	unresolvedChildManifest             = filepath.Join(fsutils.MustGetThisDir(), "testdata", "unresolved_child.yaml")
	matcherInheritanceManifest          = filepath.Join(fsutils.MustGetThisDir(), "testdata", "matcher_inheritance.yaml")
	routeWeightManifest                 = filepath.Join(fsutils.MustGetThisDir(), "testdata", "route_weight.yaml")
)
