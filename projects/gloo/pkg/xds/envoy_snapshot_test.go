package xds_test

import (
	"reflect"

	"github.com/golang/protobuf/ptypes/any"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/projects/gloo/pkg/xds"
	"github.com/solo-io/solo-kit/pkg/api/v1/control-plane/cache"
)

var _ = Describe("EnvoySnapshot", func() {

	It("clones correctly", func() {

		toBeCloned := xds.NewSnapshot("1234",
			[]cache.Resource{xds.NewEnvoyResource(&any.Any{Value: []byte("endpoint")})},
			[]cache.Resource{xds.NewEnvoyResource(&any.Any{Value: []byte("cluster")})},
			[]cache.Resource{xds.NewEnvoyResource(&any.Any{Value: []byte("route")})},
			[]cache.Resource{xds.NewEnvoyResource(&any.Any{Value: []byte("listener")})},
		)

		// Create an identical struct which is guaranteed not to have been touched to compare against
		untouched := xds.NewSnapshot("1234",
			[]cache.Resource{xds.NewEnvoyResource(&any.Any{Value: []byte("endpoint")})},
			[]cache.Resource{xds.NewEnvoyResource(&any.Any{Value: []byte("cluster")})},
			[]cache.Resource{xds.NewEnvoyResource(&any.Any{Value: []byte("route")})},
			[]cache.Resource{xds.NewEnvoyResource(&any.Any{Value: []byte("listener")})},
		)

		clone := toBeCloned.Clone()

		// Verify that original snapshot and clone are identical
		Expect(reflect.DeepEqual(toBeCloned, clone)).To(BeTrue())
		Expect(reflect.DeepEqual(untouched, clone)).To(BeTrue())

		// Mutate the clone
		clone.GetResources(xds.EndpointTypev3).Items[""].(*xds.EnvoyResource).ResourceProto().(*any.Any).Value = []byte("new_endpoint")

		// Verify that original snapshot was not mutated
		Expect(reflect.DeepEqual(toBeCloned, clone)).To(BeFalse())
		Expect(reflect.DeepEqual(toBeCloned, untouched)).To(BeTrue())
	})
})
