package awscache

import (
	"context"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ec2"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/ginkgo/extensions/table"
	. "github.com/onsi/gomega"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/plugins/aws/glooec2"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
)

var _ = Describe("Batcher tests", func() {
	It("synopsis, basic check", func() {
		secretMeta1 := core.Metadata{Name: "s1", Namespace: "default"}
		secretRef1 := secretMeta1.Ref()
		secret1 := &v1.Secret{
			Kind: &v1.Secret_Aws{
				Aws: &v1.AwsSecret{
					AccessKey: "very-access",
					SecretKey: "very-secret",
				},
			},
			Metadata: secretMeta1,
		}
		secrets := v1.SecretList{secret1}
		cb := newCache(context.TODO(), secrets)
		region1 := "us-east-1"
		upRef1 := core.ResourceRef{"up1", "default"}
		upSpec1 := &glooec2.UpstreamSpec{
			Region:    region1,
			SecretRef: secretRef1,
			Filters: []*glooec2.TagFilter{{
				Spec: &glooec2.TagFilter_Key{
					Key: "k1",
				},
			}},
			PublicIp: false,
			Port:     8080,
		}
		up1 := &glooec2.UpstreamSpecRef{
			Spec: upSpec1,
			Ref:  upRef1,
		}
		err := cb.addUpstream(up1)
		Expect(err).NotTo(HaveOccurred())

		credSpec1 := credentialSpec{
			secretRef: secretRef1,
			region:    region1,
		}
		instances := []*ec2.Instance{{
			Tags: []*ec2.Tag{{
				Key:   aws.String("k1"),
				Value: aws.String("any old value"),
			}},
		}}
		Expect(cb.addInstances(credSpec1, instances)).NotTo(HaveOccurred())
		filteredInstances1, err := cb.FilterEndpointsForUpstream(up1)
		Expect(err).NotTo(HaveOccurred())
		Expect(filteredInstances1).To(Equal(instances))

	})

	// Represent 3 credential specification cases:
	// A: secret has full access to credentialMap in it region
	// B: secret has limited access to credentialMap in it region
	// C: same as A, different region
	var (
		region1             = "us-east-1"
		region2             = "us-east-2"
		instance1           = generateTestInstance("1") // region1
		instance2           = generateTestInstance("2") // region1
		instance3           = generateTestInstance("3") // region1
		instance4           = generateTestInstance("4") // region2
		instance5           = generateTestInstance("5") // region2
		secret1, secretRef1 = generateTestSecrets("1")
		secret2, secretRef2 = generateTestSecrets("2")
		credSpecA           = generateCredSpec(region1, secretRef1)
		credSpecB           = generateCredSpec(region1, secretRef2)
		credSpecC           = generateCredSpec(region2, secretRef2)
		credInstancesA      = []*ec2.Instance{instance1, instance2, instance3}
		credInstancesB      = []*ec2.Instance{instance1, instance2}
		credInstancesC      = []*ec2.Instance{instance4, instance5}
	)
	// In this table test, we use a fixed set of instances and secrets
	// Each table entry describes what filters should be applied to the upstream and what instances should be returned
	DescribeTable("batcher should assemble and disassemble batched results", func(input filterTestInput) {
		secrets := v1.SecretList{secret1, secret2}
		cb := newCache(context.TODO(), secrets)

		// build the dummy upstreams
		upA := generateUpstreamWithCredentials("A", credSpecA, filterTestInput{})
		upB := generateUpstreamWithCredentials("B", credSpecB, filterTestInput{})
		upC := generateUpstreamWithCredentials("C", credSpecC, filterTestInput{})
		// build the upstream that we care about
		upTest := generateUpstreamWithCredentials("Test", input.credentialSpec, input)
		// prime the map with the upstreams
		Expect(cb.addUpstream(upA)).NotTo(HaveOccurred())
		Expect(cb.addUpstream(upB)).NotTo(HaveOccurred())
		Expect(cb.addUpstream(upC)).NotTo(HaveOccurred())
		Expect(cb.addUpstream(upTest)).NotTo(HaveOccurred())

		// "query" the api for each upstream
		Expect(cb.addInstances(credSpecA, credInstancesA)).NotTo(HaveOccurred())
		Expect(cb.addInstances(credSpecB, credInstancesB)).NotTo(HaveOccurred())
		Expect(cb.addInstances(credSpecC, credInstancesC)).NotTo(HaveOccurred())

		// core test: apply the filter, assert expectations
		filteredInstances, err := cb.FilterEndpointsForUpstream(upTest)
		Expect(err).NotTo(HaveOccurred())
		//Expect(len(filteredInstances)).To(Equal(len(input.expected)))
		var filteredIds []string
		for _, instance := range filteredInstances {
			filteredIds = append(filteredIds, aws.StringValue(instance.InstanceId))
		}
		var expectedIds []string
		for _, instance := range input.expected {
			expectedIds = append(expectedIds, aws.StringValue(instance.InstanceId))
		}
		Expect(filteredIds).To(ConsistOf(expectedIds))
		Expect(filteredInstances).To(ConsistOf(input.expected))

	},
		Entry("get every instance in region 1", filterTestInput{
			credentialSpec:  credSpecA,
			keyFilters:      nil,
			keyValueFilters: nil,
			expected:        []*ec2.Instance{instance1, instance2, instance3},
		}),
		Entry("get instance 1 in region 1", filterTestInput{
			credentialSpec:  credSpecA,
			keyFilters:      []string{"k1a"},
			keyValueFilters: nil,
			expected:        []*ec2.Instance{instance1},
		}),
		Entry("not match with unused tag key", filterTestInput{
			credentialSpec:  credSpecA,
			keyFilters:      []string{tagKeyThatIsNotUsed},
			keyValueFilters: nil,
			expected:        nil,
		}),
		Entry("get multiple instances having common tags", filterTestInput{
			credentialSpec:  credSpecA,
			keyFilters:      []string{commonKeyKey},
			keyValueFilters: nil,
			expected:        []*ec2.Instance{instance1, instance2, instance3},
		}),
		Entry("get multiple instances having common tags", filterTestInput{
			credentialSpec:  credSpecA,
			keyFilters:      []string{commonTagKeyKey},
			keyValueFilters: nil,
			expected:        []*ec2.Instance{instance1, instance2, instance3},
		}),
		Entry("match with key and value", filterTestInput{
			credentialSpec:  credSpecA,
			keyFilters:      nil,
			keyValueFilters: []string{"k1a:v1a"},
			expected:        []*ec2.Instance{instance1},
		}),
		Entry("match with multiple keys and values", filterTestInput{
			credentialSpec:  credSpecA,
			keyFilters:      nil,
			keyValueFilters: []string{"k1a:v1a", "k1b:v1b"},
			expected:        []*ec2.Instance{instance1},
		}),
		Entry("not match any when filters are too restrictive", filterTestInput{
			credentialSpec:  credSpecA,
			keyFilters:      nil,
			keyValueFilters: []string{"k1a:v1a", "akey:not_in_use"},
			expected:        nil,
		}),
		// casing
		Entry("key case does not matter for key filters", filterTestInput{
			credentialSpec:  credSpecA,
			keyFilters:      []string{"K1A", "k1B"},
			keyValueFilters: nil,
			expected:        []*ec2.Instance{instance1},
		}),
		Entry("key case does not matter for key value filter keys", filterTestInput{
			credentialSpec:  credSpecA,
			keyFilters:      nil,
			keyValueFilters: []string{"K1A:v1a", "K1b:v1b"},
			expected:        []*ec2.Instance{instance1},
		}),
		Entry("key case does matter for key value filter values", filterTestInput{
			credentialSpec:  credSpecA,
			keyFilters:      nil,
			keyValueFilters: []string{"k1a:V1A"},
			expected:        nil,
		}),
		// misc. test consistency validation
		Entry("test works with other credentials, no filters", filterTestInput{
			credentialSpec:  credSpecB,
			keyFilters:      nil,
			keyValueFilters: nil,
			expected:        []*ec2.Instance{instance1, instance2},
		}),
		Entry("test works with other credentials, k filters", filterTestInput{
			credentialSpec:  credSpecB,
			keyFilters:      []string{"k1a"},
			keyValueFilters: nil,
			expected:        []*ec2.Instance{instance1},
		}),
		Entry("test works with other credentials, kv filters", filterTestInput{
			credentialSpec:  credSpecB,
			keyFilters:      nil,
			keyValueFilters: []string{"k2a:v2a"},
			expected:        []*ec2.Instance{instance2},
		}),
		Entry("test works with other region, no filters", filterTestInput{
			credentialSpec:  credSpecC,
			keyFilters:      nil,
			keyValueFilters: nil,
			expected:        []*ec2.Instance{instance4, instance5},
		}),
		Entry("test works with other region, k filters", filterTestInput{
			credentialSpec:  credSpecC,
			keyFilters:      []string{"k4b"},
			keyValueFilters: nil,
			expected:        []*ec2.Instance{instance4},
		}),
		Entry("test works with other region, kv filters", filterTestInput{
			credentialSpec:  credSpecC,
			keyFilters:      nil,
			keyValueFilters: []string{"k5b:v5b"},
			expected:        []*ec2.Instance{instance5},
		}),
		Entry("credA returns no matches for (irrelevant) filters from credC", filterTestInput{
			credentialSpec:  credSpecA,
			keyFilters:      nil,
			keyValueFilters: []string{"k5b:v5b"},
			expected:        nil,
		}))

})

type filterTestInput struct {
	// use these credentials when accessing
	credentialSpec credentialSpec
	// format: <key>
	keyFilters []string
	// format: <key>:<value>
	keyValueFilters []string
	// these instances should be returned
	expected []*ec2.Instance
}

const (
	commonKeyKey        = "common_key_only"
	commonTagKeyKey     = "common_key_and_value"
	commonTagKeyValue   = "common_value_d"
	tagKeyThatIsNotUsed = "no_instances_have_this_tag"
)

// outputs basic templated instances for testing various filters
func generateTestInstance(seed string) *ec2.Instance {
	return &ec2.Instance{
		InstanceId: aws.String("i" + seed),
		Tags: []*ec2.Tag{{
			Key:   aws.String("k" + seed + "a"),
			Value: aws.String("v" + seed + "a"),
		}, {
			Key:   aws.String("k" + seed + "b"),
			Value: aws.String("v" + seed + "b"),
		}, {
			Key:   aws.String(commonKeyKey),
			Value: aws.String("v" + seed + "c"),
		}, {
			Key:   aws.String(commonTagKeyKey),
			Value: aws.String(commonTagKeyValue),
		}, {
			Key:   aws.String("unrelated_key_should_not_match"),
			Value: aws.String("unrelated_value_should_not_match"),
		}},
	}

}

func generateTestSecrets(seed string) (*v1.Secret, core.ResourceRef) {
	secretMeta := core.Metadata{Name: "s" + seed, Namespace: "default"}
	secretRef := secretMeta.Ref()
	secret := &v1.Secret{
		Kind: &v1.Secret_Aws{
			Aws: &v1.AwsSecret{
				AccessKey: "abc",
				SecretKey: "123",
			},
		},
		Metadata: secretMeta,
	}
	return secret, secretRef
}

func generateCredSpec(region string, secretRef core.ResourceRef) credentialSpec {
	return credentialSpec{
		secretRef: secretRef,
		region:    region,
	}

}

// creates an upstream with the filters and credentials defined by the input
func generateUpstreamWithCredentials(name string, credSpec credentialSpec, input filterTestInput) *glooec2.UpstreamSpecRef {
	upstreamRef := core.ResourceRef{
		Name:      name,
		Namespace: "default",
	}
	upstreamSpec := &glooec2.UpstreamSpec{
		Region:    credSpec.region,
		SecretRef: credSpec.secretRef,
	}
	for _, key := range input.keyFilters {
		f := &glooec2.TagFilter{
			Spec: &glooec2.TagFilter_Key{
				Key: key,
			},
		}
		upstreamSpec.Filters = append(upstreamSpec.Filters, f)
	}
	for _, kv := range input.keyValueFilters {
		parts := strings.Split(kv, ":")
		Expect(len(parts)).To(Equal(2))
		key := parts[0]
		val := parts[1]
		f := &glooec2.TagFilter{
			Spec: &glooec2.TagFilter_KvPair_{
				KvPair: &glooec2.TagFilter_KvPair{
					Key:   key,
					Value: val,
				},
			},
		}
		upstreamSpec.Filters = append(upstreamSpec.Filters, f)
	}
	return &glooec2.UpstreamSpecRef{
		Spec: upstreamSpec,
		Ref:  upstreamRef,
	}
}
