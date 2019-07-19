package ec2

import (
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ec2"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/ginkgo/extensions/table"
	. "github.com/onsi/gomega"
	glooec2 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/plugins/aws/ec2"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
)

var _ = Describe("Plugin", func() {
	Context("tag utils", func() {

		DescribeTable("filter from single tag key",
			func(input string) {
				output := tagFiltersKey(input)
				Expect(*output.Name).To(Equal("tag-key"))
				Expect(*output.Values[0]).To(Equal(input))
			},
			Entry("ex1", "some-key"),
			Entry("ex2", "another-key"),
		)

		DescribeTable("filter from tag key and value",
			func(key, value string) {
				output := tagFiltersKeyValue(key, value)
				Expect(*output.Name).To(Equal("tag:" + key))
				Expect(*output.Values[0]).To(Equal(value))
			},
			Entry("ex1", "some-key", "some-value"),
			Entry("ex2", "another-key", "another-value"),
		)
	})

	Context("convert filters from specs", func() {
		key1 := "abc"
		value1 := "123"
		secret := core.ResourceRef{"secret", "ns"}
		region := "us-east-1"
		DescribeTable("filter conversion",
			func(input *glooec2.UpstreamSpec, expected []*ec2.Filter) {
				output := convertFiltersFromSpec(input)
				for i, out := range output {
					Expect(out).To(Equal(expected[i]))
				}

			},
			Entry("ex1", &glooec2.UpstreamSpec{
				Region:    region,
				SecretRef: secret,
				Filters: []*glooec2.TagFilter{{
					Spec: &glooec2.TagFilter_Key{key1},
				}},
			},
				[]*ec2.Filter{{
					Name:   aws.String("tag-key"),
					Values: []*string{aws.String(key1)},
				}},
			),
			Entry("ex2", &glooec2.UpstreamSpec{
				Region:    region,
				SecretRef: secret,
				Filters: []*glooec2.TagFilter{{
					Spec: &glooec2.TagFilter_KvPair_{
						KvPair: &glooec2.TagFilter_KvPair{Key: key1, Value: value1}},
				}},
			},
				[]*ec2.Filter{{
					Name:   aws.String("tag:" + key1),
					Values: []*string{aws.String(value1)},
				}},
			),
		)
	})
})
