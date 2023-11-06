//go:build test
// +build test

package filtertests

import (
	"context"
	"log"

	"github.com/solo-io/gloo/projects/gateway2/translator/httproute/filterplugins"

	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/projects/gateway2/translator/testutils"

	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

func AssertExpectedRoute(
	plugin filterplugins.FilterPlugin,
	filter gwv1.HTTPRouteFilter, expectedRoute *v1.Route, logActual bool) {
	ctx := context.TODO()
	outputRoute := &v1.Route{
		Options: &v1.RouteOptions{},
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
