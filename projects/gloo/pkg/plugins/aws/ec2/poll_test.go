package ec2

import (
	"context"
	"fmt"
	"time"

	"k8s.io/client-go/rest"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/golang/protobuf/proto"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"

	glooec2 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/aws/ec2"
	bootstrap "github.com/solo-io/gloo/projects/gloo/pkg/bootstrap/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	corecache "github.com/solo-io/solo-kit/pkg/api/v1/clients/kube/cache"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/memory"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
)

var _ = Describe("polling", func() {

	var (
		epw            *edsWatcher
		ctx            context.Context
		cancel         context.CancelFunc
		writeNamespace string
		secretClient   v1.SecretClient
		refreshRate    time.Duration
		responses      mockListerResponses
	)

	BeforeEach(func() {
		ctx, cancel = context.WithCancel(context.Background())
		writeNamespace = "default"
		secretClient = getSecretClient(ctx)
		refreshRate = time.Second
		responses = getMockListerResponses()
		err := primeSecretClient(secretClient, []*core.ResourceRef{testSecretRef1, testSecretRef2})
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() { cancel() })

	It("should poll, one key filter upstream", func() {
		upstreams := v1.UpstreamList{&testUpstream1}
		epw = testEndpointsWatcher(ctx, writeNamespace, upstreams, secretClient, refreshRate, responses)
		ref1 := testUpstream1.Metadata.Ref()
		matchPollResponse(epw, v1.EndpointList{{
			Upstreams: []*core.ResourceRef{ref1},
			Address:   testPrivateIp1,
			Port:      testPort1,
			Metadata: &core.Metadata{
				Name:        "ec2-name-u1-namespace-default-111-111-111-111",
				Namespace:   "default",
				Annotations: map[string]string{InstanceIdAnnotationKey: "instanceIdA"},
			},
		}})
	})
	It("should poll, one key-value filter upstream", func() {
		upstreams := v1.UpstreamList{&testUpstream2}
		epw = testEndpointsWatcher(ctx, writeNamespace, upstreams, secretClient, refreshRate, responses)
		ref2 := testUpstream2.Metadata.Ref()
		matchPollResponse(epw, v1.EndpointList{{
			Upstreams: []*core.ResourceRef{ref2},
			Address:   testPublicIp1,
			Port:      testPort1,
			Metadata: &core.Metadata{
				Name:        "ec2-name-u2-namespace-default-222-222-222-222",
				Namespace:   "default",
				Annotations: map[string]string{InstanceIdAnnotationKey: "instanceIdB"},
			},
		}})
	})
})

func matchPollResponse(epw *edsWatcher, expectedList v1.EndpointList) {
	EventuallyWithOffset(1, func() error {
		endpointChan, eChan, err := epw.poll()
		if err != nil {
			return err
		}
		select {
		case ec := <-eChan:
			return ec
		case endPoint := <-endpointChan:
			return assertEndpointList(endPoint, expectedList)
		}
	}).ShouldNot(HaveOccurred())
}

func assertEndpointList(input, expected v1.EndpointList) error {
	if len(input) == 0 {
		return fmt.Errorf("no input provided")
	}
	if len(input) != len(expected) {
		return fmt.Errorf("expected equal length lists, got len: %v expected len: %v", len(input), len(expected))
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
	secretClient v1.SecretClient,
	parentRefreshRate time.Duration,
	responses mockListerResponses,
) *edsWatcher {
	return &edsWatcher{
		upstreams:         upstreams,
		watchContext:      watchCtx,
		secretClient:      secretClient,
		refreshRate:       getRefreshRate(parentRefreshRate),
		writeNamespace:    writeNamespace,
		ec2InstanceLister: newMockEc2InstanceLister(responses),
	}
}

type mockListerResponses map[CredentialKey][]*ec2.Instance
type mockEc2InstanceLister struct {
	responses mockListerResponses
}

func newMockEc2InstanceLister(responses mockListerResponses) *mockEc2InstanceLister {
	// add any test inputs to this
	return &mockEc2InstanceLister{
		responses: responses,
	}
}

func (m *mockEc2InstanceLister) ListForCredentials(ctx context.Context, cred *CredentialSpec, secrets v1.SecretList) ([]*ec2.Instance, error) {
	v, ok := m.responses[cred.GetKey()]
	if !ok {
		return nil, fmt.Errorf("invalid input, no test responses available")
	}
	return v, nil
}

func getSecretClient(ctx context.Context) v1.SecretClient {
	config := &rest.Config{}
	mc := memory.NewInMemoryResourceCache()
	var kubeCoreCache corecache.KubeCoreCache
	settings := &v1.Settings{}
	secretFactory, err := bootstrap.SecretFactoryForSettingsWithRetry(ctx, bootstrap.SecretFactoryParams{
		Settings:           settings,
		SharedCache:        mc,
		Cfg:                &config,
		Clientset:          nil,
		KubeCoreCache:      &kubeCoreCache,
		VaultClientInitMap: nil,
		PluralName:         v1.SecretCrd.Plural,
	})
	Expect(err).NotTo(HaveOccurred())
	secretClient, err := v1.NewSecretClient(ctx, secretFactory)
	Expect(err).NotTo(HaveOccurred())
	return secretClient

}

func getMockListerResponses() mockListerResponses {
	resp := make(mockListerResponses)
	region1 := "us-east-1"
	ec2Upstream1 := &glooec2.UpstreamSpec{
		Region:    region1,
		SecretRef: testSecretRef1,
		RoleArn:   "",
	}
	cred1 := NewCredentialSpecFromEc2UpstreamSpec(ec2Upstream1)
	resp[cred1.GetKey()] = []*ec2.Instance{{
		InstanceId:       aws.String("instanceIdA"),
		PrivateIpAddress: aws.String(testPrivateIp1),
		PublicIpAddress:  aws.String(testPublicIp1),
		Tags: []*ec2.Tag{{
			Key:   aws.String("k1"),
			Value: aws.String("any old value"),
		}},
		VpcId: aws.String("id1"),
	}}
	ec2Upstream2 := &glooec2.UpstreamSpec{
		Region:    region1,
		SecretRef: testSecretRef2,
		RoleArn:   "",
	}
	cred2 := NewCredentialSpecFromEc2UpstreamSpec(ec2Upstream2)
	resp[cred2.GetKey()] = []*ec2.Instance{{
		InstanceId:       aws.String("instanceIdB"),
		PrivateIpAddress: aws.String(testPrivateIp1),
		PublicIpAddress:  aws.String(testPublicIp1),
		Tags: []*ec2.Tag{{
			Key:   aws.String("k2"),
			Value: aws.String("v2"),
		}},
		VpcId: aws.String("id2"),
	}}
	return resp
}

func primeSecretClient(secretClient v1.SecretClient, secretRefs []*core.ResourceRef) error {
	for _, ref := range secretRefs {
		secret := &v1.Secret{
			Kind: &v1.Secret_Aws{
				Aws: &v1.AwsSecret{
					AccessKey: "access",
					SecretKey: "secret",
				},
			},
			Metadata: &core.Metadata{
				Name:      ref.Name,
				Namespace: ref.Namespace,
			},
		}
		_, err := secretClient.Write(secret, clients.WriteOpts{})
		if err != nil {
			return err
		}
	}
	return nil
}

var (
	testPort1      uint32 = 8080
	testPrivateIp1        = "111-111-111-111"
	testPublicIp1         = "222.222.222.222"
	testUpstream1         = v1.Upstream{
		UpstreamType: &v1.Upstream_AwsEc2{
			AwsEc2: &glooec2.UpstreamSpec{
				Region:    "us-east-1",
				SecretRef: testSecretRef1,
				Filters: []*glooec2.TagFilter{{
					Spec: &glooec2.TagFilter_Key{
						Key: "k1",
					},
				}},
				PublicIp: false,
				Port:     testPort1,
			},
		},
		Metadata: &core.Metadata{
			Name:      "u1",
			Namespace: "default",
		},
	}
	testUpstream2 = v1.Upstream{
		UpstreamType: &v1.Upstream_AwsEc2{
			AwsEc2: &glooec2.UpstreamSpec{
				Region:    "us-east-1",
				SecretRef: testSecretRef2,
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
		},
		Metadata: &core.Metadata{
			Name:      "u2",
			Namespace: "default",
		},
	}
	testSecretRef1 = &core.ResourceRef{
		Name:      "secret1",
		Namespace: "namespace",
	}
	testSecretRef2 = &core.ResourceRef{
		Name:      "secret2",
		Namespace: "namespace",
	}
)
