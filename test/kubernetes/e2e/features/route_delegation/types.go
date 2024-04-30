package route_delegation

import (
	"fmt"
	"path/filepath"

	"github.com/solo-io/skv2/codegen/util"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

const (
	// ref: test/kubernetes/e2e/features/delegation/inputs/common.yaml
	gatewayPort = 8080
)

// ref: test/kubernetes/e2e/features/delegation/inputs/common.yaml
var (
	commonManifest = filepath.Join(util.MustGetThisDir(), "inputs/common.yaml")
	proxyMeta      = metav1.ObjectMeta{
		Name:      "gloo-proxy-http-gateway",
		Namespace: "infra",
	}
	proxyDeployment = &appsv1.Deployment{ObjectMeta: proxyMeta}
	proxyService    = &corev1.Service{ObjectMeta: proxyMeta}
	proxyHostPort   = fmt.Sprintf("%s.%s.svc:%d", proxyService.Name, proxyService.Namespace, gatewayPort)

	httpbinTeam1 = &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "httpbin",
			Namespace: "team1",
		},
	}
	httpbinTeam2 = &appsv1.Deployment{
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
)

// ref: test/kubernetes/e2e/features/delegation/inputs/basic.yaml
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
	pathTeam1 = "anything/team1/foo"
	pathTeam2 = "anything/team2/foo"
)

var (
	basicRoutesManifest            = filepath.Join(util.MustGetThisDir(), "inputs/basic.yaml")
	recursiveRoutesManifest        = filepath.Join(util.MustGetThisDir(), "inputs/recursive.yaml")
	cyclicRoutesManifest           = filepath.Join(util.MustGetThisDir(), "inputs/cyclic.yaml")
	invalidChildRoutesManifest     = filepath.Join(util.MustGetThisDir(), "inputs/invalid_child.yaml")
	headerQueryMatchRoutesManifest = filepath.Join(util.MustGetThisDir(), "inputs/header_query_match.yaml")
)
