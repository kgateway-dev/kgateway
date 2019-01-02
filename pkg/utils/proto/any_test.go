package proto_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	. "github.com/solo-io/gloo/pkg/utils/proto"

	"github.com/gogo/protobuf/types"
)

var _ = Describe("Any", func() {

	It("should deserialized a proto message from map", func() {

		duration := types.Duration{
			Nanos:   1,
			Seconds: 2,
		}
		anyduration, err := types.MarshalAny(&duration)
		Expect(err).NotTo(HaveOccurred())

		protos := map[string]*types.Any{
			"duration": anyduration,
		}

		m, err := GetMessage(protos, "duration")
		Expect(err).NotTo(HaveOccurred())

		Expect(m).NotTo(BeNil())
		Expect(*m.(*types.Duration)).To(Equal(duration))
	})

	It("should error if no name found", func() {

		protos := map[string]*types.Any{}
		_, err := GetMessage(protos, "duration")
		Expect(err).To(HaveOccurred())

	})
})
