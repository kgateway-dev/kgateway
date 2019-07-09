package ec2

import (
	"context"
	"fmt"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ec2"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/ginkgo/extensions/table"
	. "github.com/onsi/gomega"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/plugins/aws/glooec2"
	"github.com/solo-io/gloo/projects/gloo/pkg/bootstrap"
	"github.com/solo-io/go-utils/kubeutils"
	corecache "github.com/solo-io/solo-kit/pkg/api/v1/clients/kube/cache"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/memory"
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
				Filters: []*glooec2.Filter{{
					Spec: &glooec2.Filter_Key{key1},
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
				Filters: []*glooec2.Filter{{
					Spec: &glooec2.Filter_KvPair_{
						KvPair: &glooec2.Filter_KvPair{Key: key1, Value: value1}},
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

var _ = Describe("polling", func() {

	var (
		epw            *edsWatcher
		ctx            context.Context
		writeNamespace string
		upstreams      v1.UpstreamList
		secretClient   *v1.SecretClient
		refreshRate    time.Duration
		responses      mockListerResponses
	)

	BeforeEach(func() {
		ctx = context.Background()
		writeNamespace = "default"
		upstreams = getUpstreams()
		secretClient = getSecretClient(ctx)
		refreshRate = time.Second
		responses = getMockListerResponses()
		epw = testEndpointsWatcher(ctx, writeNamespace, upstreams, secretClient, refreshRate, responses)
	})

	It("should poll", func() {
		Eventually(func() error {
			endpointChan, eChan, err := epw.poll()
			if err != nil {
				return err
			}
			select {
			case ec := <-eChan:
				return ec
			case endPoint := <-endpointChan:
				ref := testUpstream1.Metadata.Ref()
				return assertEndpointList(endPoint, v1.EndpointList{{
					Upstreams: []*core.ResourceRef{&ref},
					Address:   testPrivateIp1,
					Port:      testPort1,
					Metadata: core.Metadata{
						Name:      "ec2-name-u1-namespace-default--111-111-111-111",
						Namespace: "default",
					},
				}})
			}
		}).ShouldNot(HaveOccurred())
	})
})

func assertEndpointList(input, expected v1.EndpointList) error {
	if len(input) == 0 {
		return fmt.Errorf("no input provided")
	}
	for i := range input {
		a := input[i]
		b := expected[i]
		if !proto.Equal(a, b) {
			return fmt.Errorf("input does not match expectation:\n%v\n%v\n", a, b)
		}
	}
	return nil
}

func testEndpointsWatcher(
	watchCtx context.Context,
	writeNamespace string,
	upstreams v1.UpstreamList,
	secretClient *v1.SecretClient,
	parentRefreshRate time.Duration,
	responses mockListerResponses,
) *edsWatcher {
	upstreamSpecs := make(map[core.ResourceRef]*glooec2.UpstreamSpec)
	for _, us := range upstreams {
		ec2Upstream, ok := us.UpstreamSpec.UpstreamType.(*v1.UpstreamSpec_AwsEc2)
		if !ok {
			continue
		}
		upstreamSpecs[us.Metadata.Ref()] = ec2Upstream.AwsEc2
	}
	return &edsWatcher{
		upstreams:         upstreamSpecs,
		watchContext:      watchCtx,
		secretClient:      secretClient,
		refreshRate:       getRefreshRate(parentRefreshRate),
		writeNamespace:    writeNamespace,
		ec2InstanceLister: newMockEc2InstanceLister(responses),
	}
}

type mockListerResponses map[string][]*ec2.Instance
type mockEc2InstanceLister struct {
	responses mockListerResponses
}

func newMockEc2InstanceLister(responses mockListerResponses) *mockEc2InstanceLister {
	// add any test inputs to this
	return &mockEc2InstanceLister{
		responses: responses,
	}
}

func (m *mockEc2InstanceLister) ListForCredentials(ctx context.Context, ec2Upstream *glooec2.UpstreamSpec, secrets v1.SecretList) ([]*ec2.Instance, error) {
	v, ok := m.responses[ec2Upstream.SecretRef.Key()]
	if !ok {
		return nil, fmt.Errorf("invalid input, no test responses available")
	}
	return v, nil
}

func getSecretClient(ctx context.Context) *v1.SecretClient {
	config, err := kubeutils.GetConfig("", "")
	Expect(err).NotTo(HaveOccurred())
	mc := memory.NewInMemoryResourceCache()
	var kubeCoreCache corecache.KubeCoreCache
	settings := &v1.Settings{}
	secretFactory, err := bootstrap.SecretFactoryForSettings(ctx, settings, mc, &config, nil, &kubeCoreCache, v1.SecretCrd.Plural)
	Expect(err).NotTo(HaveOccurred())
	secretClient, err := v1.NewSecretClient(secretFactory)
	Expect(err).NotTo(HaveOccurred())
	err = primeSecretClient(secretClient)
	Expect(err).NotTo(HaveOccurred())
	return &secretClient

}

func getUpstreams() v1.UpstreamList {
	return v1.UpstreamList{&testUpstream1}

}

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
					Filters: []*glooec2.Filter{{
						Spec: &glooec2.Filter_Key{
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
	testCredential1 = core.ResourceRef{
		Name:      "secret",
		Namespace: "namespace",
	}
)

func getMockListerResponses() mockListerResponses {
	resp := make(mockListerResponses)
	resp[testCredential1.Key()] = []*ec2.Instance{{
		// showing nil filed names for reference purposes
		AmiLaunchIndex:                          nil,
		Architecture:                            nil,
		BlockDeviceMappings:                     nil,
		CapacityReservationId:                   nil,
		CapacityReservationSpecification:        nil,
		ClientToken:                             nil,
		CpuOptions:                              nil,
		EbsOptimized:                            nil,
		ElasticGpuAssociations:                  nil,
		ElasticInferenceAcceleratorAssociations: nil,
		EnaSupport:                              nil,
		HibernationOptions:                      nil,
		Hypervisor:                              nil,
		IamInstanceProfile:                      nil,
		ImageId:                                 nil,
		InstanceId:                              nil,
		InstanceLifecycle:                       nil,
		InstanceType:                            nil,
		KernelId:                                nil,
		KeyName:                                 nil,
		LaunchTime:                              nil,
		Licenses:                                nil,
		Monitoring:                              nil,
		NetworkInterfaces:                       nil,
		Placement:                               nil,
		Platform:                                nil,
		PrivateDnsName:                          nil,
		PrivateIpAddress:                        aws.String(testPrivateIp1),
		ProductCodes:                            nil,
		PublicDnsName:                           nil,
		PublicIpAddress:                         aws.String(testPublicIp1),
		RamdiskId:                               nil,
		RootDeviceName:                          nil,
		RootDeviceType:                          nil,
		SecurityGroups:                          nil,
		SourceDestCheck:                         nil,
		SpotInstanceRequestId:                   nil,
		SriovNetSupport:                         nil,
		State:                                   nil,
		StateReason:                             nil,
		StateTransitionReason:                   nil,
		SubnetId:                                nil,
		Tags: []*ec2.Tag{{
			Key:   aws.String("k1"),
			Value: aws.String("any old value"),
		}},
		VirtualizationType: nil,
		VpcId:              aws.String("id1"),
	}}
	return resp
}

func primeSecretClient(secretClient v1.SecretClient) error {
	secret := &v1.Secret{
		Kind: &v1.Secret_Aws{
			Aws: &v1.AwsSecret{
				AccessKey: "access",
				SecretKey: "secret",
			},
		},
		Metadata: core.Metadata{
			Name:      testCredential1.Name,
			Namespace: testCredential1.Namespace,
		},
	}
	_, err := secretClient.Write(secret, clients.WriteOpts{})
	return err
}
