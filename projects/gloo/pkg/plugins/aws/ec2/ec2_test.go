package ec2

import (
	"context"

	. "github.com/solo-io/solo-kit/test/matchers"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ec2"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"

	glooec2 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/aws/ec2"
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
		secret := &core.ResourceRef{Name: "secret", Namespace: "ns"}
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

	Context("convert to endpoint", func() {
		pubIp := "1.2.3.4"
		privateIp := "5.5.5.5"
		writeNamespace := "default"
		DescribeTable("convert to endpoints", func(input *v1.Upstream, instance *ec2.Instance, expected *v1.Endpoint) {
			out := upstreamInstanceToEndpoint(context.TODO(), writeNamespace, input, instance)
			Expect(out).To(MatchProto(expected))
		},
			Entry("should use proper default port and ip when not specified", &v1.Upstream{
				UpstreamType: &v1.Upstream_AwsEc2{
					AwsEc2: &glooec2.UpstreamSpec{
						Region:    "us-east-1",
						SecretRef: nil,
						RoleArn:   "",
						Filters:   nil,
						PublicIp:  false,
						Port:      0,
					},
				},
				Metadata: &core.Metadata{
					Name:      "ex1",
					Namespace: "default",
				},
			},
				&ec2.Instance{
					InstanceId:       aws.String("id1"),
					PublicIpAddress:  aws.String(pubIp),
					PrivateIpAddress: aws.String(privateIp),
				},
				&v1.Endpoint{
					Upstreams: []*core.ResourceRef{{Name: "ex1", Namespace: "default"}},
					Address:   privateIp,
					Port:      80,
					Metadata: &core.Metadata{
						Name:        "ec2-name-ex1-namespace-default-5-5-5-5",
						Namespace:   writeNamespace,
						Annotations: map[string]string{InstanceIdAnnotationKey: "id1"},
					},
				}),
			Entry("should use proper port and ip when specified", &v1.Upstream{
				UpstreamType: &v1.Upstream_AwsEc2{
					AwsEc2: &glooec2.UpstreamSpec{
						Region:    "us-east-1",
						SecretRef: nil,
						RoleArn:   "",
						Filters:   nil,
						PublicIp:  true,
						Port:      77,
					},
				},
				Metadata: &core.Metadata{
					Name:      "ex1",
					Namespace: "default",
				},
			},
				&ec2.Instance{
					InstanceId:       aws.String("id1"),
					PublicIpAddress:  aws.String(pubIp),
					PrivateIpAddress: aws.String(privateIp),
				},
				&v1.Endpoint{
					Upstreams: []*core.ResourceRef{{Name: "ex1", Namespace: "default"}},
					Address:   pubIp,
					Port:      77,
					Metadata: &core.Metadata{
						Name:        "ec2-name-ex1-namespace-default-1-2-3-4",
						Namespace:   writeNamespace,
						Annotations: map[string]string{InstanceIdAnnotationKey: "id1"},
					},
				}),
			Entry("should accept instances with private ip only", &v1.Upstream{
				UpstreamType: &v1.Upstream_AwsEc2{
					AwsEc2: &glooec2.UpstreamSpec{
						Region:    "us-east-1",
						SecretRef: nil,
						RoleArn:   "",
						Filters:   nil,
						PublicIp:  false,
						Port:      77,
					},
				},
				Metadata: &core.Metadata{
					Name:      "ex1",
					Namespace: "default",
				},
			},
				&ec2.Instance{
					InstanceId:       aws.String("id1"),
					PrivateIpAddress: aws.String(privateIp),
				},
				&v1.Endpoint{
					Upstreams: []*core.ResourceRef{{Name: "ex1", Namespace: "default"}},
					Address:   privateIp,
					Port:      77,
					Metadata: &core.Metadata{
						Name:        "ec2-name-ex1-namespace-default-5-5-5-5",
						Namespace:   writeNamespace,
						Annotations: map[string]string{InstanceIdAnnotationKey: "id1"},
					},
				}),
			Entry("should return nil if no ips are available for the given config", &v1.Upstream{
				UpstreamType: &v1.Upstream_AwsEc2{
					AwsEc2: &glooec2.UpstreamSpec{
						Region:    "us-east-1",
						SecretRef: nil,
						RoleArn:   "",
						Filters:   nil,
						PublicIp:  false,
						Port:      77,
					},
				},
				Metadata: &core.Metadata{
					Name:      "ex1",
					Namespace: "default",
				},
			},
				&ec2.Instance{
					InstanceId: aws.String("id1"),
				},
				nil,
			))
	})
})
