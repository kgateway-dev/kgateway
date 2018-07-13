package google

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/ginkgo/extensions/table"
	. "github.com/onsi/gomega"

	"github.com/gogo/protobuf/types"
	"github.com/solo-io/gloo/pkg/api/types/v1"
)

var _ = Describe("Spec", func() {
	Describe("Decode function", func() {
		DescribeTable("with invalid spec",

			func(f v1.FunctionSpec) {
				_, err := DecodeFunctionSpec(f)
				Expect(err).To(HaveOccurred())
			},

			Entry("empty", &types.Struct{
				Fields: map[string]*types.Value{},
			}),
			Entry("missing path", funcSpec("apple")),
			Entry("valid url with no path", funcSpec("http://apple.com")),
			Entry("url with port but no path", funcSpec("http://solo.io:8433")),
			Entry("invalid url", funcSpec("nowhere]:/apple")))

		DescribeTable("with valid spec",
			func(url, host, path string) {
				f, err := DecodeFunctionSpec(funcSpec(url))
				Expect(err).NotTo(HaveOccurred())
				Expect(f.host).To(Equal(host))
				Expect(f.path).To(Equal(path))
			},

			Entry("url with path", "http://test.com/apple", "test.com", "/apple"),
			Entry("/ path", "http://solo.io/", "solo.io", "/"))
	})

	Describe("Decode upstream", func() {
		DescribeTable("with invalid spec",
			func(u v1.UpstreamSpec) {
				_, err := DecodeUpstreamSpec(u)
				Expect(err).To(HaveOccurred())
			},

			Entry("empty", &types.Struct{
				Fields: map[string]*types.Value{},
			}),
			Entry("invalid region", upstreamSpec("solo", "gloo")),
			Entry("missing region", upstreamSpec("", "x23")),
			Entry("missing project", upstreamSpec("us-east1", "")))

		Context("with valid spec", func() {
			var (
				u   *UpstreamSpec
				err error
			)
			BeforeEach(func() {
				u, err = DecodeUpstreamSpec(upstreamSpec("us-east1", "project-231x"))
			})

			It("should not error", func() {
				Expect(err).NotTo(HaveOccurred())
			})

			It("should have correct parameters", func() {
				Expect(u.Region).To(Equal("us-east1"))
				Expect(u.ProjectId).To(Equal("project-231x"))
			})

			It("should return a host name with region and project", func() {
				Expect(u.GetGFuncHostname()).To(ContainSubstring("us-east1"))
				Expect(u.GetGFuncHostname()).To(ContainSubstring("project-231x"))

			})
		})
	})

})

func funcSpec(u string) v1.FunctionSpec {
	return &types.Struct{
		Fields: map[string]*types.Value{
			"Url": &types.Value{
				Kind: &types.Value_StringValue{StringValue: u},
			},
		},
	}
}

func upstreamSpec(region, projectID string) v1.UpstreamSpec {
	return &types.Struct{
		Fields: map[string]*types.Value{
			"region": &types.Value{
				Kind: &types.Value_StringValue{StringValue: region},
			},
			"project_id": &types.Value{
				Kind: &types.Value_StringValue{StringValue: projectID},
			},
		},
	}
}
