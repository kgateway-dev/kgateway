package filtertests

import (
	"context"
	"log"

	"google.golang.org/protobuf/proto"

	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/projects/gateway2/translator/plugins"
	"github.com/solo-io/gloo/projects/gateway2/translator/testutils"

	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

func AssertExpectedRoute(
	plugin plugins.RoutePlugin,
	expectedRoute *v1.Route,
	logActual bool,
	filters ...gwv1.HTTPRouteFilter,
) {
	outputRoute := &v1.Route{
		Options: &v1.RouteOptions{},
	}
	assertExpectedRoute(plugin, outputRoute, expectedRoute, nil, logActual, filters...)
}

func AssertExpectedRouteWithMatch(
	plugin plugins.RoutePlugin,
	outputRoute *v1.Route,
	expectedRoute *v1.Route,
	match *gwv1.HTTPRouteMatch,
	logActual bool,
	filters ...gwv1.HTTPRouteFilter,
) {
	assertExpectedRoute(plugin, outputRoute, expectedRoute, match, logActual, filters...)
}

func assertExpectedRoute(
	plugin plugins.RoutePlugin,
	outputRoute *v1.Route,
	expectedRoute *v1.Route,
	match *gwv1.HTTPRouteMatch,
	logActual bool,
	filters ...gwv1.HTTPRouteFilter,
) {
	rtCtx := &plugins.RouteContext{
		Route: &gwv1.HTTPRoute{},
		Rule: &gwv1.HTTPRouteRule{
			Filters: filters,
		},
		Match: match,
	}
	err := plugin.ApplyRoutePlugin(
		context.Background(),
		rtCtx,
		outputRoute,
	)
	Expect(err).NotTo(HaveOccurred())

	if logActual {
		actualYaml, err := testutils.MarshalYaml(outputRoute)
		Expect(err).NotTo(HaveOccurred())
		log.Print("actualYaml: \n---\n", string(actualYaml), "\n---\n")
		expectedYaml, err := testutils.MarshalYaml(expectedRoute)
		Expect(err).NotTo(HaveOccurred())
		log.Print("expectedYaml: \n---\n", string(expectedYaml), "\n---\n")
	}
	Expect(proto.Equal(outputRoute, expectedRoute)).To(BeTrue())
}
