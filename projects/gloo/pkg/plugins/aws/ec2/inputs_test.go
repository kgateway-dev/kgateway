package ec2

import (
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/plugins/aws/glooec2"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
)

var (
	testPort1      uint32 = 8080
	testPrivateIp1        = "111-111-111-111"
	testPublicIp1         = "222.222.222.222"
	testUpstream1         = v1.Upstream{
		UpstreamSpec: &v1.UpstreamSpec{
			UpstreamType: &v1.UpstreamSpec_AwsEc2{
				AwsEc2: &glooec2.UpstreamSpec{
					Region:    "us-east-1",
					SecretRef: testCredential1,
					Filters: []*glooec2.TagFilter{{
						Spec: &glooec2.TagFilter_Key{
							Key: "k1",
						},
					}},
					PublicIp: false,
					Port:     testPort1,
				},
			}},
		Metadata: core.Metadata{
			Name:      "u1",
			Namespace: "default",
		},
	}
	testUpstream2 = v1.Upstream{
		UpstreamSpec: &v1.UpstreamSpec{
			UpstreamType: &v1.UpstreamSpec_AwsEc2{
				AwsEc2: &glooec2.UpstreamSpec{
					Region:    "us-east-1",
					SecretRef: testCredential2,
					Filters: []*glooec2.TagFilter{{
						Spec: &glooec2.TagFilter_KvPair_{
							KvPair: &glooec2.TagFilter_KvPair{
								Key:   "k2",
								Value: "v2",
							},
						},
					}},
					PublicIp: true,
					Port:     testPort1,
				},
			}},
		Metadata: core.Metadata{
			Name:      "u2",
			Namespace: "default",
		},
	}
	testCredential1 = core.ResourceRef{
		Name:      "secret1",
		Namespace: "namespace",
	}
	testCredential2 = core.ResourceRef{
		Name:      "secret2",
		Namespace: "namespace",
	}
)
