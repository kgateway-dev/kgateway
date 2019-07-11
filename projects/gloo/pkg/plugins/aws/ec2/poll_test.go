package ec2

import (
	"context"
	"fmt"
	"time"

	"k8s.io/client-go/rest"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/gogo/protobuf/proto"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/plugins/aws/glooec2"
	"github.com/solo-io/gloo/projects/gloo/pkg/bootstrap"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	corecache "github.com/solo-io/solo-kit/pkg/api/v1/clients/kube/cache"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/memory"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
)

var _ = Describe("polling", func() {

	var (
		epw            *edsWatcher
		ctx            context.Context
		writeNamespace string
		secretClient   v1.SecretClient
		refreshRate    time.Duration
		responses      mockListerResponses
	)

	BeforeEach(func() {
		ctx = context.Background()
		writeNamespace = "default"
		secretClient = getSecretClient(ctx)
		refreshRate = time.Second
		responses = getMockListerResponses()
		err := primeSecretClient(secretClient, []core.ResourceRef{testCredential1, testCredential2})
		Expect(err).NotTo(HaveOccurred())
	})

	It("should poll, one key filter upstream", func() {
		upstreams := v1.UpstreamList{&testUpstream1}
		epw = testEndpointsWatcher(ctx, writeNamespace, upstreams, secretClient, refreshRate, responses)
		ref1 := testUpstream1.Metadata.Ref()
		matchPollResponse(epw, v1.EndpointList{{
			Upstreams: []*core.ResourceRef{&ref1},
			Address:   testPrivateIp1,
			Port:      testPort1,
			Metadata: core.Metadata{
				Name:      "ec2-name-u1-namespace-default--111-111-111-111",
				Namespace: "default",
			},
		}})
	})
	It("should poll, one key-value filter upstream", func() {
		upstreams := v1.UpstreamList{&testUpstream2}
		epw = testEndpointsWatcher(ctx, writeNamespace, upstreams, secretClient, refreshRate, responses)
		ref2 := testUpstream2.Metadata.Ref()
		matchPollResponse(epw, v1.EndpointList{{
			Upstreams: []*core.ResourceRef{&ref2},
			Address:   testPublicIp1,
			Port:      testPort1,
			Metadata: core.Metadata{
				Name:      "ec2-name-u2-namespace-default--222-222-222-222",
				Namespace: "default",
			},
		}})
	})
})

func matchPollResponse(epw *edsWatcher, expectedList v1.EndpointList) {
	Eventually(func() error {
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
	upstreamSpecs := make(map[core.ResourceRef]*glooec2.UpstreamSpecRef)
	for _, us := range upstreams {
		ec2Upstream, ok := us.UpstreamSpec.UpstreamType.(*v1.UpstreamSpec_AwsEc2)
		if !ok {
			continue
		}
		ref := us.Metadata.Ref()
		upstreamSpecs[ref] = &glooec2.UpstreamSpecRef{
			Spec: ec2Upstream.AwsEc2,
			Ref:  ref,
		}
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

func (m *mockEc2InstanceLister) ListForCredentials(ctx context.Context, awsRegion string, secretRef core.ResourceRef, secrets v1.SecretList) ([]*ec2.Instance, error) {
	v, ok := m.responses[secretRef.Key()]
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
	secretFactory, err := bootstrap.SecretFactoryForSettings(ctx, settings, mc, &config, nil, &kubeCoreCache, v1.SecretCrd.Plural)
	Expect(err).NotTo(HaveOccurred())
	secretClient, err := v1.NewSecretClient(secretFactory)
	Expect(err).NotTo(HaveOccurred())
	return secretClient

}

func getMockListerResponses() mockListerResponses {
	resp := make(mockListerResponses)
	resp[testCredential1.Key()] = []*ec2.Instance{{
		PrivateIpAddress: aws.String(testPrivateIp1),
		PublicIpAddress:  aws.String(testPublicIp1),
		Tags: []*ec2.Tag{{
			Key:   aws.String("k1"),
			Value: aws.String("any old value"),
		}},
		VpcId: aws.String("id1"),
	}}
	resp[testCredential2.Key()] = []*ec2.Instance{{
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

func primeSecretClient(secretClient v1.SecretClient, secretRefs []core.ResourceRef) error {
	for _, ref := range secretRefs {
		secret := &v1.Secret{
			Kind: &v1.Secret_Aws{
				Aws: &v1.AwsSecret{
					AccessKey: "access",
					SecretKey: "secret",
				},
			},
			Metadata: core.Metadata{
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
