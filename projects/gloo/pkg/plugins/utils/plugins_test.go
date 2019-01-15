package utils_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/envoyproxy/go-control-plane/pkg/util"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	. "github.com/solo-io/gloo/projects/gloo/pkg/plugins/utils"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"

	"github.com/gogo/protobuf/types"
)

var _ = Describe("Plugins", func() {

	It("should deserialized a proto message from map", func() {
		orginalMessage := &types.Api{Name: "test"}
		pluginstruct, err := util.MessageToStruct(orginalMessage)

		Expect(err).NotTo(HaveOccurred())

		protos := map[string]*types.Struct{
			"duration": pluginstruct,
		}

		outm := new(types.Api)
		err = UnmarshalStructFromMap(protos, "duration", outm)
		Expect(err).NotTo(HaveOccurred())

		Expect(outm).To(Equal(orginalMessage))
	})

	It("should error if no name found with expected error", func() {

		protos := map[string]*types.Struct{}
		var outm types.Api
		err := UnmarshalStructFromMap(protos, "duration", &outm)
		Expect(err).To(HaveOccurred())
		Expect(err).To(Equal(NotFoundError))
	})

	It("should error if proto is bad with other error", func() {

		other, err := util.MessageToStruct(&types.Api{Name: "test"})
		Expect(err).NotTo(HaveOccurred())

		protos := map[string]*types.Struct{
			"msg": other,
		}

		var outm core.Status
		err = UnmarshalStructFromMap(protos, "msg", &outm)
		Expect(err).To(HaveOccurred())
		Expect(err).NotTo(Equal(NotFoundError))
	})

	Describe("From plugins", func() {

		It("should return not found for nil plugins", func() {
			var outm types.Api
			err := UnmarshalExtension(nil, "test", &outm)
			Expect(err).To(HaveOccurred())
			Expect(err).To(Equal(NotFoundError))
		})

		It("should return not found for typed nil plugins", func() {
			var p *extensions
			var outm types.Api
			err := UnmarshalExtension(p, "test", &outm)
			Expect(err).To(HaveOccurred())
			Expect(err).To(Equal(NotFoundError))
		})

		It("should return not found for nil plugin map", func() {
			var p extensions
			p.extensions = &v1.Extensions{}
			var outm types.Api
			err := UnmarshalExtension(&p, "test", &outm)
			Expect(err).To(HaveOccurred())
			Expect(err).To(Equal(NotFoundError))
		})

	})

	Describe("From vhost plugins", func() {
		It("should work with vhost plugins", func() {

			orginalMessage := &types.Api{Name: "test"}
			pluginstruct, err := util.MessageToStruct(orginalMessage)
			Expect(err).NotTo(HaveOccurred())

			vhost := v1.VirtualHost{
				Name:    "test",
				Domains: []string{"domain"},
				VirtualHostPlugins: &v1.VirtualHostPlugins{
					Extensions: &v1.Extensions{
						Configs: map[string]*types.Struct{
							"test": pluginstruct,
						},
					},
				},
			}
			outm := new(types.Api)

			err = UnmarshalExtension(vhost.GetVirtualHostPlugins(), "test", outm)
			Expect(outm).To(Equal(orginalMessage))
		})
	})

})

type extensions struct {
	extensions *v1.Extensions
}

func (e *extensions) GetExtensions() *v1.Extensions { return e.extensions }
