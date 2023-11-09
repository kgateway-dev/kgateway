package filtertests

import (
	"context"
	"log"

	"github.com/solo-io/gloo/projects/gateway2/translator/httproute/filterplugins"

	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/projects/gateway2/translator/testutils"

	routev3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

func AssertExpectedRoute(
	plugin filterplugins.FilterPlugin,
	filter gwv1.HTTPRouteFilter, expectedRoute *routev3.Route, logActual bool) {
	outputRoute := &routev3.Route{}
	ctx := &filterplugins.RouteContext{
		Ctx:      context.TODO(),
		Route:    &gwv1.HTTPRoute{},
		Queries:  nil,
		Rule:     nil,
		Reporter: nil,
	}
	err := plugin.ApplyFilter(
		ctx,
		filter,
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

	Expect(outputRoute).To(Equal(expectedRoute))
}
